package humamw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
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

// --- I1: RejectActorHeader ----------------------------------------------

func TestI1_RejectActorHeader_XActorPresent(t *testing.T) {
	_, api := humatest.New(t, newTestConfig("reject actor header test"))
	api.UseMiddleware(RejectActorHeader())
	registerActorEcho(api)

	resp := api.Get("/echo", "X-Actor: someone-else")

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	errObj := errorBody(t, resp.Body.Bytes())
	assert.Equal(t, "actor_field_present", errObj["code"])
}

func TestI1_RejectActorHeader_AllowsCleanRequest(t *testing.T) {
	_, api := humatest.New(t, newTestConfig("reject actor header test"))
	api.UseMiddleware(RejectActorHeader())
	registerActorEcho(api)

	resp := api.Get("/echo")

	assert.Equal(t, http.StatusOK, resp.Code)
	var body struct {
		Found bool `json:"found"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.False(t, body.Found, "no auth middleware ran ahead of this test's echo handler, so no actor should be present")
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
