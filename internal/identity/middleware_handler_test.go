package identity

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestRouter(svc *Service) *gin.Engine {
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(RejectActorFields(), RequireActor(svc))
	group.GET("/me", handleMe)
	group.POST("/echo", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func doRequest(router *gin.Engine, method, path string, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// --- I1: RejectActorFields --------------------------------------------

// TestI1_RejectActorFields_BodyField — I1: a request body naming an
// actor is a 400 (actor_field_present), not a value silently read and
// ignored.
func TestI1_RejectActorFields_BodyField(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ActorID", "ownerId", "OwnerId"} {
		t.Run(field, func(t *testing.T) {
			svc := newTestService(newFakeUserRepo(), newFakeAPIKeyRepo(), nil, time.Now())
			router := newTestRouter(svc)

			body := `{"` + field + `":"someone-else","title":"a todo"}`
			rec := doRequest(router, http.MethodPost, "/api/v1/echo", body, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "actor_field_present")
		})
	}
}

func TestI1_RejectActorFields_QueryParam(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ownerId"} {
		t.Run(field, func(t *testing.T) {
			svc := newTestService(newFakeUserRepo(), newFakeAPIKeyRepo(), nil, time.Now())
			router := newTestRouter(svc)

			rec := doRequest(router, http.MethodPost, "/api/v1/echo?"+field+"=someone-else", `{}`, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "actor_field_present")
		})
	}
}

// TestI1_RejectActorFields_XActorHeader — the X-Actor header can't be
// delegated to OpenAPI's additionalProperties: false the way body fields
// can (task-2.md), so it gets its own explicit check.
func TestI1_RejectActorFields_XActorHeader(t *testing.T) {
	svc := newTestService(newFakeUserRepo(), newFakeAPIKeyRepo(), nil, time.Now())
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodPost, "/api/v1/echo", `{}`, map[string]string{"X-Actor": "someone-else"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "actor_field_present")
}

func TestI1_RejectActorFields_AllowsCleanRequest(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	agent := User{ID: "u1", Handle: "agent-a", Role: "agent", Active: true}
	users.put(agent)
	now := time.Now()
	putAPIKey(keys, "tpl_cleanrequestclean1", agent.ID, now.Add(time.Hour), nil)

	svc := newTestService(users, keys, nil, now)
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodPost, "/api/v1/echo", `{"title":"a todo"}`,
		map[string]string{"Authorization": "Bearer tpl_cleanrequestclean1"})

	assert.Equal(t, http.StatusOK, rec.Code, "a request with no actor field must not be rejected by I1's guard")
}

// --- I5: identical 401 body regardless of failure reason ----------------

// TestI5_UnauthorizedResponseBodyIdenticalAcrossFailureReasons — I5: 401
// never leaks why. Missing credential, malformed credential, and the I2
// owner-rejection all produce byte-identical response bodies; the actual
// reason is only ever visible in server-side logs.
func TestI5_UnauthorizedResponseBodyIdenticalAcrossFailureReasons(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	owner := User{ID: "owner-1", Handle: "owner", Role: "owner", Active: true}
	users.put(owner)
	inactive := User{ID: "u2", Handle: "inactive-agent", Role: "agent", Active: false}
	users.put(inactive)
	now := time.Now()
	putAPIKey(keys, "tpl_ownerkeyownerkey1", owner.ID, now.Add(time.Hour), nil)
	putAPIKey(keys, "tpl_expiredexpired123", "does-not-matter", now.Add(-time.Minute), nil)
	putAPIKey(keys, "tpl_inactiveinactive1", inactive.ID, now.Add(time.Hour), nil)

	svc := newTestService(users, keys, nil, now)
	router := newTestRouter(svc)

	cases := map[string]map[string]string{
		"missing credential":    nil,
		"malformed credential":  {"Authorization": "Basic garbage"},
		"unknown bearer token":  {"Authorization": "Bearer tpl_unknowntoken00000"},
		"expired api key":       {"Authorization": "Bearer tpl_expiredexpired123"},
		"owner-role api key":    {"Authorization": "Bearer tpl_ownerkeyownerkey1"},
		"inactive-user api key": {"Authorization": "Bearer tpl_inactiveinactive1"},
	}

	var firstBody string
	for name, headers := range cases {
		rec := doRequest(router, http.MethodGet, "/api/v1/me", "", headers)
		require.Equalf(t, http.StatusUnauthorized, rec.Code, "case %q", name)
		if firstBody == "" {
			firstBody = rec.Body.String()
			continue
		}
		assert.Equalf(t, firstBody, rec.Body.String(), "case %q must produce the same body as every other failure reason", name)
	}
}

// --- RequireActor / handleMe smoke test ---------------------------------

func TestRequireActor_SetsActorOnContextForHandler(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	agent := User{ID: "u1", Handle: "luna", Role: "agent", Active: true}
	users.put(agent)
	now := time.Now()
	putAPIKey(keys, "tpl_lunaslunaslunasluna", agent.ID, now.Add(time.Hour), nil)

	svc := newTestService(users, keys, nil, now)
	router := newTestRouter(svc)

	rec := doRequest(router, http.MethodGet, "/api/v1/me", "", map[string]string{"Authorization": "Bearer tpl_lunaslunaslunasluna"})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"handle":"luna"`)
	assert.Contains(t, rec.Body.String(), `"role":"agent"`)
}

// ensures the fake JWT verifier type in service_test.go satisfies
// JWTVerifier so both test files clearly share the same contract.
var _ JWTVerifier = (*fakeJWTVerifier)(nil)
