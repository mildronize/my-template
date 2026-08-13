package bff

import (
	"html"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/identity"
)

// actorContextKey is this package's own gin-context key for the resolved
// owner — deliberately not shared with internal/transport/publicapi's own
// (unexported, package-private) key of the same purpose. ARCHITECTURE.md's
// "no transport/shared package" decision means each transport surface
// owns this small amount of duplication rather than reaching for a shared
// package that would otherwise hold nothing else.
const actorContextKey = "bff.actor"

// setCookie centralizes the cookie attributes task-4.md's login-flow spec
// calls for on both cookies this package sets: HttpOnly (never readable
// from JS), Secure, SameSite=Lax. path "/" so the cookie is sent back on
// every route this package registers, not just the one that set it.
func setCookie(c *gin.Context, name, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, "/", "", true, true)
}

// clearCookie deletes a cookie this package previously set (maxAge<0).
func clearCookie(c *gin.Context, name string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, "", -1, "/", "", true, true)
}

// renderLoginError is the one error page GET /callback ever writes,
// regardless of which specific check failed (unrecognized state, expired
// state cookie, token-exchange failure, unrecognized sub, wrong role) —
// mirrors I5's "401 never leaks why" applied to this surface's own error
// shape (_contract/API.md's BFF conventions: "bff returns HTML ... not
// this JSON shape"). The specific reason is logged server-side only,
// exactly like identity.Service.unauthorized's own pattern.
func renderLoginError(c *gin.Context, logger *slog.Logger, reason string) {
	if logger != nil {
		logger.Warn("bff: login failed", "reason", reason)
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusUnauthorized, "<!doctype html><html><body><h1>Login failed</h1>"+
		"<p>%s</p><p><a href=\"/login\">Try again</a></p></body></html>",
		html.EscapeString("Something went wrong signing you in. Please try again."))
}

// RequireSession gates GET / (and any future authenticated bff route) on a
// valid session cookie (task-4.md step 3, I1, I12). A missing cookie, one
// that fails signature/expiry verification, or one that resolves to a
// user that no longer exists, is inactive, or has role="agent" (I12 —
// defense in depth: this check exists independently of GET /callback's own
// I12 check, in case a session cookie somehow carried an agent's users.id)
// all redirect to /login — never a 401 JSON body, this surface serves
// HTML (_contract/API.md's BFF conventions).
func RequireSession(signer *Signer, users identity.UserRepo, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(sessionCookieName)
		if err != nil || cookie == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		userID, err := signer.ParseSessionCookie(cookie)
		if err != nil {
			if logger != nil {
				logger.Debug("bff: session cookie failed verification", "error", err)
			}
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		user, err := users.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			if logger != nil {
				logger.Debug("bff: session resolved to a missing user", "user_id", userID)
			}
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		if user.Role == "agent" {
			// I12, checked at this layer too, not only at GET /callback.
			if logger != nil {
				logger.Warn("bff: session resolved to role=agent, rejected (I12)", "user_id", userID)
			}
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		if !user.Active {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Set(actorContextKey, user)
		c.Next()
	}
}

// ActorFromContext returns the identity.User RequireSession resolved for
// this request. view_handler.go reads this instead of ever querying
// users itself (I4).
func ActorFromContext(c *gin.Context) (identity.User, bool) {
	v, ok := c.Get(actorContextKey)
	if !ok {
		return identity.User{}, false
	}
	user, ok := v.(identity.User)
	return user, ok
}
