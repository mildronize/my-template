// Package identity owns the users and api_keys tables and the
// actor-resolution middleware (API key -> JWT -> reject). Unlike
// internal/todo, keep this directory on fork — every service built from
// this template needs its own identity/auth seam.
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
