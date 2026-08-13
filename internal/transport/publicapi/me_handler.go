package publicapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
// existed) — task-3 brought it onto the same generated-interface,
// openapi-validated path as every other endpoint instead of leaving it on
// a bespoke route, by embedding MeServer alongside TodoServer (and
// KeysServer) in cmd/server's composite api.ServerInterface
// implementation. The underlying handleMe function, and its
// behavior/tests, are unchanged by this file's move out of
// internal/identity (ARCHITECTURE.md — identity keeps no transport code).
type MeServer struct{}

// GetMe implements api.ServerInterface.
func (MeServer) GetMe(c *gin.Context) { handleMe(c) }

// handleMe reads the actor RequireActor (middleware.go, this package)
// already resolved onto the gin context — it never queries users/api_keys
// itself (I4).
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
