package bff

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/identity"
)

// TestI11_LoginRedirectAlwaysIncludesPKCEChallenge — I11: the login flow
// must never be able to construct an auth URL without a PKCE
// verifier/challenge pair. Asserts the redirect Location oauth2's
// AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)) produces always
// carries code_challenge and code_challenge_method=S256 — not just that
// the library supports one (task-4.md's own suggested test shape).
func TestI11_LoginRedirectAlwaysIncludesPKCEChallenge(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, nil)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code, "GET /login must redirect to the issuer's authorize endpoint")

	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	q := loc.Query()

	assert.NotEmpty(t, q.Get("code_challenge"), "auth URL must always carry a PKCE code_challenge (I11)")
	assert.Equal(t, "S256", q.Get("code_challenge_method"), "auth URL must always declare S256 (I11)")
	assert.NotEmpty(t, q.Get("state"), "auth URL must carry the CSRF state value")

	// The state cookie itself must be set — /callback reads the verifier
	// back out of it, never out of the URL (the verifier is never sent to
	// the browser as a query param at all).
	cookies := rec.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == stateCookieName {
			found = true
			assert.True(t, c.HttpOnly, "state cookie must be HttpOnly")
			assert.True(t, c.Secure, "state cookie must be Secure")
		}
	}
	assert.True(t, found, "GET /login must set the state cookie")
}

// TestLogin_MissingConfigShowsErrorNotACrash confirms an unconfigured bff
// (SSO_CLIENT_ID/SECRET unset — the state GETTING-STARTED.md's Step 1
// leaves a fresh clone in) answers with a clear error page rather than
// building a malformed redirect or panicking.
func TestLogin_MissingConfigShowsErrorNotACrash(t *testing.T) {
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	cfg.SSOClientID = "" // simulate scripts/register.sh's Step 1 not having run yet
	signer := NewSigner([]byte(cfg.SessionSecret))
	router := newTestRouter(cfg, signer, nil, repo, nil)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusFound, rec.Code, "an unconfigured bff must not attempt a redirect to Hydra")
	assert.Contains(t, rec.Body.String(), "Login failed")
}
