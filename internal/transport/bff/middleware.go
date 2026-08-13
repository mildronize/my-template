package bff

import (
	"html"
	"log/slog"
	"net/http"
	"net/url"

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

// secureFromURL reports whether cookies should carry the Secure attribute
// for a service reachable at rawURL — true when rawURL's scheme is
// "https", false when it's "http". This is task-10's fix (see
// .chief/milestone-2/_plan/_todo.md): Secure is derived from
// cfg.AuthAudience, the same value that already has to be correct for
// OAuth redirect matching to work at all (oauth.go's redirectURL), rather
// than a separately-settable flag someone could ship in the wrong
// position. `http://localhost` (GETTING-STARTED.md's documented local-dev
// setup) now correctly gets a non-Secure cookie — Safari refuses to store
// a Secure cookie over plain http, which is exactly the bug this fixes —
// while any real (always-https) deployment is unaffected.
//
// A rawURL that fails to parse (e.g. a missing scheme separator) fails
// safe to Secure=true, the stricter attribute, rather than silently
// downgrading to a non-Secure cookie on a malformed config. An empty or
// schemeless rawURL parses successfully with an empty Scheme, so it falls
// through to the false branch below like any other non-"https" value —
// in practice this never reaches a real request anyway, since both
// callers (NewLoginHandler, NewCallbackHandler) already refuse to set any
// cookie at all while configured(cfg) is false, which an empty
// cfg.AuthAudience always implies.
func secureFromURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	return u.Scheme == "https"
}

// setCookie centralizes the cookie attributes task-4.md's login-flow spec
// calls for on both cookies this package sets: HttpOnly (never readable
// from JS), Secure (per secureFromURL above), SameSite=Lax. path "/" so
// the cookie is sent back on every route this package registers, not just
// the one that set it.
func setCookie(c *gin.Context, name, value string, maxAge int, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, "/", "", secure, true)
}

// clearCookie deletes a cookie this package previously set (maxAge<0).
func clearCookie(c *gin.Context, name string, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, "", -1, "/", "", secure, true)
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

// ActorFromContext returns the identity.User RequireJSONSession
// (json_middleware.go) resolved for this request — every handler in this
// package (me_handler.go, todo_handler.go, keys_handler.go) reads it
// through this one function instead of ever querying users itself (I4).
func ActorFromContext(c *gin.Context) (identity.User, bool) {
	v, ok := c.Get(actorContextKey)
	if !ok {
		return identity.User{}, false
	}
	user, ok := v.(identity.User)
	return user, ok
}
