package humamw

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"

	"github.com/mildronize/my-template/internal/identity"
)

// forbiddenActorHeader mirrors
// internal/transport/publicapi.forbiddenActorHeader. Duplicated
// deliberately, not shared: publicapi already imports this package
// (actorFromContext, middleware.go's own doc comment), so the reverse
// import this package would need to reuse publicapi's constant would be a
// cycle. This is the same duplication this file already carried for this
// exact constant pre-fix — extended below to forbiddenFieldNames for the
// identical reason, now that this middleware checks body/query too.
const forbiddenActorHeader = "X-Actor"

// forbiddenFieldNames mirrors
// internal/transport/publicapi.forbiddenFieldNames (I1) — duplicated for
// the same import-cycle reason forbiddenActorHeader already was (see that
// const's own doc comment above). internal/transport/publicapi still
// keeps its own copy: internal/transport/bff's /api/bff group (ticket 7,
// not yet migrated) mounts publicapi.RejectActorFields directly as gin
// middleware and needs it independently of this package.
var forbiddenFieldNames = map[string]struct{}{
	"actor":   {},
	"actorid": {},
	"ownerid": {},
}

// RejectActorFields is the router-agnostic huma.Middlewares equivalent of
// publicapi.RejectActorFields (I1) — checks the X-Actor header, every
// query param, and the request body for the reserved actor/actorId/
// ownerId field names, independently of huma's own request validation.
//
// Fix (story-1/ticket-6 finding, contract.md's "Middleware mapping"
// table): this used to be RejectActorHeader, covering only the X-Actor
// header — ticket 6 originally relied on huma's own
// additionalProperties:false default and
// Config.RejectUnknownQueryParameters to catch body/query actor fields
// instead of porting that half of the old gin middleware. That doesn't
// preserve I1's ordering: huma's own body/query validation runs *inside*
// the parse→validate→resolve→handle chain that every
// api.UseMiddleware — including RequireActor — wraps *entirely*
// (huma.go:881), so an actor field in the body/query would only be
// caught *after* RequireActor's DB-backed credential resolution, not
// before it — reversing the documented reason this check is registered
// ahead of RequireActor in the first place (an unauthenticated,
// shape-violating request should never spend a DB lookup on credential
// resolution). This function now performs the body/query check itself,
// exactly like publicapi.RejectActorFields/hasForbiddenBodyField did (and
// still does, for the not-yet-migrated /api/bff group), so the ordering
// guarantee holds for /api/v1 again too. Huma's additionalProperties:
// false/RejectUnknownQueryParameters defaults stay enabled underneath as
// a second-layer catch-all for non-actor unrecognized fields (typos,
// etc.) — not a conflict: this check runs first, before huma's own
// parsing ever sees the request, and only short-circuits for these three
// specific field names.
//
// Registered on both apiV1 and apiBFF via huma.API.UseMiddleware
// (contract.md describes this as "registered on both
// configV1.Middlewares and configBFF.Middlewares" — huma v2.39.1
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
func RejectActorFields() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Header(forbiddenActorHeader) != "" {
			writeError(ctx, http.StatusBadRequest,
				"request may not declare an actor — identity comes only from the resolved credential")
			return
		}

		requestURL := ctx.URL()
		for key := range requestURL.Query() {
			if _, forbidden := forbiddenFieldNames[strings.ToLower(key)]; forbidden {
				writeError(ctx, http.StatusBadRequest,
					"request may not declare an actor — identity comes only from the resolved credential")
				return
			}
		}

		if hasForbiddenBodyField(ctx) {
			writeError(ctx, http.StatusBadRequest,
				"request may not declare an actor — identity comes only from the resolved credential")
			return
		}

		next(ctx)
	}
}

// hasForbiddenBodyField peeks at ctx's request body, parsed as a generic
// map[string]json.RawMessage exactly like
// publicapi.hasForbiddenBodyField (independent of whatever typed
// request-body struct the operation this request eventually reaches
// declares), and restores the body afterward so huma's own downstream
// parsing still sees the full, unconsumed body — mirrors the gin-era
// io.NopCloser(bytes.NewReader(body)) trick from that same function.
//
// huma.Context has no body-reset method (its BodyReader() method is
// read-only, by design — docs/docs/features/middleware.md's own
// "Unwrapping" section is the documented escape hatch for exactly this
// kind of router-specific need: "if you need to access the underlying
// router-specific request and response objects, you can Unwrap() them
// using the router-specific adapter package"), so this reaches through
// humagin.Unwrap to the underlying *gin.Context and replaces
// Request.Body directly. This repo only ever attaches RejectActorFields
// to a humagin-backed huma.API (cmd/server/main.go's apiV1H/apiBFFH), so
// that adapter-specific reach-through is safe here; this package's own
// tests that exercise this function build the same humagin-backed
// harness for that reason (see newGinTestAPI in actor_test.go), rather
// than the humatest/humaflow harness this package's other tests use.
//
// A body that isn't a JSON object (missing, empty, an array, a scalar,
// malformed JSON) is not this guard's concern — it returns false and
// leaves that to whatever validates the request shape otherwise (huma's
// own schema validation, downstream).
func hasForbiddenBodyField(ctx huma.Context) bool {
	reader := ctx.BodyReader()
	if reader == nil {
		return false
	}

	body, err := io.ReadAll(reader)
	// Restore regardless of outcome — even a read error or an empty body
	// must leave Request.Body in a consumable state for huma's own
	// parsing, matching hasForbiddenBodyField's original gin behavior.
	humagin.Unwrap(ctx).Request.Body = io.NopCloser(bytes.NewReader(body))

	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return false
	}

	var generic map[string]json.RawMessage
	if err := json.Unmarshal(body, &generic); err != nil {
		return false
	}
	for key := range generic {
		if _, forbidden := forbiddenFieldNames[strings.ToLower(key)]; forbidden {
			return true
		}
	}
	return false
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
// RejectActorFields (I1, both surfaces) is wired into cmd/server/main.go's
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
