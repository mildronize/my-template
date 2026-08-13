package bff

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MeServer adapts handleBFFMe to internal/bffapi's generated
// ServerInterface's GetMe method — GET /api/bff/me, the session-check
// endpoint backing the SPA's AuthGate-equivalent hook (milestone-3/
// task-2, _contract/API.md).
type MeServer struct{}

// GetMe implements bffapi.ServerInterface.
func (MeServer) GetMe(c *gin.Context) { handleBFFMe(c) }

// bffMeResponse is GET /api/bff/me's response shape (_contract/API.md) —
// deliberately the exact same shape as internal/transport/publicapi's own
// GET /api/v1/me response (meResponse, me_handler.go): one response shape
// for "who am I," regardless of which surface asked. Not literally the
// same Go type as publicapi's meResponse (that type is unexported there,
// and the two surfaces' generated bffapi.Me/api.Me types are already
// independent per-package types by construction — see bff-openapi.yaml's
// own info.description on why two spec files exist at all) — the
// contract that matters is the wire shape, which this matches field for
// field.
type bffMeResponse struct {
	Handle string `json:"handle"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

// handleBFFMe reads the actor RequireJSONSession already resolved onto
// the gin context (ActorFromContext) — it never queries users itself
// (I4).
func handleBFFMe(c *gin.Context) {
	user, ok := ActorFromContext(c)
	if !ok {
		// Unreachable when mounted behind RequireJSONSession, but fails
		// safe (401, not a panic) if this route is ever wired without it.
		c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
		return
	}
	c.JSON(http.StatusOK, bffMeResponse{
		Handle: user.Handle,
		Role:   user.Role,
		Active: user.Active,
	})
}
