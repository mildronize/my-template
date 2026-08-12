package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ErrUnauthorized is the single sentinel every failure path in
// ResolveActor produces — I5: 401 never leaks why. The specific reason
// each call site passes to unauthorized() is logged server-side only and
// never reaches the returned error's caller-visible value.
var ErrUnauthorized = errors.New("identity: unauthorized")

// UserRepo is the subset of Repo's user-facing methods Service depends
// on. Declared here (not in repo.go) so tests can supply a fake without a
// real database — repo.go's *Repo satisfies this interface structurally,
// with no import of internal/db required on this side.
type UserRepo interface {
	GetUserByID(ctx context.Context, id string) (User, error)
	GetUserByHandle(ctx context.Context, handle string) (User, error)
	GetUserBySSOSubject(ctx context.Context, sub string) (User, error)
	CreateUser(ctx context.Context, handle, role string, ssoSubject *string) (User, error)
}

// APIKeyRepo is the subset of Repo's api_keys-facing methods Service
// depends on.
type APIKeyRepo interface {
	GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error)
	CreateAPIKey(ctx context.Context, userID, keyHash, keyPrefix string, expiresAt time.Time) (APIKey, error)
	RevokeAPIKey(ctx context.Context, id, userID string) (APIKey, error)
}

// JWTVerifier verifies a Bearer token as an SSO-issued JWT (task-2.md
// step 3) and returns its `sub` claim on success. NewJWTVerifier (jwt.go)
// builds the production instance; tests inject a fake, or a real instance
// pointed at an httptest JWKS server.
type JWTVerifier interface {
	Verify(ctx context.Context, token string) (sub string, err error)
}

// Clock lets tests control "now" for expiry checks (I9) without sleeping
// or backdating fixtures.
type Clock func() time.Time

// Service implements the actor-resolution contract from task-2.md: try
// the token as an API key first; only if no api_keys row matched at all,
// try it as a JWT; then funnel whatever either branch resolved through
// one shared gate that rejects role='owner' (I2) and inactive users, so
// that check is implemented once but independently exercised per
// credential path by the tests.
type Service struct {
	Users   UserRepo
	APIKeys APIKeyRepo
	JWT     JWTVerifier // nil disables the JWT branch entirely (dormant seam, GOAL.md)
	Logger  *slog.Logger
	Now     Clock
}

// NewService wires a Service from its dependencies. jwtVerifier may be
// nil — the JWT branch is a wired-but-dormant seam (GOAL.md) and a
// deployment without SSO_ISSUER/AUTH_AUDIENCE configured simply never
// takes it, falling straight through to "unauthorized" for any Bearer
// token that isn't a live API key.
func NewService(users UserRepo, apiKeys APIKeyRepo, jwtVerifier JWTVerifier, logger *slog.Logger) *Service {
	return &Service{Users: users, APIKeys: apiKeys, JWT: jwtVerifier, Logger: logger, Now: time.Now}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// HashAPIKey returns the SHA-256 hex digest of a raw API key, matching
// DATA_MODEL.md's api_keys.key_hash column (I8: the raw key is never
// stored). Exported so cmd/issue-key hashes exactly the way this package
// later verifies — one definition, not two copies that could drift apart.
func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// unauthorized is ResolveActor's single point of "what a 401 looks like
// from the inside": every failure reason is logged here, server-side
// only, and the same ErrUnauthorized sentinel goes back to the caller
// regardless of reason (I5) — never a per-branch, differently-shaped
// error.
func (s *Service) unauthorized(reason string) error {
	s.logger().Debug("resolveActor: unauthorized", "reason", reason)
	return ErrUnauthorized
}

// bearerToken extracts the credential from an Authorization header value,
// or reports it missing/malformed (task-2.md step 1).
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

// ResolveActor is the actor-resolution middleware's core (task-2.md):
//
//  1. Missing/malformed Authorization -> unauthorized, no further checks.
//  2. Try the token as an API key first. A row that's expired or revoked
//     stops here (I9) — it does NOT fall through to the JWT branch, since
//     a token that matched an api_keys row is an API key, full stop.
//  3. Only if no api_keys row matched at all, try it as a JWT (I6, I7,
//     I10). A JWT validation failure here is the routine case (most
//     Bearer tokens are API keys), so it's logged at debug, not error.
//  4. Whatever either branch resolved (or didn't) passes through one
//     shared gate rejecting role='owner' (I2) and inactive users — this
//     is where the request either becomes a resolved User or, one more
//     time, unauthorized.
func (s *Service) ResolveActor(ctx context.Context, authorizationHeader string) (User, error) {
	token, ok := bearerToken(authorizationHeader)
	if !ok {
		return User{}, s.unauthorized("missing or non-Bearer Authorization header")
	}

	user, matched, err := s.tryAPIKey(ctx, token)
	if err != nil {
		// A matched-but-expired/revoked api_keys row (I9) — do not try
		// the JWT branch with the same token.
		return User{}, err
	}
	if !matched {
		user, matched = s.tryJWT(ctx, token)
	}
	if !matched {
		return User{}, s.unauthorized("bearer token resolved via neither API key nor JWT")
	}

	return s.gate(user)
}

// tryAPIKey looks the token up as an api_keys row. Three outcomes: no row
// at all (matched=false, nil error — try JWT next); a row that's expired
// or revoked (matched=true, err=ErrUnauthorized — I9, never falls through
// to JWT); or a live row (matched=true, nil error, user resolved via its
// user_id).
func (s *Service) tryAPIKey(ctx context.Context, token string) (User, bool, error) {
	hash := HashAPIKey(token)
	key, err := s.APIKeys.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, false, nil // not an API key at all — try JWT next
		}
		s.logger().Error("resolveActor: api key lookup failed", "error", err)
		return User{}, false, s.unauthorized("api key lookup error")
	}

	// I9 — checked live, on every request, not just at issuance.
	if key.RevokedAt != nil || !key.ExpiresAt.After(s.now()) {
		return User{}, true, s.unauthorized("api key expired or revoked")
	}

	user, err := s.Users.GetUserByID(ctx, key.UserID)
	if err != nil {
		s.logger().Error("resolveActor: api key referenced a missing user", "error", err, "user_id", key.UserID)
		return User{}, true, s.unauthorized("api key's user_id has no matching user")
	}
	return user, true, nil
}

// tryJWT attempts the token as an SSO-issued JWT. A validation failure
// here is the routine case, not a server error — most Bearer tokens
// presented to this service are API keys, so this logs at debug (matching
// my-task's resolveActor comment) and simply reports matched=false; there
// is no separately-shaped "JWT branch failed" error, since both branches
// funnel into the same final unauthorized() in ResolveActor.
func (s *Service) tryJWT(ctx context.Context, token string) (User, bool) {
	if s.JWT == nil {
		return User{}, false
	}

	sub, err := s.JWT.Verify(ctx, token)
	if err != nil {
		s.logger().Debug("resolveActor: bearer token failed JWT validation", "error", err)
		return User{}, false
	}

	// I10 — a JWT sub with no matching users.sso_subject row is
	// unauthorized, never auto-created.
	user, err := s.Users.GetUserBySSOSubject(ctx, sub)
	if err != nil {
		s.logger().Debug("resolveActor: JWT subject has no matching user (I10)", "sub", sub)
		return User{}, false
	}
	return user, true
}

// gate is the one shared checkpoint both credential paths funnel through:
// role='owner' is rejected (I2) and inactive users are rejected. It is
// implemented once, here, rather than duplicated inside tryAPIKey and
// tryJWT — TestI2_BearerNeverResolvesToOwner_APIKeyPath and
// TestI2_BearerNeverResolvesToOwner_JWTPath each drive execution through
// this same code via a different entry branch, which is what proves I2
// holds on both paths without the check itself being copy-pasted twice.
func (s *Service) gate(user User) (User, error) {
	if user.Role == "owner" {
		return User{}, s.unauthorized("bearer credential resolved to role=owner (I2)")
	}
	if !user.Active {
		return User{}, s.unauthorized("resolved user is inactive")
	}
	return user, nil
}

// --- CLI key issuance (cmd/issue-key) -------------------------------------

const (
	apiKeyRawPrefix    = "tpl_"
	apiKeyPrefixLength = 12 // "tpl_" + first 8 chars of the random portion (DATA_MODEL.md/API.md)
	apiKeyRandomBytes  = 32
	apiKeyTTL          = 90 * 24 * time.Hour // matches my-task's mtk_ convention
)

// KeyIssuanceResult is what cmd/issue-key needs to print: the raw key
// (shown exactly once) plus the metadata that actually got stored.
type KeyIssuanceResult struct {
	User   User
	APIKey APIKey
	// RawKey exists only here, in memory, at issuance time — it is never
	// stored (I8) and this is the only place in the codebase it appears.
	RawKey string
}

// IssueAPIKeyForHandle mirrors my-task's agent:add: find-or-create a
// users row for handle (role='agent', active, no sso_subject — API-key
// only identity, DATA_MODEL.md), then mint it a new API key. This is the
// only path that ever produces a raw key (task-2.md — no
// POST /api/v1/keys exists).
func (s *Service) IssueAPIKeyForHandle(ctx context.Context, handle string) (KeyIssuanceResult, error) {
	user, err := s.Users.GetUserByHandle(ctx, handle)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return KeyIssuanceResult{}, fmt.Errorf("looking up user %q: %w", handle, err)
		}
		user, err = s.Users.CreateUser(ctx, handle, "agent", nil)
		if err != nil {
			return KeyIssuanceResult{}, fmt.Errorf("creating user %q: %w", handle, err)
		}
	}

	raw, err := generateRawAPIKey()
	if err != nil {
		return KeyIssuanceResult{}, fmt.Errorf("generating key: %w", err)
	}
	hash := HashAPIKey(raw)
	prefix := raw[:apiKeyPrefixLength]

	key, err := s.APIKeys.CreateAPIKey(ctx, user.ID, hash, prefix, s.now().Add(apiKeyTTL))
	if err != nil {
		return KeyIssuanceResult{}, fmt.Errorf("storing key: %w", err)
	}

	return KeyIssuanceResult{User: user, APIKey: key, RawKey: raw}, nil
}

// generateRawAPIKey returns a new "tpl_<64 hex chars>" raw key. The first
// apiKeyPrefixLength characters of the result are what gets stored as
// key_prefix (in the clear); the whole string is hashed for key_hash and
// never stored itself.
func generateRawAPIKey() (string, error) {
	buf := make([]byte, apiKeyRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return apiKeyRawPrefix + hex.EncodeToString(buf), nil
}
