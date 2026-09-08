package bff

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mildronize/my-template/internal/identity"
)

// ApiKey defines the wire shape of one API key (bff-openapi.yaml's ApiKey
// schema) — replaces internal/bffapi.ApiKey (deleted this ticket,
// mirroring contract.md's "File/package layout" for /api/v1: the deleted
// package's framework-agnostic response types move into this package
// alongside the huma structs that now produce identical JSON). Same field
// names/JSON tags as before (this ticket deliberately keeps oapi-codegen's
// own `Id`-style Go field names rather than renaming to `ID` the way
// internal/transport/publicapi's ticket-6 migration did — same JSON wire
// shape either way, a lower-risk choice for this ticket's own test-file
// diff since every existing bffapi.ApiKey/.../User/.../ActivityCursor
// field reference in this package's tests only needs the `bffapi.`
// qualifier dropped, not a field-name rename too).
type ApiKey struct {
	Id        string    `json:"id"`
	Prefix    string    `json:"prefix"`
	Handle    string    `json:"handle"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ApiKeyList is GET /api/bff/keys' response envelope.
type ApiKeyList struct {
	Keys []ApiKey `json:"keys"`
}

// ListKeysOutput is ListKeys' huma.Register output.
type ListKeysOutput struct {
	Body ApiKeyList
}

// RevokeKeyInput is RevokeKey's huma.Register input — the key id, from the
// URL path.
type RevokeKeyInput struct {
	ID string `path:"id" doc:"The API key's own id."`
}

// RevokeKeyOutput is RevokeKey's huma.Register output — no Body field, so
// huma's own DefaultStatus inference (this operation sets DefaultStatus:
// 204 explicitly below) matches today's c.Status(http.StatusNoContent).
type RevokeKeyOutput struct{}

// toBFFKey's only caller is ListKeys, which only ever calls
// identity.Service.ListAllAgentAPIKeys — the one identity.APIKey-producing
// method that populates Handle (milestone-4 fix-round, handle-exposure).
// k.Handle is asserted non-nil here rather than defended against, on
// purpose — mirrors the pre-ticket-7 gin handler's own identical
// reasoning: a nil Handle reaching this function would mean
// ListAllAgentAPIKeys' own JOIN (not LEFT JOIN) somehow returned a row
// with no matching user, a bug in that query, not a normal runtime
// condition this handler should paper over.
func toBFFKey(k identity.APIKey) ApiKey {
	return ApiKey{
		Id:        k.ID,
		Prefix:    k.KeyPrefix,
		Handle:    *k.Handle,
		CreatedAt: k.CreatedAt,
		ExpiresAt: k.ExpiresAt,
	}
}

var bffKeyNotFoundMessage = "no such key"

// RegisterKeysRoutes registers GET /api/bff/keys and DELETE
// /api/bff/keys/{id} on api (ticket-7) — I21: every role='agent' user's
// non-revoked keys (not the session owner's own — an owner never holds a
// key, I2), session-gated only, no further scoping. There is deliberately
// no CreateKey/IssueKey/RotateKey operation here — issuance and rotation
// stay CLI-only regardless of which surface is asking (negative_check_test.go
// checks this surface never grows one). api must already have
// humamw.RequireSession attached (cmd/server/main.go).
func RegisterKeysRoutes(api huma.API, svc *identity.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "listKeys",
		Method:      http.MethodGet,
		Path:        "/api/bff/keys",
		Summary:     "List every agent's non-revoked API keys (I21).",
		Description: "Every role='agent' user's non-revoked keys (revoked_at IS NULL), not just the session owner's own — the session owner never holds a key at all (role='owner' can't be issued one, I2). Regardless of expiry: an expired-but-unrevoked key still shows up so the owner can see it needs rotating.",
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, _ *struct{}) (*ListKeysOutput, error) {
		if _, err := actorFromContext(ctx); err != nil {
			return nil, err
		}

		keys, err := svc.ListAllAgentAPIKeys(ctx)
		if err != nil {
			return nil, internalServerError()
		}

		resp := ApiKeyList{Keys: make([]ApiKey, 0, len(keys))}
		for _, k := range keys {
			resp.Keys = append(resp.Keys, toBFFKey(k))
		}
		return &ListKeysOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "revokeKey",
		Method:        http.MethodDelete,
		Path:          "/api/bff/keys/{id}",
		Summary:       "Revoke any agent's API key by id (I21).",
		Description:   "Sets revoked_at. Not scoped to a \"caller's own\" key — there is no such thing on this owner-only surface (I2) — any agent's key id may be revoked, session-gated only.",
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusNotFound},
	}, func(ctx context.Context, input *RevokeKeyInput) (*RevokeKeyOutput, error) {
		if _, err := actorFromContext(ctx); err != nil {
			return nil, err
		}

		if _, err := svc.RevokeAnyAgentAPIKey(ctx, input.ID); err != nil {
			if errors.Is(err, identity.ErrNotFound) {
				return nil, huma.Error404NotFound(bffKeyNotFoundMessage)
			}
			return nil, internalServerError()
		}
		return &RevokeKeyOutput{}, nil
	})
}
