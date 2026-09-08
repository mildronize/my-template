package bff

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestSecureFromURL_FollowsConfiguredScheme is task-10's regression test
// (.chief/milestone-2/_plan/_todo.md): the cookie Secure attribute must be
// derived from cfg.AuthAudience's scheme, not hardcoded. This directly
// covers the bug that broke มายด์'s own first login attempt — an
// http://localhost AuthAudience (GETTING-STARTED.md's documented local-dev
// setup) must produce Secure=false, since Safari refuses to store a Secure
// cookie over plain http. A real (always-https) deployment must still get
// Secure=true, unaffected by this change.
func TestSecureFromURL_FollowsConfiguredScheme(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		secure bool
	}{
		{"http scheme is never secure", "http://localhost:8080", false},
		{"http scheme, no port", "http://localhost", false},
		{"https scheme is always secure", "https://app.example.test", true},
		{"https scheme, local-style host", "https://localhost:8443", true},
		// A URL that fails to parse outright fails safe to Secure=true
		// (the stricter attribute) rather than silently downgrading
		// cookies on a malformed config — see secureFromURL's own doc
		// comment. An empty value parses successfully with an empty
		// scheme, so it lands on the same false branch as any other
		// non-"https" scheme (never reachable in practice: both callers
		// refuse to set any cookie while configured(cfg) is false, which
		// an empty AuthAudience always implies).
		{"unparseable URL fails safe to secure", "://not-a-url", true},
		{"empty URL is not https, not secure", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := secureFromURL(tc.url)
			if got != tc.secure {
				t.Errorf("secureFromURL(%q) = %v, want %v", tc.url, got, tc.secure)
			}
		})
	}
}

// --- I1 on the BFF surface (story-1/ticket-7's own required check) ------
//
// contract.md's "Middleware mapping" table and ticket-6's own report
// document a real regression ticket-6 hit and fixed for /api/v1: relying
// on huma's automatic additionalProperties:false/RejectUnknownQueryParameters
// defaults alone to reject a body/query actor field moves that rejection
// from *before* auth to *after* it, since huma's own request validation
// runs *inside* the parse->validate->resolve->handle chain every
// api.UseMiddleware (including humamw.RequireSession) wraps entirely. The
// fix — humamw.RejectActorFields, checking header+query+body pre-auth —
// is already attached to apiBFFH (cmd/server/main.go's newHumaAPIs,
// shared with apiV1H), so this file doesn't re-solve anything; it exists
// only to verify (ticket-7's own explicit instruction) that the shared
// middleware's ordering guarantee actually holds for this surface too,
// the same way internal/transport/publicapi/middleware_test.go's own
// TestI1_RejectActorFields_BodyField/QueryParam/XActorHeader verified it
// for /api/v1 after ticket-6's fix.

// echoBody/echoInput/echoOutput/registerEchoOperation mirror
// internal/transport/publicapi/middleware_test.go's own pieces of the
// same name — a minimal synthetic operation standing in for "any real
// operation on this surface," so these tests don't need to satisfy a
// real domain operation's own body-shape requirements (CreateTodoRequest's
// required title/clientRequestId, etc.) just to prove the pre-auth
// ordering property.
type echoBody struct {
	Title string `json:"title,omitempty"`
}

type echoInput struct{ Body echoBody }
type echoOutput struct{}

// registerEchoOperation registers POST /api/bff/echo. DefaultStatus is
// pinned to 200 (huma's own no-Body-field default is 204) so
// TestI1_RejectActorFields_AllowsCleanRequest_BFF's assertion on it is
// unambiguous.
func registerEchoOperation(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "bffEcho",
		Method:        http.MethodPost,
		Path:          "/api/bff/echo",
		DefaultStatus: http.StatusOK,
	}, func(_ context.Context, _ *echoInput) (*echoOutput, error) {
		return &echoOutput{}, nil
	})
}

// newMiddlewareTestRouter mirrors newTestRouter's own composition
// (bff_testutil_test.go/newHumaBFFAPI: humamw.RejectActorFields then
// humamw.RequireSession, the same order cmd/server/main.go's wireBFF
// attaches to apiBFFH) but registers only the synthetic echo operation
// above, no login/callback routes and no database-backed signer/users —
// every test below exercises the pre-auth I1 guard, which by design runs
// before RequireSession's own session-cookie lookup, so no session (and
// no real *identity.Repo) ever needs to be presented to reach it. users
// is nil for the same reason RequireSession's own signer/users are never
// dereferenced on this path.
func newMiddlewareTestRouter() *gin.Engine {
	router := gin.New()
	signer := NewSigner([]byte("test-middleware-secret-not-for-production"))
	api := newHumaBFFAPI(router, signer, nil)
	registerEchoOperation(api)
	return router
}

func doMiddlewareRequest(router *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestI1_RejectActorFields_BodyField_BFF mirrors internal/transport/
// publicapi's own TestI1_RejectActorFields_BodyField exactly — a request
// body naming an actor is rejected pre-auth, no session required to reach
// the check.
func TestI1_RejectActorFields_BodyField_BFF(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ActorID", "ownerId", "OwnerId"} {
		t.Run(field, func(t *testing.T) {
			router := newMiddlewareTestRouter()

			body := `{"` + field + `":"someone-else","title":"a todo"}`
			rec := doMiddlewareRequest(router, http.MethodPost, "/api/bff/echo", body, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "actor_field_present")
		})
	}
}

// TestI1_RejectActorFields_QueryParam_BFF mirrors internal/transport/
// publicapi's own TestI1_RejectActorFields_QueryParam.
func TestI1_RejectActorFields_QueryParam_BFF(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ownerId"} {
		t.Run(field, func(t *testing.T) {
			router := newMiddlewareTestRouter()

			rec := doMiddlewareRequest(router, http.MethodPost, "/api/bff/echo?"+field+"=someone-else", `{}`, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "actor_field_present")
		})
	}
}

// TestI1_RejectActorFields_XActorHeader_BFF mirrors internal/transport/
// publicapi's own TestI1_RejectActorFields_XActorHeader — the X-Actor
// header check that was never at risk of the body/query ordering
// regression (it was already pre-auth before ticket-6's fix), verified
// here too for completeness against this surface's own wiring.
func TestI1_RejectActorFields_XActorHeader_BFF(t *testing.T) {
	router := newMiddlewareTestRouter()

	rec := doMiddlewareRequest(router, http.MethodPost, "/api/bff/echo", `{}`, map[string]string{"X-Actor": "someone-else"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "actor_field_present")
}

// TestI1_RejectActorFields_AllowsCleanRequest_UnauthenticatedStillReaches401_BFF
// is the ordering half of the positive counterpart, against the synthetic
// echo router: a clean request (no actor-shaped field) with no session
// cookie either must fail at RequireSession (401 unauthorized) — never at
// RejectActorFields (400 actor_field_present) — proving I1 doesn't
// misfire on ordinary input and correctly lets a clean-but-unauthenticated
// request proceed exactly one more step, into session resolution.
func TestI1_RejectActorFields_AllowsCleanRequest_UnauthenticatedStillReaches401_BFF(t *testing.T) {
	router := newMiddlewareTestRouter()

	rec := doMiddlewareRequest(router, http.MethodPost, "/api/bff/echo", `{"title":"a todo"}`, nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "a clean request with no session must fail at RequireSession (401), not be misidentified as an actor-field violation (400)")
	assert.NotContains(t, rec.Body.String(), "actor_field_present")
}

// TestI1_RejectActorFields_AllowsCleanRequest_FullSuccess_BFF is this
// same property's full end-to-end proof — mirrors internal/transport/
// publicapi's own TestI1_RejectActorFields_AllowsCleanRequest (which
// presents a valid Authorization header to prove the whole chain, not
// just the ordering) — against a real, DB-backed router and a genuinely
// valid session cookie (newBFFRouterForOwner, this package's own
// standard fixture), hitting POST /api/bff/todos with a clean body: the
// request must reach the handler and succeed (201), overlapping with but
// independent of TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie's own
// broader CRUD proof.
func TestI1_RejectActorFields_AllowsCleanRequest_FullSuccess_BFF(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title":           "a clean todo",
		"clientRequestId": "i1-clean-request-1",
	})

	assert.Equal(t, http.StatusCreated, rec.Code, "a clean, authenticated request with no actor field must not be rejected by I1's guard")
}
