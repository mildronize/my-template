// Package publicapi is the REST transport surface for agents/skills,
// key-authenticated (_contract/API.md) — the public API distinct from the
// owner-facing internal/transport/bff surface. It holds every HTTP-facing
// piece for both the todo domain and identity: the huma operation
// registrations (todo_handler.go, me_handler.go, keys_handler.go,
// story-1/ticket-6) and, in this file, the pieces every one of those still
// shares plus internal/transport/bff's own /api/bff group middleware
// (ARCHITECTURE.md: "Why transport is not inside a domain module
// anymore"). No domain module or internal/identity may import this
// package back (ARCHITECTURE.md rule 4) — dependencies point one way,
// from here down into internal/domain/* and internal/identity, never the
// reverse.
//
// story-1/ticket-6 (huma migration, /api/v1 half): this file used to also
// hold RequireActor/ActorFromContext(*gin.Context) — the actor-resolution
// middleware /api/v1 mounted directly on its gin.RouterGroup. That half
// is gone from here now: /api/v1 runs on huma (cmd/server/main.go's
// apiV1H), whose own actor resolution is internal/transport/humamw's
// router-agnostic RequireActor (ticket 5), attached once via
// apiV1H.UseMiddleware(...) rather than per-package gin middleware. This
// file's own actorFromContext (below) reads what that middleware resolved
// via humamw.ActorFromContext, mirroring the old ActorFromContext(c)'s
// role for every operation in this package (I4: only middleware ever
// resolves identity, handlers never do). RejectActorFields itself is
// UNCHANGED and still lives here: internal/transport/bff's own /api/bff
// group (cmd/server/main.go's wireBFF, ticket 7's scope) still mounts it
// directly as gin middleware — it has no huma-specific state, so there is
// nothing ticket-6-specific to port for that half of I1 yet.
//
// This doc comment lives here, not on a per-domain handler file like
// todo_handler.go, deliberately (task-9, Blocker B): every file that
// registers a `<Domain>` huma operation set is expected to be deleted
// whole on fork (docs/GETTING-STARTED.md Step 8), and Go's package doc
// convention reads from whichever file happens to declare it — this file
// is the one that survives that deletion.
package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/transport/humamw"
)

// forbiddenFieldNames mirrors my-task's actor-guard.ts
// FORBIDDEN_FIELD_NAMES (I1) — a request declaring any of these, in the
// body or the query string, is a 400, not a value that gets silently
// read and ignored. Still bespoke, gin-based logic — used only by
// internal/transport/bff's own /api/bff group now (see this file's own
// doc comment) — because /api/v1's equivalent protection is huma's own
// additionalProperties: false / RejectUnknownQueryParameters default,
// per contract.md's "Middleware mapping" table (ticket-6).
var forbiddenFieldNames = map[string]struct{}{
	"actor":   {},
	"actorid": {},
	"ownerid": {},
}

const forbiddenActorHeader = "X-Actor"

// ErrorEnvelope matches _contract/API.md's error shape. Exported so
// internal/transport/bff can reuse this exact type/constructor for its
// own hand-written error responses instead of redefining an equivalent
// shape in a second place — _contract/API.md's BFF section is explicit
// that bff-openapi.yaml "reuses publicapi's envelope," not a second
// envelope kept in sync by hand. Unaffected by ticket-6's huma migration:
// /api/v1's own errors now go through humamw's ErrorEnvelope (an
// intentionally distinct type, per that package's own doc comment) or
// this package's own apiError (todo_handler.go/keys_handler.go/
// me_handler.go), but bff's hand-rolled responses still need this one.
type ErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint,omitempty"`
	} `json:"error"`
}

// NewErrorEnvelope builds an ErrorEnvelope. Exported alongside
// ErrorEnvelope itself, for the same reason.
func NewErrorEnvelope(code, message, hint string) ErrorEnvelope {
	var e ErrorEnvelope
	e.Error.Code = code
	e.Error.Message = message
	e.Error.Hint = hint
	return e
}

// RejectActorFields is I1's standalone request-shape guard — a request
// declaring an actor is a 400 request-shape problem, not a 401 credential
// problem. Register it on a gin group *before* any credential-resolution
// middleware, so a shape violation never spends a DB lookup on credential
// resolution. Only internal/transport/bff's /api/bff group uses this now
// (ticket-6's own doc comment above) — /api/v1 moved to huma's own
// additionalProperties:false/RejectUnknownQueryParameters defaults plus
// humamw.RejectActorHeader for the one piece (X-Actor) that stays
// bespoke under huma too.
//
// It checks body keys, query params, and the X-Actor header itself,
// independently of whatever gin binding or `additionalProperties: false`
// does — that property has to be remembered on every request schema, and
// a forked service that adds an endpoint without it would otherwise
// silently lose this protection.
func RejectActorFields() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader(forbiddenActorHeader) != "" {
			abortActorFieldPresent(c)
			return
		}

		for key := range c.Request.URL.Query() {
			if _, forbidden := forbiddenFieldNames[strings.ToLower(key)]; forbidden {
				abortActorFieldPresent(c)
				return
			}
		}

		if hasForbiddenBodyField(c) {
			abortActorFieldPresent(c)
			return
		}

		c.Next()
	}
}

// hasForbiddenBodyField parses the request body as a generic
// map[string]json.RawMessage (independent of any typed request binding)
// and restores the body afterwards so the real handler can still read
// it. A body that isn't a JSON object (missing, empty, an array, a
// scalar, malformed JSON) is not this guard's concern — it returns false
// and leaves that to whatever validates the request shape otherwise.
func hasForbiddenBodyField(c *gin.Context) bool {
	if c.Request.Body == nil {
		return false
	}
	body, err := io.ReadAll(c.Request.Body)
	c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
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

func abortActorFieldPresent(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusBadRequest, NewErrorEnvelope(
		"actor_field_present",
		"request may not declare an actor — identity comes only from the resolved credential",
		"",
	))
}

// --- huma-era shared pieces (ticket-6) --------------------------------

// unauthorizedMessage is the one 401 message this package's own huma
// operations ever return via actorFromContext's defensive branch below
// (I5 — "401 never leaks why" — mirrors humamw.unauthorizedMessage's own
// identical text, duplicated here rather than exported from humamw since
// that package deliberately keeps no publicapi-specific surface).
const unauthorizedMessage = "authentication required"

// actorFromContext reads the actor apiV1H's attached humamw.RequireActor
// middleware already resolved onto ctx (humamw.ActorFromContext) —
// every huma operation in this package that needs to know who's calling
// uses this instead of querying users/api_keys itself (I4), mirroring the
// old ActorFromContext(c *gin.Context)'s role before ticket-6. The error
// branch is defensive, mirroring handleMe's old comment: unreachable
// given main.go's intended middleware order (humamw.RejectActorHeader,
// humamw.RequireActor, then the operation handler), and only here in
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
// directly (todo_handler.go's per-`type` dispatch/cross-field checks,
// which CreateTodoEventRequest's own schema deliberately leaves to
// handler code — see that type's doc comment), as opposed to a huma
// request-validation failure (422, huma's own default for a missing/
// extra/wrong-typed field, contract.md's "Behavior note") or a
// domain-layer error mapped via huma.Error404NotFound/Error403Forbidden
// (whose status codes are unambiguous under humamw.InstallErrorEnvelope's
// codeForStatus).
//
// Deliberately bypasses huma.NewError/humamw's process-wide override:
// humamw/error.go's own codeForStatus doc comment says "after this
// migration, 400 is used by exactly one path (the X-Actor header
// guard)" — routing a handler-level validation_error at 400 through
// NewError would silently mislabel it "actor_field_present" instead,
// since codeForStatus keys purely off the numeric status, not the
// call site. This type satisfies huma.StatusError directly instead
// (huma.go's own returned-error handling: `errors.As(err, &se)` writes
// se's own JSON verbatim, never calling NewError at all) — the same
// {error:{code,message,hint}} shape every other error on this service
// uses, built by hand, matching huma's own documented "Custom Errors"
// extension pattern (docs/docs/features/response-errors.md).
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

// internalServerError is this package's one 500 response, shared by
// every operation in todo_handler.go/keys_handler.go — one source of the
// message text rather than the same literal repeated at every
// unexpected-failure branch (code-review, story-1/ticket-6: Duplicated
// Code, minor).
func internalServerError() error {
	return huma.Error500InternalServerError("internal error")
}
