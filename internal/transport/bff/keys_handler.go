package bff

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/transport/publicapi"
)

// bffKeyNotFoundBody is the one 404 response body RevokeKey ever writes
// for an unknown or not-this-caller's key id (I3). Reuses
// internal/transport/publicapi's ErrorEnvelope/NewErrorEnvelope directly,
// same reasoning as todo_handler.go's bffTodoNotFoundError.
var bffKeyNotFoundBody = publicapi.NewErrorEnvelope("not_found", "no such key", "")

// KeysServer adapts identity.Service to internal/bffapi's generated
// ServerInterface's keys-shaped subset (ListKeys, RevokeKey) — GET
// /api/bff/keys and DELETE /api/bff/keys/{id}. I21 (_contract/
// INVARIANTS.md, milestone-4): unlike internal/transport/publicapi's own
// KeysServer (still self-scoped, I3, unchanged), this surface deliberately
// spans every role='agent' user's keys, not the session owner's own —
// milestone-2/3's session-owner-scoped semantics are REPLACED here, not
// kept alongside a new endpoint (GOAL.md's "GET/DELETE /api/bff/keys —
// replace, not add beside" decision), since the old semantics could
// structurally never return anything (no key is ever issued to
// role='owner' — I2). Calls the exact same *identity.Service instance
// internal/transport/publicapi.KeysServer calls (ARCHITECTURE.md's
// shared-service-layer rule), just different methods on it. There is
// deliberately no CreateKey/IssueKey/RotateKey method here — issuance and
// rotation stay CLI-only regardless of which surface is asking
// (_contract/API.md, milestone-3/_goal/GOAL.md Done-when 5) — see
// negative_check_test.go for the check that this surface never grows one.
type KeysServer struct {
	Service *identity.Service
}

// NewKeysServer builds a KeysServer on top of svc.
func NewKeysServer(svc *identity.Service) *KeysServer {
	return &KeysServer{Service: svc}
}

func toBFFKey(k identity.APIKey) bffapi.ApiKey {
	return bffapi.ApiKey{
		Id:        k.ID,
		Prefix:    k.KeyPrefix,
		CreatedAt: k.CreatedAt,
		ExpiresAt: k.ExpiresAt,
	}
}

// ListKeys implements bffapi.ServerInterface — GET /api/bff/keys. Returns
// every role='agent' user's non-revoked keys (I21) — a valid owner session
// is required to reach this at all (bffOwnerID), but the query itself is
// deliberately not scoped to that session's own user_id: an owner never
// holds a key (I2), so scoping to ownerID would always be empty, which is
// exactly milestone-2/3's bug this replaces.
func (s *KeysServer) ListKeys(c *gin.Context) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	keys, err := s.Service.ListAllAgentAPIKeys(c.Request.Context())
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	resp := bffapi.ApiKeyList{Keys: make([]bffapi.ApiKey, 0, len(keys))}
	for _, k := range keys {
		resp.Keys = append(resp.Keys, toBFFKey(k))
	}
	c.JSON(http.StatusOK, resp)
}

// RevokeKey implements bffapi.ServerInterface — DELETE /api/bff/keys/{id}.
// I21: session-gated (a valid owner session is required to reach this at
// all), but no longer scoped to that session's own user_id — the owner may
// revoke any agent's key. An id that never existed (or was already
// revoked) returns not_found, the same "absence, not permission" shape I3
// gives todos, applied here to the single dimension left to protect once
// user_id-scoping is gone.
func (s *KeysServer) RevokeKey(c *gin.Context, id string) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	if _, err := s.Service.RevokeAnyAgentAPIKey(c.Request.Context(), id); err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, bffKeyNotFoundBody)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}
