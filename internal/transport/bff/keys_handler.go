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
// /api/bff/keys and DELETE /api/bff/keys/{id}, both scoped to the
// session's own owner (I3). Calls the exact same *identity.Service
// instance/methods internal/transport/publicapi.KeysServer calls
// (ARCHITECTURE.md's shared-service-layer rule). There is deliberately no
// CreateKey/IssueKey/RotateKey method here — issuance and rotation stay
// CLI-only regardless of which surface is asking (_contract/API.md,
// milestone-3/_goal/GOAL.md Done-when 5) — see negative_check_test.go for
// the check that this surface never grows one.
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
// the session owner's own non-revoked keys regardless of expiry, same
// listing behavior as publicapi.KeysServer.ListKeys.
func (s *KeysServer) ListKeys(c *gin.Context) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	keys, err := s.Service.ListAPIKeys(c.Request.Context(), ownerID)
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
// Owner-scoped, same 404 rule as todos (I3): another owner's key id, or
// an id that never existed, both return not_found — never forbidden.
func (s *KeysServer) RevokeKey(c *gin.Context, id string) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	if _, err := s.Service.RevokeAPIKey(c.Request.Context(), ownerID, id); err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, bffKeyNotFoundBody)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}
