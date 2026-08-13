package bff

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"github.com/mildronize/my-template/internal/platform"
)

// randomState returns a fresh, unguessable CSRF state value — "standard
// CSRF-for-OAuth practice, independent of PKCE" (task-4.md step 1). 32
// random bytes, base64url-encoded, same shape as oauth2.GenerateVerifier's
// own randomness but for a different purpose (state ties this browser's
// /login to its own /callback; the verifier is PKCE's proof of possession
// of the /login request specifically).
func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewLoginHandler builds GET /login (task-4.md step 1, _contract/API.md).
// Every request through this handler generates a fresh PKCE verifier
// (oauth2.GenerateVerifier — I11: never reused across logins, never
// skipped) and a fresh CSRF state value, signs both into the short-lived
// state cookie (session.go's one signing helper), and redirects to Hydra's
// authorize endpoint with the corresponding S256 code_challenge always
// present in the URL (I11).
func NewLoginHandler(cfg *platform.Config, signer *Signer, logger *slog.Logger) gin.HandlerFunc {
	// Parsed once here, at handler-construction time (wireBFF calls this
	// once at startup), not per-request — see middleware.go's
	// secureFromURL doc comment (task-10).
	secure := secureFromURL(cfg.AuthAudience)

	return func(c *gin.Context) {
		if !configured(cfg) {
			renderLoginError(c, logger, "owner login not configured — SSO_ISSUER/SSO_CLIENT_ID/SSO_CLIENT_SECRET/AUTH_AUDIENCE must all be set (see docs/GETTING-STARTED.md Step 1)")
			return
		}

		verifier := oauth2.GenerateVerifier()
		state, err := randomState()
		if err != nil {
			renderLoginError(c, logger, "generating CSRF state failed")
			return
		}

		cookieValue, err := signer.NewStateCookie(state, verifier)
		if err != nil {
			renderLoginError(c, logger, "signing state cookie failed")
			return
		}
		setCookie(c, stateCookieName, cookieValue, int(stateTTL.Seconds()), secure)

		authURL := oauthConfig(cfg).AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
		c.Redirect(http.StatusFound, authURL)
	}
}
