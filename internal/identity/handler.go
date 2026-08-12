package identity

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

// RegisterRoutes mounts this module's HTTP routes on group — the
// /api/v1 group cmd/server builds with RejectActorFields and
// RequireActor already attached (I1's request-shape guard runs before
// I2/I5's credential resolution). Settings endpoints (/keys) are
// task-4's; this task wires only GET /me, the natural smoke test that
// the actor-resolution middleware actually works end to end.
func RegisterRoutes(group *gin.RouterGroup, svc *Service) {
	group.GET("/me", handleMe)
}

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
