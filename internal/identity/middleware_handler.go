package identity

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// actorContextKey is the gin context key RequireActor stores the
// resolved User under. handler.go reads it via ActorFromContext — I4:
// only this middleware ever queries users/api_keys; handlers never do
// their own lookup.
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

// errorEnvelope matches _contract/API.md's error shape.
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint,omitempty"`
	} `json:"error"`
}

func newErrorEnvelope(code, message, hint string) errorEnvelope {
	var e errorEnvelope
	e.Error.Code = code
	e.Error.Message = message
	e.Error.Hint = hint
	return e
}

// unauthorizedBody is the one 401 response body RequireActor ever writes
// (I5) — a package-level value, not reconstructed per call, so there is
// no risk of it drifting between call sites as the middleware evolves.
var unauthorizedBody = newErrorEnvelope("unauthorized", "authentication required", "")

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
	c.AbortWithStatusJSON(http.StatusBadRequest, newErrorEnvelope(
		"actor_field_present",
		"request may not declare an actor — identity comes only from the resolved credential",
		"",
	))
}

// RequireActor is the actor-resolution middleware (task-2.md): it calls
// Service.ResolveActor and, on success, stores the resolved User on the
// gin context for handlers to read via ActorFromContext (I4). On any
// failure this is the middleware's one and only 401 write — every
// failure reason ResolveActor logged server-side collapses to the exact
// same response body here (I5), regardless of which check inside
// ResolveActor actually failed.
func RequireActor(svc *Service) gin.HandlerFunc {
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

// ActorFromContext returns the User RequireActor resolved for this
// request. Handlers call this instead of ever querying users/api_keys
// themselves (I4).
func ActorFromContext(c *gin.Context) (User, bool) {
	v, ok := c.Get(actorContextKey)
	if !ok {
		return User{}, false
	}
	user, ok := v.(User)
	return user, ok
}
