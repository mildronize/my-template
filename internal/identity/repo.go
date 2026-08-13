// Package identity owns the users and api_keys tables and the
// actor-resolution logic (API key -> JWT -> reject) behind them — see
// doc.go for why the gin middleware that drives it lives in
// internal/transport/publicapi instead. Unlike internal/domain/todo, keep
// this directory on fork — every service built from this template needs
// its own identity/auth seam.
package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/mildronize/my-template/internal/db"
)

// ErrNotFound is returned by every Repo lookup when no row matches.
// service.go and handler.go only ever see this domain-level sentinel —
// never sql.ErrNoRows — so nothing outside this file needs to know sqlc or
// database/sql exist (ARCHITECTURE.md rule 2: only repo.go/*_repo.go may
// import the sqlc-generated package).
var ErrNotFound = errors.New("identity: not found")

// User is this package's own representation of a users row, deliberately
// distinct from db.User (the sqlc-generated type) so every other file in
// this package can talk about "a resolved actor" without importing
// internal/db itself.
type User struct {
	ID     string
	Handle string
	Role   string
	Active bool
	// SSOSubject is "" when the users.sso_subject column is NULL — a row
	// that only ever authenticates via API key (DATA_MODEL.md).
	SSOSubject string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// APIKey is this package's own representation of an api_keys row.
type APIKey struct {
	ID        string
	UserID    string
	KeyHash   string
	KeyPrefix string
	CreatedAt time.Time
	ExpiresAt time.Time
	// RevokedAt is nil when the key has not been revoked.
	RevokedAt *time.Time
	// Handle is the owning user's handle — nil for every method that has
	// no reason to resolve one (GetAPIKeyByHash, CreateAPIKey,
	// ListAPIKeysByOwner, RevokeAPIKey: each already scoped to a single
	// known userID, so a caller resolving its own identity gains nothing
	// from a redundant handle on every row). Populated only by
	// ListAllAgentAPIKeys (milestone-4 fix-round, handle-exposure) — the
	// owner-facing settings page's whole reason for existing is showing
	// WHICH agent a row belongs to (my-task's own api-key-settings.tsx:
	// `{k.handle}` on every row, "Revoke {handle}'s key?"), and that is
	// the one query whose own SQL (db/queries/api_keys.sql) now joins
	// users for it.
	//
	// A pointer on the shared type, not a second, ListAllAgentAPIKeys-only
	// return type: every consumer already handles APIKey as one shape
	// (RevokeAnyAgentAPIKey takes/returns it, keys_handler.go's toBFFKey
	// maps it), and a second parallel type would need either its own
	// toBFFKey twin or a lossy conversion back to APIKey before reaching
	// that mapper — more moving parts for one optional field. Every other
	// method leaves this nil, never a guessed or empty-string value, so a
	// caller can tell "not resolved by this method" apart from a
	// hypothetical future empty handle.
	Handle *string
}

func userFromRow(row db.User) User {
	return User{
		ID:         row.ID,
		Handle:     row.Handle,
		Role:       row.Role,
		Active:     row.Active,
		SSOSubject: row.SsoSubject.String,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

func apiKeyFromRow(row db.ApiKey) APIKey {
	k := APIKey{
		ID:        row.ID,
		UserID:    row.UserID,
		KeyHash:   row.KeyHash,
		KeyPrefix: row.KeyPrefix,
		CreatedAt: row.CreatedAt,
		ExpiresAt: row.ExpiresAt,
	}
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		k.RevokedAt = &t
	}
	return k
}

// Repo is the only type in this package that imports the sqlc-generated
// package (internal/db) — every other file reaches the database only
// through Repo's methods (ARCHITECTURE.md rule 2, I4).
type Repo struct {
	q *db.Queries
}

// NewRepo builds a Repo on top of an already-open *sql.DB (see
// platform.OpenDB).
func NewRepo(conn *sql.DB) *Repo {
	return &Repo{q: db.New(conn)}
}

// GetUserByID looks up a users row by id.
func (r *Repo) GetUserByID(ctx context.Context, id string) (User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return userFromRow(row), nil
}

// GetUserByHandle looks up a users row by handle. handle is COLLATE
// NOCASE at the schema level (see the migration), so this comparison is
// case-insensitive without needing an explicit COLLATE here — matching
// DATA_MODEL.md's "matched exactly, case-insensitively, never fuzzily".
func (r *Repo) GetUserByHandle(ctx context.Context, handle string) (User, error) {
	row, err := r.q.GetUserByHandle(ctx, handle)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return userFromRow(row), nil
}

// GetUserBySSOSubject looks up a users row by its sso_subject (Hydra's
// `sub` claim) — the JWT branch's lookup (I10).
func (r *Repo) GetUserBySSOSubject(ctx context.Context, sub string) (User, error) {
	row, err := r.q.GetUserBySSOSubject(ctx, sql.NullString{String: sub, Valid: true})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return userFromRow(row), nil
}

// CreateUser inserts a new users row. ssoSubject is nil for a row that
// only ever authenticates via API key (DATA_MODEL.md). id and the
// timestamps are generated here, not left to the caller.
func (r *Repo) CreateUser(ctx context.Context, handle, role string, ssoSubject *string) (User, error) {
	now := time.Now().UTC()
	arg := db.CreateUserParams{
		ID:        uuid.NewString(),
		Handle:    handle,
		Role:      role,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if ssoSubject != nil {
		arg.SsoSubject = sql.NullString{String: *ssoSubject, Valid: true}
	}
	row, err := r.q.CreateUser(ctx, arg)
	if err != nil {
		return User{}, err
	}
	return userFromRow(row), nil
}

// GetAPIKeyByHash looks up an api_keys row by key_hash — the API-key
// branch's lookup (task-2.md step 2).
func (r *Repo) GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error) {
	row, err := r.q.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKey{}, ErrNotFound
		}
		return APIKey{}, err
	}
	return apiKeyFromRow(row), nil
}

// CreateAPIKey inserts a new api_keys row. keyHash and keyPrefix are
// derived by the caller (service.go) from the raw key; the raw key itself
// is never passed to or stored by this layer (I8). id and created_at are
// generated here.
func (r *Repo) CreateAPIKey(ctx context.Context, userID, keyHash, keyPrefix string, expiresAt time.Time) (APIKey, error) {
	row, err := r.q.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:        uuid.NewString(),
		UserID:    userID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return APIKey{}, err
	}
	return apiKeyFromRow(row), nil
}

// ListAPIKeysByOwner returns userID's own non-revoked keys
// (`revoked_at IS NULL`), regardless of expiry — an expired-but-unrevoked
// key still shows up so the caller can see it needs rotating (API.md
// `GET /api/v1/keys`; this is a listing decision, distinct from I9's
// auth-time check in Service.tryAPIKey). Ordered created_at descending,
// matching todo's ListByOwner.
func (r *Repo) ListAPIKeysByOwner(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := r.q.ListAPIKeysByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	keys := make([]APIKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, apiKeyFromRow(row))
	}
	return keys, nil
}

// RevokeAPIKey sets revoked_at on the key identified by (id, userID) — the
// userID scoping means a caller can only ever revoke its own key.
// ErrNotFound covers both "no such key" and "not this caller's key",
// mirroring I3's ownership-scoping rule (absence, not permission) applied
// here to keys instead of todos.
func (r *Repo) RevokeAPIKey(ctx context.Context, id, userID string) (APIKey, error) {
	row, err := r.q.RevokeAPIKey(ctx, db.RevokeAPIKeyParams{
		RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		ID:        id,
		UserID:    userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKey{}, ErrNotFound
		}
		return APIKey{}, err
	}
	return apiKeyFromRow(row), nil
}

// ListAllAgentAPIKeys returns every role='agent' user's non-revoked keys
// (I21) — the owner-facing settings-page query, deliberately not scoped to
// any one user_id. Unlike ListAPIKeysByOwner (which structurally can never
// be non-empty for a session owner, since no key is ever issued to
// role='owner' — I2), this is the query the settings page actually needs.
//
// milestone-4 fix-round (handle-exposure): the underlying query
// (db/queries/api_keys.sql) now JOINs users for the owning agent's
// handle, so every APIKey returned here has Handle set (never nil) — the
// query's own JOIN, not LEFT JOIN, guarantees a matching users row exists
// for every api_keys row it returns.
func (r *Repo) ListAllAgentAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := r.q.ListAllAgentAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]APIKey, 0, len(rows))
	for _, row := range rows {
		k := apiKeyFromRow(db.ApiKey{
			ID:        row.ID,
			UserID:    row.UserID,
			KeyHash:   row.KeyHash,
			KeyPrefix: row.KeyPrefix,
			CreatedAt: row.CreatedAt,
			ExpiresAt: row.ExpiresAt,
			RevokedAt: row.RevokedAt,
		})
		handle := row.Handle
		k.Handle = &handle
		keys = append(keys, k)
	}
	return keys, nil
}

// RevokeAPIKeyByID revokes the key identified by id alone (I21) — no
// user_id scoping, since the owner-facing endpoint may revoke any agent's
// key, not just one caller's own. Session-gating (must be an owner at all)
// is the handler's job, not this query's; ErrNotFound covers both "no such
// key" and "already revoked", the same "absence, not permission" shape
// RevokeAPIKey gives the self-scoped case.
func (r *Repo) RevokeAPIKeyByID(ctx context.Context, id string) (APIKey, error) {
	row, err := r.q.RevokeAPIKeyByID(ctx, db.RevokeAPIKeyByIDParams{
		RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		ID:        id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKey{}, ErrNotFound
		}
		return APIKey{}, err
	}
	return apiKeyFromRow(row), nil
}

// DisableOtherAPIKeys revokes every one of userID's still-live keys except
// keepID, returning how many it revoked. This is service.go's Rotate
// (I13) calling in, after it has already issued and stored the key it
// passes as keepID — deliberately built from ListAPIKeysByOwner +
// RevokeAPIKey (both already I4-legal, already tested) rather than a new
// sqlc query, so I4 ("one repo, one table" per domain module) doesn't grow
// a second db/queries/*.sql surface just for this. A key that was already
// revoked before this call (e.g. one the owner revoked by hand earlier) is
// simply skipped, not double-counted or errored on — ListAPIKeysByOwner
// already excludes it.
func (r *Repo) DisableOtherAPIKeys(ctx context.Context, userID, keepID string) (int, error) {
	live, err := r.ListAPIKeysByOwner(ctx, userID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, key := range live {
		if key.ID == keepID {
			continue
		}
		if _, err := r.RevokeAPIKey(ctx, key.ID, userID); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
