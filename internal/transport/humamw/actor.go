package humamw

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mildronize/my-template/internal/identity"
)

// forbiddenActorHeader mirrors
// internal/transport/publicapi.forbiddenActorHeader — the one piece of
// I1's RejectActorFields that stays bespoke under huma (contract.md's
// middleware-mapping table: body/query rejection becomes free via huma's
// own additionalProperties:false default and
// Config.RejectUnknownQueryParameters, but nothing in huma's request
// validation knows about a header this repo has specifically reserved for
// identity).
const forbiddenActorHeader = "X-Actor"

// RejectActorHeader is the router-agnostic huma.Middlewares equivalent of
// publicapi.RejectActorFields' X-Actor check (I1) — the ticket-5 port of
// the one piece of that gin middleware that isn't obsoleted by huma
// defaults. Registered on both apiV1 and apiBFF via
// huma.API.UseMiddleware (contract.md describes this as "registered on
// both configV1.Middlewares and configBFF.Middlewares" — huma v2.39.1
// actually has no such Config field; API-wide middleware is added after
// construction via api.UseMiddleware(...), per
// docs/docs/features/middleware.md's "Router-agnostic" section. Same
// intent — I1 applies to both surfaces, per INVARIANTS.md — reached
// through the API huma v2.39.1 actually exposes), so it must not depend
// on which surface it's running on.
//
// Per contract.md's citation of huma.go:881, huma wraps the *entire*
// parse→validate→resolve→handle chain in
// api.Middlewares().Handler(...) — so this check still runs before body
// parsing, preserving today's "fail on shape before spending a DB lookup"
// ordering even though request-body parsing itself is now huma's job
// rather than a step this middleware performs by hand.
func RejectActorHeader() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Header(forbiddenActorHeader) != "" {
			writeError(ctx, http.StatusBadRequest,
				"request may not declare an actor — identity comes only from the resolved credential")
			return
		}
		next(ctx)
	}
}

// actorContextKey is a private type (not a bare string, unlike gin's own
// c.Set/c.Get keying by string) so a value stored here can never collide
// with an unrelated context.WithValue key from elsewhere in the
// process — the standard Go guidance for context keys, and worth
// following here since huma handlers receive a plain context.Context
// (huma.go:779's Register[I, O any](..., handler func(context.Context, *I)
// (*O, error))), not a package-scoped gin.Context these keys never leave.
type actorContextKey struct{}

// ActorFromContext returns the identity.User RequireActor or
// RequireSession (below) resolved for this request — the huma-side
// equivalent of publicapi.ActorFromContext(c *gin.Context) and
// bff.ActorFromContext(c *gin.Context), ported to the plain
// context.Context a huma operation handler actually receives. Not a
// replacement for either of those two functions in this ticket — both
// stay exactly as they are, since nothing under internal/transport/
// publicapi or internal/transport/bff moves onto huma until tickets 6/7;
// this is new code, addressing new (not-yet-wired) call sites only.
func ActorFromContext(ctx context.Context) (identity.User, bool) {
	user, ok := ctx.Value(actorContextKey{}).(identity.User)
	return user, ok
}

// withActor stores user on ctx's underlying context.Context via
// huma.WithValue, which propagates the value to the real request context
// (and so to ActorFromContext, later middleware, and the operation
// handler) — mirrors gin's c.Set(actorContextKey, user) in
// publicapi/middleware.go's RequireActor, but through huma's own
// context-propagation seam (docs/docs/features/middleware.md's "Context
// Values" section) rather than a gin-specific one.
func withActor(ctx huma.Context, user identity.User) huma.Context {
	return huma.WithValue(ctx, actorContextKey{}, user)
}

// unauthorizedMessage is the one 401 message both RequireActor and
// RequireSession ever write (I5 — "401 never leaks why" — mirrors
// publicapi.unauthorizedBody/bff.jsonUnauthorizedBody's identical
// reasoning): the specific check that failed is for server-side logs
// only, never the response body.
const unauthorizedMessage = "authentication required"

// RequireActor is the router-agnostic port of
// publicapi.RequireActor (Bearer credential, public API) — same
// identity.Service.ResolveActor call, same single 401 write site
// regardless of which check inside ResolveActor failed (I5), now storing
// the resolved actor via withActor/ActorFromContext instead of a gin
// context key.
//
// Not yet registered on apiV1's huma.Config in this ticket — see this
// package's doc comment: it's shared plumbing, "built and verified in
// isolation before any real route moves onto it" (ticket-5), ready for
// ticket 6 to attach once apiV1 actually serves real operations. Only
// RejectActorHeader (I1, both surfaces) is wired into cmd/server/main.go's
// new huma.API instances this ticket — attaching an actor-resolution
// middleware ahead of any operation that would need it has no observable
// effect yet and nothing to verify it against beyond this package's own
// unit tests.
func RequireActor(svc *identity.Service) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		user, err := svc.ResolveActor(ctx.Context(), ctx.Header("Authorization"))
		if err != nil {
			writeError(ctx, http.StatusUnauthorized, unauthorizedMessage)
			return
		}
		next(withActor(ctx, user))
	}
}

// SessionCookieReader abstracts the one bff.Signer method RequireSession
// needs (ParseSessionCookie) — declared here, not imported as a concrete
// *bff.Signer type, so this package doesn't have to import
// internal/transport/bff just to spell out a parameter type (bff.Signer
// already satisfies this interface structurally; production callers, once
// tickets 6/7 wire this middleware for real, pass their existing
// *bff.Signer straight through).
type SessionCookieReader interface {
	ParseSessionCookie(value string) (userID string, err error)
}

// RequireSession is the router-agnostic port of
// bff.RequireJSONSession (session cookie, BFF) — the "BFF
// session-equivalent" ticket-5 asks for alongside RequireActor. Mirrors
// that function's checks in the same order (cookie present → signature/
// expiry via signer.ParseSessionCookie → user lookup → I12's role check →
// active check), collapsing every failure to the same 401
// (unauthorizedMessage, I5) the same way bff.RequireJSONSession does via
// jsonUnauthorizedBody.
//
// cookieName is passed in by the caller (ticket 6/7's real wiring, not
// this ticket) rather than hard-coded here or imported from
// internal/transport/bff — bff.sessionCookieName is unexported, and
// duplicating its literal value ("session") here would be a second source
// of truth for something bff/session.go already owns; a parameter avoids
// both problems without needing to export anything from bff in this
// foundational ticket, which doesn't wire this middleware onto a real
// huma.API config at all yet (see RequireActor's own doc comment above
// for why).
func RequireSession(signer SessionCookieReader, cookieName string, users identity.UserRepo) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		cookie, err := huma.ReadCookie(ctx, cookieName)
		if err != nil || cookie.Value == "" {
			writeError(ctx, http.StatusUnauthorized, unauthorizedMessage)
			return
		}

		userID, err := signer.ParseSessionCookie(cookie.Value)
		if err != nil {
			writeError(ctx, http.StatusUnauthorized, unauthorizedMessage)
			return
		}

		user, err := users.GetUserByID(ctx.Context(), userID)
		if err != nil {
			writeError(ctx, http.StatusUnauthorized, unauthorizedMessage)
			return
		}
		if user.Role == "agent" {
			// I12: a BFF session never resolves to role='agent'.
			writeError(ctx, http.StatusUnauthorized, unauthorizedMessage)
			return
		}
		if !user.Active {
			writeError(ctx, http.StatusUnauthorized, unauthorizedMessage)
			return
		}

		next(withActor(ctx, user))
	}
}
