package publicapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mildronize/my-template/internal/identity"
)

// ApiKey defines the wire shape of one API key (openapi.yaml's ApiKey
// schema) — replaces internal/api.ApiKey (deleted this ticket,
// contract.md's "File/package layout": internal/api's framework-agnostic
// response types move into this package alongside the huma structs that
// now produce identical JSON). Same field names/JSON tags as before.
type ApiKey struct {
	ID        string    `json:"id"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ApiKeyList is GET /api/v1/keys' response envelope.
type ApiKeyList struct {
	Keys []ApiKey `json:"keys"`
}

// ListKeysOutput is ListKeys' huma.Register output.
type ListKeysOutput struct {
	Body ApiKeyList
}

// RevokeKeyInput is RevokeKey's huma.Register input — the key id, from
// the URL path.
type RevokeKeyInput struct {
	ID string `path:"id" doc:"The API key's own id."`
}

// RevokeKeyOutput is RevokeKey's huma.Register output — no Body field,
// so huma's own DefaultStatus inference (huma.go) gives this operation
// 204, matching today's c.Status(http.StatusNoContent).
type RevokeKeyOutput struct{}

func toAPIKey(k identity.APIKey) ApiKey {
	return ApiKey{
		ID:        k.ID,
		Prefix:    k.KeyPrefix,
		CreatedAt: k.CreatedAt,
		ExpiresAt: k.ExpiresAt,
	}
}

var notFoundKeyMessage = "no such key"

// RegisterKeysRoutes registers GET /api/v1/keys and DELETE
// /api/v1/keys/{id} on api (ticket-6) — both scoped to the caller's own
// keys (I3). There is deliberately no CreateKey/IssueKey operation here:
// issuance stays CLI-only (cmd/issue-key, API.md) — this surface only
// ever lists or revokes. api must already have humamw.RequireActor
// attached (cmd/server/main.go).
func RegisterKeysRoutes(api huma.API, svc *identity.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "listKeys",
		Method:      http.MethodGet,
		Path:        "/api/v1/keys",
		Summary:     "List the caller's own non-revoked API keys.",
		Description: "Lists the caller's own non-revoked keys (revoked_at IS NULL), regardless of expiry — an expired-but-unrevoked key still shows up so the caller can see it needs rotating (I9 is about the auth-time check, not this listing decision).",
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, _ *struct{}) (*ListKeysOutput, error) {
		user, err := actorFromContext(ctx)
		if err != nil {
			return nil, err
		}

		keys, err := svc.ListAPIKeys(ctx, user.ID)
		if err != nil {
			return nil, internalServerError()
		}

		resp := ApiKeyList{Keys: make([]ApiKey, 0, len(keys))}
		for _, k := range keys {
			resp.Keys = append(resp.Keys, toAPIKey(k))
		}
		return &ListKeysOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "revokeKey",
		Method:        http.MethodDelete,
		Path:          "/api/v1/keys/{id}",
		Summary:       "Revoke one of the caller's own API keys.",
		Description:   "Sets revoked_at. Owner-scoped, same 404 rule as every other owner-scoped resource on this surface (I3). There is deliberately no POST /api/v1/keys — issuance is CLI-only (cmd/issue-key).",
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusNotFound},
	}, func(ctx context.Context, input *RevokeKeyInput) (*RevokeKeyOutput, error) {
		user, err := actorFromContext(ctx)
		if err != nil {
			return nil, err
		}

		if _, err := svc.RevokeAPIKey(ctx, user.ID, input.ID); err != nil {
			if errors.Is(err, identity.ErrNotFound) {
				return nil, huma.Error404NotFound(notFoundKeyMessage)
			}
			return nil, internalServerError()
		}
		return &RevokeKeyOutput{}, nil
	})
}
