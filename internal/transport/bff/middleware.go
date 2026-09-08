package bff

import (
	"context"
	"html"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/transport/humamw"
)

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

// --- huma-era shared pieces (ticket-7) --------------------------------
//
// story-1/ticket-7: this file used to also hold actorContextKey and
// ActorFromContext(c *gin.Context) — the gin-context actor lookup
// json_middleware.go's own RequireJSONSession populated via c.Set. Both
// are gone now: /api/bff runs on huma (cmd/server/main.go's apiBFFH),
// whose own session resolution is internal/transport/humamw's
// router-agnostic RequireSession (ticket 5), attached once via
// apiBFFH.UseMiddleware(...) rather than per-package gin middleware.
// json_middleware.go itself is deleted (its one production caller, the
// /api/bff gin.RouterGroup, no longer exists — every handler that used to
// sit behind it is now a huma.Register operation on apiBFFH instead).
// This section mirrors internal/transport/publicapi/middleware.go's own
// "huma-era shared pieces" section exactly — same shapes, same reasoning,
// deliberately duplicated rather than shared (ARCHITECTURE.md's "no
// transport/shared package" decision: each transport surface owns this
// small amount of duplication).

// unauthorizedMessage is the one 401 message this package's own huma
// operations ever return via actorFromContext's defensive branch below
// (I5 — "401 never leaks why" — mirrors humamw.unauthorizedMessage's own
// identical text, duplicated here rather than exported from humamw since
// that package deliberately keeps no bff-specific surface).
const unauthorizedMessage = "authentication required"

// actorFromContext reads the actor apiBFFH's attached humamw.RequireSession
// middleware already resolved onto ctx (humamw.ActorFromContext) — every
// huma operation in this package that needs to know who's calling uses
// this instead of querying users itself (I4), mirroring the old
// ActorFromContext(c *gin.Context)'s role before ticket-7 (and
// publicapi.actorFromContext's own identical role for /api/v1). The error
// branch is defensive, mirroring bffOwnerID's old comment: unreachable
// given main.go's intended middleware order (humamw.RejectActorFields,
// humamw.RequireSession, then the operation handler), and only here in
// case an operation is ever registered on a huma.API instance without
// that chain attached.
func actorFromContext(ctx context.Context) (identity.User, error) {
	user, ok := humamw.ActorFromContext(ctx)
	if !ok {
		return identity.User{}, huma.Error401Unauthorized(unauthorizedMessage)
	}
	return user, nil
}

// apiError is this package's own huma.StatusError — used for the 400
// validation_error responses this package's own operation code raises
// directly (todo_handler.go's per-`type` dispatch/cross-field checks and
// its cursor-pair check, which CreateTodoEventRequest's/ListActivityInput's
// own schemas deliberately leave to handler code), as opposed to a huma
// request-validation failure (422, huma's own default for a missing/
// extra/wrong-typed field, contract.md's "Behavior note") or a
// domain-layer error mapped via huma.Error404NotFound/Error403Forbidden
// (whose status codes are unambiguous under humamw.InstallErrorEnvelope's
// codeForStatus). Mirrors internal/transport/publicapi's own apiError
// exactly, including the same reason for bypassing huma.NewError/humamw's
// process-wide override — see that type's own doc comment.
type apiError struct {
	status int
	Err    struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint,omitempty"`
	} `json:"error"`
}

func (e *apiError) Error() string  { return e.Err.Message }
func (e *apiError) GetStatus() int { return e.status }

// newAPIError builds an apiError. status is caller-chosen deliberately
// (not fixed to 400) so a future non-ambiguous status this package might
// need can also use it without adding a second near-identical type.
func newAPIError(status int, code, message, hint string) *apiError {
	e := &apiError{status: status}
	e.Err.Code = code
	e.Err.Message = message
	e.Err.Hint = hint
	return e
}

// newValidationError builds a 400 validation_error apiError — this
// package's own handler-level rejection code, distinct from huma's
// automatic schema validation (422, see apiError's own doc comment).
func newValidationError(message, hint string) *apiError {
	return newAPIError(http.StatusBadRequest, "validation_error", message, hint)
}

// internalServerError is this package's one 500 response, shared by every
// operation in todo_handler.go/keys_handler.go/users_handler.go — one
// source of the message text rather than the same literal repeated at
// every unexpected-failure branch (mirrors publicapi's own
// internalServerError).
func internalServerError() error {
	return huma.Error500InternalServerError("internal error")
}
