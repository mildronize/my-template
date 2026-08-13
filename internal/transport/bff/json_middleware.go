// json_middleware.go holds the session-gating middleware for
// milestone-3/task-2's new session-authenticated JSON surface
// (bff-openapi.yaml, internal/bffapi). _contract/API.md is explicit that
// "missing, expired, or wrong-role session → 401 on this JSON surface (a
// behavior change from milestone-2's redirect-to-/login, since a fetch
// call can't follow a redirect the way a browser navigation does — the
// SPA's own AuthGate-equivalent hook is what turns a 401 into a redirect,
// client-side, not the BFF)". milestone-2's HTML view had its own
// redirect-shaped session gate (middleware.go's RequireSession) built on
// the same checks below (cookie parse, signature/expiry, user lookup,
// I12's role check, active check) via the same Signer/identity.UserRepo
// primitives, but answering with a 401 body instead of a redirect is a
// different-enough response path that this file owns its own middleware
// rather than parameterizing that one — milestone-3/task-3 later deleted
// RequireSession once this one became its only caller's replacement (its
// last production caller, view_handler.go, was retired by the SPA).
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
// this is the html/template view's now-deleted RequireSession, session
// gate reused here for the JSON surface (see this file's package comment
// for why the two must fail differently).
//
// The I2/I12 boundary (why owner writes exist on bff and never on
// publicapi), condensed from milestone-3's _contract/API.md: I2 (a Bearer
// credential never resolves to role='owner') and I12 (checked here — a
// BFF session never resolves to role='agent') are two halves of one
// design, not independent rules that happen to coexist. An owner has no
// Bearer credential to present at all (I2 forecloses it structurally), so
// a BFF session is the *only* way an owner ever authenticates; an agent
// has no session to present, so a Bearer credential is the only way an
// agent ever authenticates. The two proof-of-identity mechanisms are
// disjoint by construction — which is why the write endpoints this
// middleware gates exist only on this package and can never be added to
// internal/transport/publicapi "for consistency": doing so would mean
// either issuing owners a Bearer credential (breaching I2) or teaching
// publicapi a session concept it has no reason to grow. See
// _contract/API.md's "The I2/I12 boundary" section for the full
// reasoning; this is a pointer, not a restatement.
//
// A missing cookie, one that fails signature/expiry verification, or one
// that resolves to a user that no longer exists, is inactive, or has
// role="agent" (I12, checked here independently of GET /callback's own
// check, for the same defense-in-depth reasoning) all answer 401 with
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
			// I12, checked at this layer too, not only at GET /callback.
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
