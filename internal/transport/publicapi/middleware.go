// Package publicapi is the REST transport surface for agents/skills,
// key-authenticated (_contract/API.md) — the public API distinct from the
// owner-facing internal/transport/bff surface a later task adds. It holds
// every HTTP-facing piece for both the todo domain and identity: the
// generated-interface adapters (todo_handler.go, me_handler.go,
// keys_handler.go) and the actor-resolution middleware (this file, moved
// here from internal/identity's old handler.go/middleware_handler.go —
// ARCHITECTURE.md: "Why transport is not inside a domain module
// anymore"). No domain module or internal/identity may import this
// package back (ARCHITECTURE.md rule 4) — dependencies point one way,
// from here down into internal/domain/* and internal/identity, never the
// reverse.
//
// This doc comment lives here, not on a per-domain handler file like
// todo_handler.go, deliberately (task-9, Blocker B): every file that
// implements a `<Domain>Server` is expected to be deleted whole on fork
// (docs/GETTING-STARTED.md Step 8), and Go's package doc convention reads
// from whichever file happens to declare it — this file is the one that
// survives that deletion.
package publicapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/identity"
)

// newAPIError builds an internal/api.Error-shaped response body — the
// generated type the openapi-validated handlers (todo_handler.go and any
// domain handler modeled on it) write directly, as opposed to this file's
// own hand-rolled ErrorEnvelope. Shared here (task-9, Blocker B/C) so a
// fork copying a domain handler file into this package never redeclares
// it — it previously lived inside todo_handler.go, a file Step 8 of
// docs/GETTING-STARTED.md deletes on fork, and which a same-package copy
// (e.g. quote_handler.go) would otherwise redeclare.
func newAPIError(code, message string) api.Error {
	e := api.Error{}
	e.Error.Code = code
	e.Error.Message = message
	return e
}

// actorID reads the actor RequireActor (this file) already resolved onto
// the gin context (ActorFromContext) and returns its id — every handler in
// this package that needs to know who's calling uses this instead of
// querying users/api_keys itself (I4), and never looks a row up by id
// alone without also knowing whose it must be (I3). The !ok branch is
// defensive, mirroring handleMe: it should be unreachable given the
// intended middleware order (RejectActorFields, RequireActor, then the
// handler), and is here only in case a route is ever wired without that
// chain. Shared here for the same reason as newAPIError above — it
// previously lived inside todo_handler.go, a file Step 8 deletes on fork.
func actorID(c *gin.Context) (string, bool) {
	user, ok := ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
		return "", false
	}
	return user.ID, true
}

// actorContextKey is the gin context key RequireActor stores the
// resolved identity.User under. todo_handler.go/me_handler.go/
// keys_handler.go read it via ActorFromContext — I4: only this middleware
// ever queries users/api_keys, handlers never do their own lookup.
const actorContextKey = "identity.actor"

// forbiddenFieldNames mirrors my-task's actor-guard.ts
// FORBIDDEN_FIELD_NAMES (I1) — a request declaring any of these, in the
// body or the query string, is a 400, not a value that gets silently
// read and ignored.
var forbiddenFieldNames = map[string]struct{}{
	"actor":   {},
	"actorid": {},
	"ownerid": {},
}

const forbiddenActorHeader = "X-Actor"

// ErrorEnvelope matches _contract/API.md's error shape. Exported
// (milestone-3/task-2) so internal/transport/bff can reuse this exact
// type/constructor for its own hand-written error responses instead of
// redefining an equivalent shape in a second place — _contract/API.md's
// BFF section is explicit that bff-openapi.yaml "reuses publicapi's
// envelope," not a second envelope kept in sync by hand. Both surfaces'
// generated-interface glue (this package's own todo_handler.go/
// keys_handler.go's use of internal/api.Error for openapi-shaped
// responses, and internal/bffapi's own Error for its validator) already
// coexist with this hand-rolled type today, producing the identical JSON
// shape via a different Go type for the same reason — this export doesn't
// change that pattern, it just makes the hand-written half of it
// available to a second transport package.
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

// unauthorizedBody is the one 401 response body RequireActor ever writes
// (I5) — a package-level value, not reconstructed per call, so there is
// no risk of it drifting between call sites as the middleware evolves.
var unauthorizedBody = NewErrorEnvelope("unauthorized", "authentication required", "")

// RejectActorFields is I1's standalone request-shape guard (task-2.md —
// deliberately a separate middleware, not folded into actor resolution:
// a request declaring an actor is a 400 request-shape problem, not a 401
// credential problem). Register it on the /api/v1 group *before*
// RequireActor, so a shape violation never spends a DB lookup on
// credential resolution.
//
// It checks body keys, query params, and the X-Actor header itself,
// independently of whatever oapi-codegen's generated binding or
// `additionalProperties: false` does — that property has to be
// remembered on every request schema, and a forked service that adds an
// endpoint without it would otherwise silently lose this protection.
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

// RequireActor is the actor-resolution middleware (task-2.md): it calls
// identity.Service.ResolveActor and, on success, stores the resolved
// identity.User on the gin context for handlers to read via
// ActorFromContext (I4). On any failure this is the middleware's one and
// only 401 write — every failure reason ResolveActor logged server-side
// collapses to the exact same response body here (I5), regardless of
// which check inside ResolveActor actually failed.
func RequireActor(svc *identity.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := svc.ResolveActor(c.Request.Context(), c.GetHeader("Authorization"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
			return
		}
		c.Set(actorContextKey, user)
		c.Next()
	}
}

// ActorFromContext returns the identity.User RequireActor resolved for
// this request. Handlers call this instead of ever querying
// users/api_keys themselves (I4).
func ActorFromContext(c *gin.Context) (identity.User, bool) {
	v, ok := c.Get(actorContextKey)
	if !ok {
		return identity.User{}, false
	}
	user, ok := v.(identity.User)
	return user, ok
}
