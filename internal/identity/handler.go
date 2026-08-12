package identity

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/api"
)

// meResponse is GET /api/v1/me's response shape (_contract/API.md).
type meResponse struct {
	Handle string `json:"handle"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

// MeServer adapts handleMe to internal/api's generated
// ServerInterface's GetMe method. GET /api/v1/me predates openapi.yaml
// (task-2 hand-wired it directly on the gin group before that file
// existed) — task-3 brings it onto the same generated-interface,
// openapi-validated path as every other endpoint instead of leaving it on
// a bespoke route, by embedding MeServer alongside internal/todo's Server
// in cmd/server's composite api.ServerInterface implementation. The
// underlying handleMe function, and its behavior/tests, are unchanged.
type MeServer struct{}

// GetMe implements api.ServerInterface.
func (MeServer) GetMe(c *gin.Context) { handleMe(c) }

// handleMe reads the actor RequireActor already resolved onto the gin
// context — it never queries users/api_keys itself (I4).
func handleMe(c *gin.Context) {
	user, ok := ActorFromContext(c)
	if !ok {
		// RequireActor guarantees this is set whenever the middleware
		// chain runs in the intended order. Treated as a defensive 401
		// (not a panic) in case a route is ever wired without it.
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
		return
	}
	c.JSON(http.StatusOK, meResponse{
		Handle: user.Handle,
		Role:   user.Role,
		Active: user.Active,
	})
}

// notFoundBody is the one 404 response body RevokeKey ever writes for an
// unknown or not-this-caller's key id — same "not_found, never forbidden"
// shape I3 gives todos (internal/todo/handler.go's notFoundError),
// applied here to keys.
var notFoundBody = newErrorEnvelope("not_found", "no such key", "")

// KeysServer adapts Service to internal/api's generated ServerInterface's
// keys-shaped subset (ListKeys, RevokeKey) — GET /api/v1/keys and
// DELETE /api/v1/keys/{id}, both scoped to the caller's own keys (I3).
// There is deliberately no CreateKey/IssueKey method here: issuance stays
// CLI-only (cmd/issue-key, API.md) — this server only ever lists or
// revokes.
type KeysServer struct {
	Service *Service
}

// NewKeysServer builds a KeysServer on top of svc.
func NewKeysServer(svc *Service) *KeysServer {
	return &KeysServer{Service: svc}
}

func toAPIKey(k APIKey) api.ApiKey {
	return api.ApiKey{
		Id:        k.ID,
		Prefix:    k.KeyPrefix,
		CreatedAt: k.CreatedAt,
		ExpiresAt: k.ExpiresAt,
	}
}

// keysActorID mirrors internal/todo/handler.go's actorID: reads the actor
// RequireActor already resolved onto the gin context — this handler never
// queries users/api_keys itself (I4). The !ok branch is defensive,
// unreachable given the intended middleware order.
func keysActorID(c *gin.Context) (string, bool) {
	user, ok := ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
		return "", false
	}
	return user.ID, true
}

// ListKeys implements api.ServerInterface — GET /api/v1/keys. Returns the
// caller's own non-revoked keys regardless of expiry (API.md) — an
// expired-but-unrevoked key still shows up here even though the exact
// same key would fail authentication (I9); those are two different checks
// (see Service.ListAPIKeys's doc comment).
func (s *KeysServer) ListKeys(c *gin.Context) {
	ownerID, ok := keysActorID(c)
	if !ok {
		return
	}

	keys, err := s.Service.ListAPIKeys(c.Request.Context(), ownerID)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	resp := api.ApiKeyList{Keys: make([]api.ApiKey, 0, len(keys))}
	for _, k := range keys {
		resp.Keys = append(resp.Keys, toAPIKey(k))
	}
	c.JSON(http.StatusOK, resp)
}

// RevokeKey implements api.ServerInterface — DELETE /api/v1/keys/{id}.
// Owner-scoped, same 404 rule as todos (I3): another owner's key id, or
// an id that never existed, both return not_found — never forbidden.
func (s *KeysServer) RevokeKey(c *gin.Context, id string) {
	ownerID, ok := keysActorID(c)
	if !ok {
		return
	}

	if _, err := s.Service.RevokeAPIKey(c.Request.Context(), ownerID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, notFoundBody)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}
