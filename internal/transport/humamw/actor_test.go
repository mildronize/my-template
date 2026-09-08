package humamw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/identity"
)

// actorEchoOutput/registerActorEcho give every test below one shared
// "who did the middleware chain resolve" probe: a bare operation whose
// handler reads ActorFromContext(ctx) and echoes what it found back as
// the response body. This is the actual "actor-context propagation"
// mechanism this ticket's acceptance criteria names — RequireActor/
// RequireSession never hand the resolved identity.User to a handler any
// other way (I4: one seam reads identity), so proving the handler can
// read it back via ActorFromContext is proving the middleware→handler
// contract works end to end, not just that withActor doesn't panic.
type actorEchoOutput struct {
	Body struct {
		Found   bool   `json:"found"`
		ActorID string `json:"actorId,omitempty"`
	}
}

func registerActorEcho(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "actor-echo",
		Method:      http.MethodGet,
		Path:        "/echo",
	}, func(ctx context.Context, _ *struct{}) (*actorEchoOutput, error) {
		out := &actorEchoOutput{}
		user, ok := ActorFromContext(ctx)
		out.Body.Found = ok
		if ok {
			out.Body.ActorID = user.ID
		}
		return out, nil
	})
}

// --- I1: RejectActorFields ------------------------------------------------
//
// These tests build a humagin-backed harness (newGinTestAPI, below)
// rather than this file's other tests' humatest/humaflow one:
// RejectActorFields' body-field check (hasForbiddenBodyField, actor.go)
// reaches through humagin.Unwrap to restore the peeked body, which panics
// against a non-gin huma.Context — and every request through this
// middleware runs that check (once the header/query checks pass), even a
// clean GET with no body at all. This exactly mirrors the adapter
// cmd/server/main.go actually wires RejectActorFields onto in production
// (apiV1H/apiBFFH, both humagin-backed), so it is not testing something
// looser than the real deployment shape.

// newGinTestAPI builds a fresh gin.Engine wrapped in a humagin-backed
// huma.API — the same construction cmd/server/main.go's newHumaAPIs uses
// for apiV1H/apiBFFH, minus the parts (OpenAPIPath/DocsPath disabling,
// RejectUnknownQueryParameters) this package's own isolated tests don't
// need.
func newGinTestAPI() (*gin.Engine, huma.API) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := humagin.New(router, newTestConfig("reject actor fields test"))
	return router, api
}

// doGinRequest sends a request through router, optionally setting headers
// (a nil body means no body is sent at all, matching a bare GET).
func doGinRequest(router *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// bodyEchoInput/bodyEchoOutput/registerBodyEcho give the body-field and
// body-restore tests below a real huma operation with a typed JSON body —
// registerActorEcho (above) takes no body at all, so it can't stand in
// for "does a real request body still reach huma's own downstream
// parsing after this middleware ran."
type bodyEchoInput struct {
	Body struct {
		Title string `json:"title,omitempty"`
	}
}

type bodyEchoOutput struct {
	Body struct {
		Title string `json:"title"`
	}
}

func registerBodyEcho(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "body-echo",
		Method:      http.MethodPost,
		Path:        "/body-echo",
	}, func(_ context.Context, in *bodyEchoInput) (*bodyEchoOutput, error) {
		out := &bodyEchoOutput{}
		out.Body.Title = in.Body.Title
		return out, nil
	})
}

func TestI1_RejectActorFields_XActorHeader(t *testing.T) {
	router, api := newGinTestAPI()
	api.UseMiddleware(RejectActorFields())
	registerActorEcho(api)

	resp := doGinRequest(router, http.MethodGet, "/echo", "", map[string]string{"X-Actor": "someone-else"})

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	errObj := errorBody(t, resp.Body.Bytes())
	assert.Equal(t, "actor_field_present", errObj["code"])
}

// TestI1_RejectActorFields_QueryParam — the ticket-6 finding this fix
// addresses: a request declaring an actor field in the query string must
// be rejected before any auth/DB-backed credential resolution ever runs,
// exactly like the header check. This is a bespoke pre-parse check again
// (not huma's own RejectUnknownQueryParameters default), so no
// Authorization header is needed to reach it — there is no auth
// middleware in this isolated harness at all.
func TestI1_RejectActorFields_QueryParam(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ActorID", "ownerId", "OwnerId"} {
		t.Run(field, func(t *testing.T) {
			router, api := newGinTestAPI()
			api.UseMiddleware(RejectActorFields())
			registerActorEcho(api)

			resp := doGinRequest(router, http.MethodGet, "/echo?"+field+"=someone-else", "", nil)

			assert.Equal(t, http.StatusBadRequest, resp.Code)
			errObj := errorBody(t, resp.Body.Bytes())
			assert.Equal(t, "actor_field_present", errObj["code"])
		})
	}
}

// TestI1_RejectActorFields_BodyField — same finding as the query-param
// case above, for the request body. bodyEchoInput declares no
// actor/actorId/ownerId field, so a request carrying one would otherwise
// only be caught by huma's own additionalProperties:false default
// (422 validation_error, downstream of body-echo's operation handler) —
// this asserts the bespoke pre-parse check catches it first instead
// (400 actor_field_present), never reaching huma's own parsing at all.
func TestI1_RejectActorFields_BodyField(t *testing.T) {
	for _, field := range []string{"actor", "actorId", "ActorID", "ownerId", "OwnerId"} {
		t.Run(field, func(t *testing.T) {
			router, api := newGinTestAPI()
			api.UseMiddleware(RejectActorFields())
			registerBodyEcho(api)

			body := `{"` + field + `":"someone-else","title":"a todo"}`
			resp := doGinRequest(router, http.MethodPost, "/body-echo", body, nil)

			assert.Equal(t, http.StatusBadRequest, resp.Code)
			errObj := errorBody(t, resp.Body.Bytes())
			assert.Equal(t, "actor_field_present", errObj["code"])
		})
	}
}

func TestI1_RejectActorFields_AllowsCleanRequest(t *testing.T) {
	router, api := newGinTestAPI()
	api.UseMiddleware(RejectActorFields())
	registerActorEcho(api)

	resp := doGinRequest(router, http.MethodGet, "/echo", "", nil)

	assert.Equal(t, http.StatusOK, resp.Code)
	var body struct {
		Found bool `json:"found"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.False(t, body.Found, "no auth middleware ran ahead of this test's echo handler, so no actor should be present")
}

// TestI1_RejectActorFields_RestoresBodyForDownstreamParsing is the
// positive counterpart to TestI1_RejectActorFields_BodyField: it proves a
// real, clean request body still reaches huma's own downstream JSON
// parsing correctly *after* this middleware peeked at it and restored it
// — not just that the middleware's own rejection logic behaves right in
// isolation. hasForbiddenBodyField (actor.go) reads the whole body via
// ctx.BodyReader() and must leave Request.Body re-readable afterward
// (io.NopCloser(bytes.NewReader(body))); if that restore were broken —
// e.g. left the body empty, or left it closed/already-consumed — huma's
// own request binding (which reads the body a second time, independently,
// via the same ctx.BodyReader() seam) would either fail to parse it or
// silently see an empty body, and this test would catch either case: a
// non-200 status, or a 200 with an empty/wrong Title.
func TestI1_RejectActorFields_RestoresBodyForDownstreamParsing(t *testing.T) {
	router, api := newGinTestAPI()
	api.UseMiddleware(RejectActorFields())
	registerBodyEcho(api)

	resp := doGinRequest(router, http.MethodPost, "/body-echo", `{"title":"a real todo"}`, nil)

	require.Equal(t, http.StatusOK, resp.Code, "response body: %s", resp.Body.String())
	var body struct {
		Title string `json:"title"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, "a real todo", body.Title,
		"huma's own downstream body parsing must still see the full body after RejectActorFields peeked at and restored it")
}

// --- RequireActor (Bearer, public) ---------------------------------------

func TestRequireActor_PropagatesResolvedActorToContext(t *testing.T) {
	repo := identity.NewRepo(newTestDB(t))
	svc := identity.NewService(repo, repo, nil, nil)
	user, rawKey := seedAgentWithAPIKey(t, repo)

	_, api := humatest.New(t, newTestConfig("require actor test"))
	api.UseMiddleware(RequireActor(svc))
	registerActorEcho(api)

	resp := api.Get("/echo", "Authorization: Bearer "+rawKey)

	require.Equal(t, http.StatusOK, resp.Code)
	var body struct {
		Found   bool   `json:"found"`
		ActorID string `json:"actorId"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.True(t, body.Found)
	assert.Equal(t, user.ID, body.ActorID)
}

func TestRequireActor_MissingCredentialIsUnauthorized(t *testing.T) {
	repo := identity.NewRepo(newTestDB(t))
	svc := identity.NewService(repo, repo, nil, nil)

	_, api := humatest.New(t, newTestConfig("require actor test"))
	api.UseMiddleware(RequireActor(svc))
	registerActorEcho(api)

	resp := api.Get("/echo")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
	errObj := errorBody(t, resp.Body.Bytes())
	assert.Equal(t, "unauthorized", errObj["code"])
}

func TestRequireActor_InvalidCredentialIsUnauthorized(t *testing.T) {
	repo := identity.NewRepo(newTestDB(t))
	svc := identity.NewService(repo, repo, nil, nil)

	_, api := humatest.New(t, newTestConfig("require actor test"))
	api.UseMiddleware(RequireActor(svc))
	registerActorEcho(api)

	resp := api.Get("/echo", "Authorization: Bearer not-a-real-key")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

// --- RequireSession (BFF session-equivalent) ------------------------------

const testSessionCookieName = "session"

// fakeSigner is a minimal SessionCookieReader stand-in — this package's
// own tests verify RequireSession's request-handling logic in isolation
// (ticket-5's own framing: "built and verified in isolation before any
// real route moves onto it"), not bff.Signer's HMAC/expiry mechanics,
// which internal/transport/bff's own session_test.go already covers
// exhaustively.
type fakeSigner struct {
	cookieToUserID map[string]string
}

func (f *fakeSigner) ParseSessionCookie(value string) (string, error) {
	id, ok := f.cookieToUserID[value]
	if !ok {
		return "", errors.New("fakeSigner: invalid or expired cookie")
	}
	return id, nil
}

// fakeUserRepo is a minimal identity.UserRepo stand-in, local to this
// package's own tests — mirrors internal/identity/service_test.go's own
// fakeUserRepo in spirit, but that one is unexported/package-private to
// internal/identity and so cannot be reused here (the same reasoning
// internal/transport/publicapi's own testutil doc comment already gives
// for driving a real DB instead — this file uses a fake, not a real DB,
// since RequireSession's own logic doesn't touch API keys or JWTs at all,
// only a plain user lookup).
type fakeUserRepo struct {
	users map[string]identity.User
}

func (f *fakeUserRepo) GetUserByID(_ context.Context, id string) (identity.User, error) {
	u, ok := f.users[id]
	if !ok {
		return identity.User{}, identity.ErrNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) GetUserByHandle(context.Context, string) (identity.User, error) {
	return identity.User{}, identity.ErrNotFound
}

func (f *fakeUserRepo) GetUserBySSOSubject(context.Context, string) (identity.User, error) {
	return identity.User{}, identity.ErrNotFound
}

func (f *fakeUserRepo) CreateUser(context.Context, string, string, *string) (identity.User, error) {
	return identity.User{}, errors.New("fakeUserRepo: CreateUser not supported")
}

func (f *fakeUserRepo) ListActiveUsers(context.Context) ([]identity.User, error) {
	return nil, nil
}

var _ identity.UserRepo = (*fakeUserRepo)(nil)

func TestRequireSession_PropagatesResolvedActorToContext(t *testing.T) {
	owner := identity.User{ID: "owner-1", Handle: "the-owner", Role: "owner", Active: true}
	signer := &fakeSigner{cookieToUserID: map[string]string{"valid-cookie": owner.ID}}
	users := &fakeUserRepo{users: map[string]identity.User{owner.ID: owner}}

	_, api := humatest.New(t, newTestConfig("require session test"))
	api.UseMiddleware(RequireSession(signer, testSessionCookieName, users))
	registerActorEcho(api)

	resp := api.Get("/echo", "Cookie: "+testSessionCookieName+"=valid-cookie")

	require.Equal(t, http.StatusOK, resp.Code)
	var body struct {
		Found   bool   `json:"found"`
		ActorID string `json:"actorId"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.True(t, body.Found)
	assert.Equal(t, owner.ID, body.ActorID)
}

func TestRequireSession_MissingCookieIsUnauthorized(t *testing.T) {
	signer := &fakeSigner{cookieToUserID: map[string]string{}}
	users := &fakeUserRepo{users: map[string]identity.User{}}

	_, api := humatest.New(t, newTestConfig("require session test"))
	api.UseMiddleware(RequireSession(signer, testSessionCookieName, users))
	registerActorEcho(api)

	resp := api.Get("/echo")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
	errObj := errorBody(t, resp.Body.Bytes())
	assert.Equal(t, "unauthorized", errObj["code"])
}

func TestRequireSession_InvalidCookieIsUnauthorized(t *testing.T) {
	signer := &fakeSigner{cookieToUserID: map[string]string{}}
	users := &fakeUserRepo{users: map[string]identity.User{}}

	_, api := humatest.New(t, newTestConfig("require session test"))
	api.UseMiddleware(RequireSession(signer, testSessionCookieName, users))
	registerActorEcho(api)

	resp := api.Get("/echo", "Cookie: "+testSessionCookieName+"=not-a-real-cookie")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

// TestI12_RequireSession_AgentRoleRejected — I12: a BFF session never
// resolves to role='agent', checked here independently of whatever else
// might check it once tickets 6/7 wire this for real, mirroring
// bff.RequireJSONSession's own defense-in-depth reasoning for the same
// check.
func TestI12_RequireSession_AgentRoleRejected(t *testing.T) {
	agent := identity.User{ID: "agent-1", Handle: "an-agent", Role: "agent", Active: true}
	signer := &fakeSigner{cookieToUserID: map[string]string{"valid-cookie": agent.ID}}
	users := &fakeUserRepo{users: map[string]identity.User{agent.ID: agent}}

	_, api := humatest.New(t, newTestConfig("require session test"))
	api.UseMiddleware(RequireSession(signer, testSessionCookieName, users))
	registerActorEcho(api)

	resp := api.Get("/echo", "Cookie: "+testSessionCookieName+"=valid-cookie")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestRequireSession_InactiveUserRejected(t *testing.T) {
	owner := identity.User{ID: "owner-1", Handle: "the-owner", Role: "owner", Active: false}
	signer := &fakeSigner{cookieToUserID: map[string]string{"valid-cookie": owner.ID}}
	users := &fakeUserRepo{users: map[string]identity.User{owner.ID: owner}}

	_, api := humatest.New(t, newTestConfig("require session test"))
	api.UseMiddleware(RequireSession(signer, testSessionCookieName, users))
	registerActorEcho(api)

	resp := api.Get("/echo", "Cookie: "+testSessionCookieName+"=valid-cookie")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}
