package publicapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/identity"
)

// echoBody is a minimal, mostly-empty request body for this file's own
// synthetic /api/v1/echo operation (below) — declares exactly one
// legitimate field (title) so a request carrying only that field is
// "clean" under huma's additionalProperties:false default, while a
// request also carrying an actor-shaped field (actor/actorId/ownerId,
// none of which this struct declares) is rejected as an unknown field —
// the same mechanism contract.md's "Middleware mapping" table describes
// as I1's body-field guard's ticket-6 replacement.
type echoBody struct {
	Title string `json:"title,omitempty"`
}

type echoInput struct {
	Body echoBody
}

type echoOutput struct{}

// registerEchoOperation registers POST /api/v1/echo — a stand-in for "any
// real operation on this surface," standing in for the old bare
// `group.POST("/echo", func(c *gin.Context) { c.Status(http.StatusOK) })`
// this file used pre-ticket-6. DefaultStatus is pinned to 200 (not huma's
// own no-Body-field default of 204) to match that old handler's own
// c.Status(http.StatusOK) exactly, since TestI1_RejectActorFields_
// AllowsCleanRequest (below) asserts on it.
func registerEchoOperation(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "echo",
		Method:        http.MethodPost,
		Path:          "/api/v1/echo",
		DefaultStatus: http.StatusOK,
	}, func(_ context.Context, _ *echoInput) (*echoOutput, error) {
		return &echoOutput{}, nil
	})
}

// newMiddlewareTestRouter mirrors newIntegrationRouter/
// newKeysIntegrationRouter's shape (publicapi_testutil_test.go's
// newHumaV1API: humamw.RejectActorFields then humamw.RequireActor(svc),
// the same order and mechanism cmd/server/main.go attaches to apiV1H) —
// plus GET /api/v1/me (the real operation, RegisterMeRoutes) and this
// file's own synthetic POST /api/v1/echo (registerEchoOperation), just
// enough surface for this file's middleware-focused tests. ticket-6: this
// used to mount publicapi.RejectActorFields/RequireActor directly as gin
// middleware on a bare "/api/v1" group with a hand-wired "/me"/"echo" —
// /api/v1 itself no longer works that way in production (it runs on huma,
// cmd/server/main.go's apiV1H), so this harness now mounts the same
// huma-based wiring instead, per the story-1/ticket-6 contract
// clarification: test *harness* is expected to change when the transport
// framework changes; only assertions must not drift without being
// flagged.
func newMiddlewareTestRouter(svc *identity.Service) *gin.Engine {
	router := gin.New()
	api := newHumaV1API(router, svc)
	RegisterMeRoutes(api)
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

// --- I1: actor-field rejection ------------------------------------------

// TestI1_RejectActorFields_BodyField — I1: a request body naming an
// actor is rejected, not a value silently read and ignored.
//
// Fix (story-1/ticket-6 finding, contract.md's "Middleware mapping"
// table): ticket-6 originally retired this body-field check for /api/v1,
// relying on huma's own additionalProperties:false default instead (422
// validation_error) — but that default runs *inside* huma's own
// parse->validate->resolve->handle chain, which every api.UseMiddleware,
// including humamw.RequireActor, wraps entirely (huma.go:881). That moved
// actor-field rejection from *before* auth to *after* it, reversing I1's
// own documented ordering intent (middleware.go's old doc comment: "so a
// shape violation never spends a DB lookup on credential resolution").
// humamw.RejectActorFields (renamed from RejectActorHeader) now performs
// this check itself again, pre-auth, exactly like the original
// pre-ticket-6 behavior — so this test no longer needs an Authorization
// header to reach it, and the original assertion (400
// actor_field_present) holds again.
func TestI1_RejectActorFields_BodyField(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ActorID", "ownerId", "OwnerId"} {
		t.Run(field, func(t *testing.T) {
			conn := newTestDB(t)
			repo := identity.NewRepo(conn)
			svc := identity.NewService(repo, repo, nil, nil)
			router := newMiddlewareTestRouter(svc)

			body := `{"` + field + `":"someone-else","title":"a todo"}`
			rec := doMiddlewareRequest(router, http.MethodPost, "/api/v1/echo", body, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "actor_field_present")
		})
	}
}

// TestI1_RejectActorFields_QueryParam — see TestI1_RejectActorFields_
// BodyField's own doc comment: same fix, same reason no Authorization
// header is needed here either.
func TestI1_RejectActorFields_QueryParam(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ownerId"} {
		t.Run(field, func(t *testing.T) {
			conn := newTestDB(t)
			repo := identity.NewRepo(conn)
			svc := identity.NewService(repo, repo, nil, nil)
			router := newMiddlewareTestRouter(svc)

			rec := doMiddlewareRequest(router, http.MethodPost, "/api/v1/echo?"+field+"=someone-else", `{}`, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "actor_field_present")
		})
	}
}

// TestI1_RejectActorFields_XActorHeader — the X-Actor header can't be
// delegated to huma's own additionalProperties:false default the way
// body fields can (contract.md's "Middleware mapping" table), so it
// keeps its own explicit check — humamw.RejectActorFields (ticket-5,
// widened by this fix), attached to apiV1H the same way cmd/server/main.go
// does.
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
// reason is only ever visible in server-side logs. Runs against
// humamw.RequireActor (ticket-5/6) instead of the old publicapi.RequireActor
// — same property, same identity.Service.ResolveActor call underneath.
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

// --- RequireActor / GetMe smoke test ---------------------------------

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
