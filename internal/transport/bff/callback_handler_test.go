package bff

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/identity"
)

// callbackRequest builds a GET /callback?code=...&state=... request,
// optionally carrying a state cookie.
func callbackRequest(state, code, stateCookieValue string, hasCookie bool) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/callback?code="+code+"&state="+state, nil)
	if hasCookie {
		req.AddCookie(&http.Cookie{Name: stateCookieName, Value: stateCookieValue})
	}
	return req
}

// TestI11_CallbackNeverExchangesWithoutStateCookie — I11's other half: a
// missing or tampered state cookie means there is no PKCE verifier to
// exchange with, so the token exchange must never be attempted at all
// (task-4.md's own suggested test shape). Both sub-cases assert the fake
// token endpoint saw zero requests.
func TestI11_CallbackNeverExchangesWithoutStateCookie(t *testing.T) {
	t.Run("missing state cookie", func(t *testing.T) {
		conn := newTestDB(t)
		repo := identity.NewRepo(conn)
		idp := newFakeIDP(t, "test-client")
		cfg := idp.testConfig()
		signer := NewSigner([]byte(cfg.SessionSecret))
		router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, nil, nil)

		req := callbackRequest("some-state", "some-code", "", false)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.NotEqual(t, http.StatusFound, rec.Code, "no state cookie must never redirect to / (no session)")
		assert.Equal(t, int64(0), idp.tokenHits.Load(), "the token endpoint must never be hit without a state cookie to read a verifier from")
		assertNoSessionCookieSet(t, rec)
	})

	t.Run("tampered state cookie", func(t *testing.T) {
		conn := newTestDB(t)
		repo := identity.NewRepo(conn)
		idp := newFakeIDP(t, "test-client")
		cfg := idp.testConfig()
		signer := NewSigner([]byte(cfg.SessionSecret))
		router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, nil, nil)

		valid, err := signer.NewStateCookie("real-state", "real-verifier")
		require.NoError(t, err)
		tampered := valid[:len(valid)-2] + "xx" // flip the tail of the signature

		req := callbackRequest("real-state", "some-code", tampered, true)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.NotEqual(t, http.StatusFound, rec.Code, "a tampered state cookie must never redirect to / (no session)")
		assert.Equal(t, int64(0), idp.tokenHits.Load(), "the token endpoint must never be hit with a tampered state cookie")
		assertNoSessionCookieSet(t, rec)
	})

	t.Run("state query param mismatch", func(t *testing.T) {
		conn := newTestDB(t)
		repo := identity.NewRepo(conn)
		idp := newFakeIDP(t, "test-client")
		cfg := idp.testConfig()
		signer := NewSigner([]byte(cfg.SessionSecret))
		router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, nil, nil)

		valid, err := signer.NewStateCookie("real-state", "real-verifier")
		require.NoError(t, err)

		// A valid, well-signed cookie -- but the callback's own ?state=
		// query param doesn't match what it carries (classic CSRF shape:
		// an attacker's own /login-initiated state cookie, replayed
		// against a victim's browser with the attacker's own code/state).
		req := callbackRequest("attacker-state", "some-code", valid, true)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.NotEqual(t, http.StatusFound, rec.Code)
		assert.Equal(t, int64(0), idp.tokenHits.Load(), "a state mismatch must never reach the token exchange")
	})
}

func assertNoSessionCookieSet(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			t.Fatalf("a session cookie must never be set on a failed callback, got %q", c.Value)
		}
	}
}

// TestI12_BFFSessionNeverResolvesToAgent_Callback — I12: a sub that
// resolves to an agent row must be rejected at the callback itself, not
// just at the view middleware (task-4.md: "write
// TestI12_BFFSessionNeverResolvesToAgent against this code path directly,
// not just against the view handler's middleware").
func TestI12_BFFSessionNeverResolvesToAgent_Callback(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	agent := seedUser(t, conn, "agent", "agent-sub-1", true)

	idp := newFakeIDP(t, "test-client")
	idp.setSub(agent.SSOSubject)
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, nil, nil)

	valid, err := signer.NewStateCookie("real-state", "real-verifier")
	require.NoError(t, err)

	req := callbackRequest("real-state", "some-code", valid, true)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusFound, rec.Code, "a sub resolving to role=agent must never redirect to / (I12)")
	assert.Equal(t, int64(1), idp.tokenHits.Load(), "the exchange itself is legitimate here -- it's the role check afterward that must reject this")
	assertNoSessionCookieSet(t, rec)
}

// TestCallback_UnrecognizedSubIsAnErrorPageNeverAJITRow proves the "no
// JIT" rule (DATA_MODEL.md's "Owner provisioning" note, I10's human
// sibling): a sub with no matching users.sso_subject row must be an error
// page, and must never create one.
func TestCallback_UnrecognizedSubIsAnErrorPageNeverAJITRow(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)

	idp := newFakeIDP(t, "test-client")
	idp.setSub("sub-that-was-never-seeded")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, nil, nil)

	valid, err := signer.NewStateCookie("real-state", "real-verifier")
	require.NoError(t, err)

	req := callbackRequest("real-state", "some-code", valid, true)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusFound, rec.Code)
	assertNoSessionCookieSet(t, rec)

	var count int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count))
	assert.Equal(t, 0, count, "an unrecognized sub must never create a users row")
}

// TestCallback_SuccessfulOwnerLoginSetsSessionAndRedirects is the positive
// case behind the I11/I12 negatives above: a real owner row, a real
// (fake-signed) id_token, a valid state cookie -- the callback must
// exchange, verify, resolve, and redirect to / with a working session
// cookie.
func TestCallback_SuccessfulOwnerLoginSetsSessionAndRedirects(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	owner := seedUser(t, conn, "owner", "owner-sub-1", true)

	idp := newFakeIDP(t, "test-client")
	idp.setSub(owner.SSOSubject)
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, nil, nil)

	valid, err := signer.NewStateCookie("real-state", "real-verifier")
	require.NoError(t, err)

	req := callbackRequest("real-state", "some-code", valid, true)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/", rec.Header().Get("Location"))
	assert.Equal(t, int64(1), idp.tokenHits.Load())

	var sessionValue string
	var stateCleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessionValue = c.Value
		}
		if c.Name == stateCookieName && c.MaxAge < 0 {
			stateCleared = true
		}
	}
	require.NotEmpty(t, sessionValue, "a successful callback must set a session cookie")
	assert.True(t, stateCleared, "the state cookie must be cleared on success")

	gotUserID, err := signer.ParseSessionCookie(sessionValue)
	require.NoError(t, err)
	assert.Equal(t, owner.ID, gotUserID)
}
