package publicapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/identity"
)

// newMiddlewareTestRouter mirrors newIntegrationRouter/
// newKeysIntegrationRouter's shape but only mounts RejectActorFields,
// RequireActor, and a bare /me + /echo route — just enough surface for
// this file's middleware-focused tests. Unlike milestone-1's version of
// these tests (identity/middleware_handler_test.go), this one drives a
// real identity.Service against a real temp-file SQLite database instead
// of identity/service_test.go's fakes: those fakes are unexported and
// package-private to internal/identity, and RequireActor/RejectActorFields
// moved out of that package to here (ARCHITECTURE.md — a domain module or
// internal/identity holds no transport code), so this package can no
// longer reach them. This matches the pattern every other handler test in
// this package already uses (todo_handler_test.go, keys_handler_test.go).
func newMiddlewareTestRouter(svc *identity.Service) *gin.Engine {
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(RejectActorFields(), RequireActor(svc))
	group.GET("/me", handleMe)
	group.POST("/echo", func(c *gin.Context) { c.Status(http.StatusOK) })
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

// --- I1: RejectActorFields --------------------------------------------

// TestI1_RejectActorFields_BodyField — I1: a request body naming an
// actor is a 400 (actor_field_present), not a value silently read and
// ignored. RejectActorFields runs before RequireActor and aborts the
// chain outright, so the service behind it never actually gets called —
// an empty, freshly-migrated database is enough here.
func TestI1_RejectActorFields_BodyField(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ActorID", "ownerId", "OwnerId"} {
		t.Run(field, func(t *testing.T) {
			repo := identity.NewRepo(newTestDB(t))
			svc := identity.NewService(repo, repo, nil, nil)
			router := newMiddlewareTestRouter(svc)

			body := `{"` + field + `":"someone-else","title":"a todo"}`
			rec := doMiddlewareRequest(router, http.MethodPost, "/api/v1/echo", body, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "actor_field_present")
		})
	}
}

func TestI1_RejectActorFields_QueryParam(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ownerId"} {
		t.Run(field, func(t *testing.T) {
			repo := identity.NewRepo(newTestDB(t))
			svc := identity.NewService(repo, repo, nil, nil)
			router := newMiddlewareTestRouter(svc)

			rec := doMiddlewareRequest(router, http.MethodPost, "/api/v1/echo?"+field+"=someone-else", `{}`, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "actor_field_present")
		})
	}
}

// TestI1_RejectActorFields_XActorHeader — the X-Actor header can't be
// delegated to OpenAPI's additionalProperties: false the way body fields
// can (task-2.md), so it gets its own explicit check.
func TestI1_RejectActorFields_XActorHeader(t *testing.T) {
	repo := identity.NewRepo(newTestDB(t))
	svc := identity.NewService(repo, repo, nil, nil)
	router := newMiddlewareTestRouter(svc)

	rec := doMiddlewareRequest(router, http.MethodPost, "/api/v1/echo", `{}`, map[string]string{"X-Actor": "someone-else"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "actor_field_present")
}

func TestI1_RejectActorFields_AllowsCleanRequest(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	svc := identity.NewService(repo, repo, nil, nil)
	router := newMiddlewareTestRouter(svc)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	rec := doMiddlewareRequest(router, http.MethodPost, "/api/v1/echo", `{"title":"a todo"}`,
		map[string]string{"Authorization": "Bearer " + rawKey})

	assert.Equal(t, http.StatusOK, rec.Code, "a request with no actor field must not be rejected by I1's guard")
}

// --- I5: identical 401 body regardless of failure reason ----------------

// TestI5_UnauthorizedResponseBodyIdenticalAcrossFailureReasons — I5: 401
// never leaks why. Missing credential, malformed credential, and the I2
// owner-rejection all produce byte-identical response bodies; the actual
// reason is only ever visible in server-side logs.
func TestI5_UnauthorizedResponseBodyIdenticalAcrossFailureReasons(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	svc := identity.NewService(repo, repo, nil, nil)
	router := newMiddlewareTestRouter(svc)
	ctx := context.Background()

	owner, err := repo.CreateUser(ctx, "owner", "owner", nil)
	require.NoError(t, err)
	ownerRawKey := "tpl_ownerkeyownerkey1"
	_, err = repo.CreateAPIKey(ctx, owner.ID, identity.HashAPIKey(ownerRawKey), ownerRawKey[:12], time.Now().Add(time.Hour))
	require.NoError(t, err)

	// An expired key. Its user_id ("does-not-matter") deliberately names no
	// real users row — tryAPIKey's expiry check runs before it ever looks
	// the user up, so this key must fail on expiry alone, never reach that
	// lookup (SQLite's own FK enforcement is off by default for this
	// driver/DSN, so the insert itself is not the thing under test here).
	expiredRawKey := "tpl_expiredexpired123"
	_, err = repo.CreateAPIKey(ctx, "does-not-matter", identity.HashAPIKey(expiredRawKey), expiredRawKey[:12], time.Now().Add(-time.Minute))
	require.NoError(t, err)

	// An inactive user. repo.CreateUser always creates an active row
	// (DATA_MODEL.md — there is no "create inactive" constructor), so
	// this test flips the flag directly against the database afterwards,
	// the same way internal/domain/todo's repo_test.go manipulates
	// created_at directly when the schema itself offers no other way to
	// reach the state under test.
	inactive, err := repo.CreateUser(ctx, "inactive-agent", "agent", nil)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `UPDATE users SET active = FALSE WHERE id = ?`, inactive.ID)
	require.NoError(t, err)
	inactiveRawKey := "tpl_inactiveinactive1"
	_, err = repo.CreateAPIKey(ctx, inactive.ID, identity.HashAPIKey(inactiveRawKey), inactiveRawKey[:12], time.Now().Add(time.Hour))
	require.NoError(t, err)

	cases := map[string]map[string]string{
		"missing credential":    nil,
		"malformed credential":  {"Authorization": "Basic garbage"},
		"unknown bearer token":  {"Authorization": "Bearer tpl_unknowntoken00000"},
		"expired api key":       {"Authorization": "Bearer " + expiredRawKey},
		"owner-role api key":    {"Authorization": "Bearer " + ownerRawKey},
		"inactive-user api key": {"Authorization": "Bearer " + inactiveRawKey},
	}

	var firstBody string
	for name, headers := range cases {
		rec := doMiddlewareRequest(router, http.MethodGet, "/api/v1/me", "", headers)
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
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	svc := identity.NewService(repo, repo, nil, nil)
	router := newMiddlewareTestRouter(svc)
	_, rawKey := createAgentWithKey(t, conn, "luna")

	rec := doMiddlewareRequest(router, http.MethodGet, "/api/v1/me", "", map[string]string{"Authorization": "Bearer " + rawKey})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"handle":"luna"`)
	assert.Contains(t, rec.Body.String(), `"role":"agent"`)
}
