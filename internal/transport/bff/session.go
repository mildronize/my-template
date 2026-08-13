// Package bff is the owner-facing transport surface (task-4.md,
// _contract/API.md's BFF section): GET /login, GET /callback, and the
// minimal authenticated view of the caller's own todos, all
// session-cookie-gated. Per ARCHITECTURE.md this package may import gin
// (it's transport) and internal/domain/todo + internal/identity (both are
// what it composes) — it must never be imported by either of those.
package bff

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Cookie names — distinct per DATA_MODEL.md's "BFF session" note and
// task-4.md's login-flow spec: sessionCookieName is the long-lived (well,
// an hour) proof of a completed login; stateCookieName only survives one
// redirect round-trip (GET /login -> Hydra -> GET /callback) and carries
// the PKCE verifier, never the other way around.
const (
	sessionCookieName = "session"
	stateCookieName   = "oauth_state"

	sessionTTL = time.Hour       // "reasonable TTL... an hour is fine" (task-4.md)
	stateTTL   = 5 * time.Minute // "short-lived (5-minute)" (task-4.md)
)

// errInvalidCookie is returned by ParseSessionCookie/ParseStateCookie for
// every failure reason (malformed, bad signature, expired) — callers
// (RequireSession, the callback handler) collapse all of them to the same
// "redirect to /login" / "show a generic error page" behavior, mirroring
// I5's "401 never leaks why" applied to this surface's own failure modes
// (_contract/API.md's BFF conventions).
var errInvalidCookie = errors.New("bff: invalid or expired cookie")

// Signer HMAC-signs and verifies both of this package's cookies. One
// signing helper, two uses (task-4.md's login-flow spec, step 1) — the
// state cookie and the session cookie differ only in what payload they
// carry and how long it's valid for, not in how the value is protected.
//
// Signer carries its own clock (like identity.Service's Clock) so tests
// can control "now" for expiry checks without sleeping — and, per
// task-4.md's Done-when-9 test, so a test can call NewSessionCookie
// directly to establish a session without driving it through
// GET /callback at all.
type Signer struct {
	secret []byte
	now    func() time.Time
}

// NewSigner builds a Signer from a raw secret. secret must be non-empty —
// callers are responsible for that (cmd/server/main.go generates an
// ephemeral one rather than ever constructing a Signer with an empty key,
// since an empty HMAC key makes every cookie trivially forgeable).
func NewSigner(secret []byte) *Signer {
	return &Signer{secret: secret, now: time.Now}
}

func (s *Signer) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// sign HMAC-signs payload and returns a cookie-safe string:
// base64url(payload) + "." + base64url(hmac-sha256(payload)). Unexported —
// only this file's typed wrappers (below) ever call it, so every caller
// outside this file works with typed claims, never a raw payload.
func (s *Signer) sign(payload []byte) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(payload)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// verify checks token's signature (constant-time, via hmac.Equal — never a
// plain byte-slice ==) and returns the payload it carried. This is the one
// place a forged or tampered cookie of either kind gets caught, before any
// caller ever unmarshals its claims.
func (s *Signer) verify(token string) ([]byte, error) {
	dot := strings.LastIndexByte(token, '.')
	if dot < 0 {
		return nil, errInvalidCookie
	}
	payload, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return nil, errInvalidCookie
	}
	sig, err := base64.RawURLEncoding.DecodeString(token[dot+1:])
	if err != nil {
		return nil, errInvalidCookie
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write(payload)
	expected := mac.Sum(nil)
	if !hmac.Equal(sig, expected) {
		return nil, errInvalidCookie
	}
	return payload, nil
}

// sessionClaims is the whole session cookie payload — DATA_MODEL.md's "BFF
// session" note: "{userID, expiresAt}", nothing else, no server-side store
// to cross-reference.
type sessionClaims struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewSessionCookie signs a session cookie value for userID, valid for
// sessionTTL from the Signer's own clock. Exported specifically so a test
// can establish a valid session directly (task-4.md's Done-when-9 test:
// "establishes a valid session cookie for that owner — call session.go's
// signing function directly, no need to drive it through /callback") and
// so the I12 view-middleware test can forge a session that resolves to an
// agent's users.id without needing a real login flow either.
func (s *Signer) NewSessionCookie(userID string) (string, error) {
	claims := sessionClaims{UserID: userID, ExpiresAt: s.clock().Add(sessionTTL)}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("bff: marshaling session claims: %w", err)
	}
	return s.sign(payload), nil
}

// ParseSessionCookie verifies value's signature and expiry and returns the
// userID it carried. This never touches a database — the signature plus
// the embedded expiresAt is the entire validity proof (DATA_MODEL.md: "no
// database round-trip to validate a session exists").
func (s *Signer) ParseSessionCookie(value string) (userID string, err error) {
	payload, err := s.verify(value)
	if err != nil {
		return "", err
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", errInvalidCookie
	}
	if !s.clock().Before(claims.ExpiresAt) {
		return "", errInvalidCookie
	}
	return claims.UserID, nil
}

// stateClaims is the state cookie's payload: the CSRF state value GET
// /login generated (checked back against the callback's own `state` query
// param — "standard CSRF-for-OAuth practice, independent of PKCE",
// task-4.md) and the PKCE verifier GET /callback must read back out to
// complete the token exchange (I11).
type stateClaims struct {
	State     string    `json:"state"`
	Verifier  string    `json:"verifier"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewStateCookie signs a state cookie carrying state and verifier, valid
// for stateTTL from the Signer's own clock.
func (s *Signer) NewStateCookie(state, verifier string) (string, error) {
	claims := stateClaims{State: state, Verifier: verifier, ExpiresAt: s.clock().Add(stateTTL)}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("bff: marshaling state claims: %w", err)
	}
	return s.sign(payload), nil
}

// ParseStateCookie verifies value's signature and expiry and returns the
// state and verifier it carried. A missing cookie, a tampered one, or one
// that's expired are all errInvalidCookie — the callback handler treats
// all three identically (task-4.md: "state cookie missing/tampered" is one
// case for I11's test, not three).
func (s *Signer) ParseStateCookie(value string) (state, verifier string, err error) {
	payload, err := s.verify(value)
	if err != nil {
		return "", "", err
	}
	var claims stateClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", "", errInvalidCookie
	}
	if !s.clock().Before(claims.ExpiresAt) {
		return "", "", errInvalidCookie
	}
	return claims.State, claims.Verifier, nil
}
