package bff

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
)

// TestAuthenticatedViewRendersOwnersOwnTodos — Done-when 9: the view must
// prove it renders scoped domain data, not just that GET / answers with a
// 200. Seeds an owner + a todo belonging to that owner directly against a
// test DB, establishes a valid session cookie by calling session.go's
// signing function directly (no /callback round-trip needed), issues
// GET /, and asserts the seeded todo's title appears in the response
// body.
func TestAuthenticatedViewRendersOwnersOwnTodos(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	todoSvc := todo.NewService(todo.NewRepo(conn))

	owner := seedUser(t, conn, "owner", "owner-sub-1", true)
	seedTodo(t, conn, owner.ID, "buy dog food for เจ้านาย")

	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, todoSvc)

	sessionValue, err := signer.NewSessionCookie(owner.ID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionValue})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "buy dog food for เจ้านาย",
		"GET / must render the seeded todo's title, not just answer 200 (Done-when 9)")
}

// TestAuthenticatedViewOnlyShowsOwnersOwnTodos is Done-when-9's negative
// twin: a todo belonging to a *different* owner must never appear, proving
// GET / actually scopes through todo.Service.ListTodos(ownerID) rather
// than, say, listing every todo in the table.
func TestAuthenticatedViewOnlyShowsOwnersOwnTodos(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	todoSvc := todo.NewService(todo.NewRepo(conn))

	owner := seedUser(t, conn, "owner", "owner-sub-1", true)
	otherOwner := seedUser(t, conn, "owner", "owner-sub-2", true)
	seedTodo(t, conn, owner.ID, "owner own todo")
	seedTodo(t, conn, otherOwner.ID, "someone elses private todo")

	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, todoSvc)

	sessionValue, err := signer.NewSessionCookie(owner.ID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionValue})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "owner own todo")
	assert.NotContains(t, rec.Body.String(), "someone elses private todo")
}

// TestI12_BFFSessionNeverResolvesToAgent_ViewMiddleware — I12's defense in
// depth: a session cookie that somehow carries an agent's users.id (forged
// here directly via Signer.NewSessionCookie, bypassing /callback entirely
// — task-4.md: "a forged/tampered cookie somehow carrying an agent's
// users.id — defense in depth, test it directly rather than assuming step
// 2 makes it unreachable") must be rejected identically to a missing
// session: redirected to /login, never rendering any todos.
func TestI12_BFFSessionNeverResolvesToAgent_ViewMiddleware(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	todoSvc := todo.NewService(todo.NewRepo(conn))

	agent := seedUser(t, conn, "agent", "agent-sub-1", true)
	seedTodo(t, conn, agent.ID, "an agent should never see this rendered")

	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, todoSvc)

	sessionValue, err := signer.NewSessionCookie(agent.ID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionValue})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code, "a session resolving to role=agent must redirect, not render (I12)")
	assert.Equal(t, "/login", rec.Header().Get("Location"))
	assert.NotContains(t, rec.Body.String(), "an agent should never see this rendered")
}

// TestViewMiddleware_MissingSessionRedirectsToLogin is the ordinary,
// non-I12 case: no cookie at all also redirects to /login, the same
// destination a rejected role gets (I5's "don't leak why", applied here).
func TestViewMiddleware_MissingSessionRedirectsToLogin(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	todoSvc := todo.NewService(todo.NewRepo(conn))

	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, todoSvc)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}
