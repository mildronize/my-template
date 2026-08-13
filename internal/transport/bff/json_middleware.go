// json_middleware.go holds the session-gating middleware for
// milestone-3/task-2's new session-authenticated JSON surface
// (bff-openapi.yaml, internal/bffapi) — distinct from middleware.go's
// RequireSession (the pre-existing HTML view's gate) because the two
// surfaces must fail differently, not because the underlying session
// check differs: _contract/API.md is explicit that "missing, expired, or
// wrong-role session → 401 on this JSON surface (a behavior change from
// milestone-2's redirect-to-/login, since a fetch call can't follow a
// redirect the way a browser navigation does — the SPA's own
// AuthGate-equivalent hook is what turns a 401 into a redirect,
// client-side, not the BFF)". Reusing RequireSession's own gin.HandlerFunc
// directly would have made every request to this new surface redirect
// instead of answering 401, which is exactly the behavior the contract
// says must NOT happen here — so this file reuses RequireSession's
// checks (cookie parse, signature/expiry, user lookup, I12's role check,
// active check) via the same Signer/identity.UserRepo primitives, but
// owns its own response path.
package bff

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/transport/publicapi"
)

// jsonUnauthorizedBody is the one 401 response body RequireJSONSession
// ever writes (I5 — same "don't leak why" rule RequireSession's own
// redirect already follows, applied to this surface's JSON shape).
// Reuses internal/transport/publicapi.ErrorEnvelope/NewErrorEnvelope
// directly rather than redefining an equivalent type here — _contract/
// API.md's explicit "bff-openapi.yaml reuses publicapi's envelope"
// decision, applied to this package's one hand-written 401 body the same
// way todo_handler.go/keys_handler.go (this file, below) reuse it for
// their 404 bodies.
var jsonUnauthorizedBody = publicapi.NewErrorEnvelope("unauthorized", "authentication required", "")

// RequireJSONSession gates every route under /api/bff (milestone-3/
// task-2's new JSON surface, bff-openapi.yaml) on a valid session cookie —
// the JSON-surface counterpart to RequireSession (middleware.go), which
// gates the HTML view instead. See middleware.go's RequireSession doc
// comment for the full I2/I12 boundary reasoning (condensed from
// _contract/API.md): the short version is that a BFF session is the only
// way an owner ever authenticates (I2 forecloses issuing one a Bearer
// credential), and every route this middleware gates is therefore safe to
// treat as an owner-authenticated write surface that internal/transport/
// publicapi can never grow an equivalent of.
//
// A missing cookie, one that fails signature/expiry verification, or one
// that resolves to a user that no longer exists, is inactive, or has
// role="agent" (I12, checked here independently of GET /callback's own
// check and of RequireSession's own check, for the same defense-in-depth
// reasoning RequireSession's doc comment gives) all answer 401 with
// jsonUnauthorizedBody — never a redirect, per _contract/API.md's BFF
// section quoted above this file's package comment.
func RequireJSONSession(signer *Signer, users identity.UserRepo, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(sessionCookieName)
		if err != nil || cookie == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
			return
		}

		userID, err := signer.ParseSessionCookie(cookie)
		if err != nil {
			if logger != nil {
				logger.Debug("bff: session cookie failed verification (json surface)", "error", err)
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
			return
		}

		user, err := users.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			if logger != nil {
				logger.Debug("bff: session resolved to a missing user (json surface)", "user_id", userID)
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
			return
		}
		if user.Role == "agent" {
			// I12, checked at this layer too, not only at GET /callback and
			// RequireSession.
			if logger != nil {
				logger.Warn("bff: session resolved to role=agent, rejected (I12, json surface)", "user_id", userID)
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
			return
		}
		if !user.Active {
			c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
			return
		}

		c.Set(actorContextKey, user)
		c.Next()
	}
}
