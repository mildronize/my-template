package bff

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
)

// secureCompareStrings is a constant-time string comparison for the CSRF
// state check below — the same reasoning hmac.Equal exists for signature
// comparison in session.go, applied here to a plain string equality
// instead of a MAC.
func secureCompareStrings(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// NewCallbackHandler builds GET /callback (task-4.md step 2, _contract/
// API.md). idVerifier verifies the token exchange's id_token the same way
// internal/identity/jwt.go verifies a Bearer JWT (RS256-pinned, iss/aud/exp
// checked — I6, I7) but scoped to this OAuth client's own id ("aud" for an
// id_token is the client_id per OIDC, not this service's AUTH_AUDIENCE,
// which is what publicapi's own Bearer-JWT verifier is pinned to instead)
// — cmd/server/main.go builds it via the same identity.NewJWTVerifier
// constructor with cfg.SSOClientID as the audience, reusing already-tested
// I6/I7 logic rather than a second, bespoke JWT verifier.
//
// users is only ever used for GetUserBySSOSubject here (I4 — this package
// never queries any other identity table itself).
func NewCallbackHandler(cfg *platform.Config, signer *Signer, idVerifier identity.JWTVerifier, users identity.UserRepo, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !configured(cfg) || idVerifier == nil {
			renderLoginError(c, logger, "owner login not configured — SSO_ISSUER/SSO_CLIENT_ID/SSO_CLIENT_SECRET/AUTH_AUDIENCE must all be set (see docs/GETTING-STARTED.md Step 1)")
			return
		}

		// Step 2.a: read the state cookie back before touching the query
		// string's own `code`/`state` at all. A missing or tampered state
		// cookie (I11) means there is no verifier to exchange with, so
		// this returns before oauthConfig(cfg).Exchange is ever called —
		// the exchange is never attempted without a verifier in hand.
		stateCookie, err := c.Cookie(stateCookieName)
		if err != nil || stateCookie == "" {
			renderLoginError(c, logger, "no state cookie present (missing, expired, or already used)")
			return
		}
		clearCookie(c, stateCookieName) // one-time use regardless of what happens next

		wantState, verifier, err := signer.ParseStateCookie(stateCookie)
		if err != nil {
			renderLoginError(c, logger, "state cookie failed verification: "+err.Error())
			return
		}

		gotState := c.Query("state")
		if gotState == "" || !secureCompareStrings(gotState, wantState) {
			renderLoginError(c, logger, "state mismatch (possible CSRF)")
			return
		}

		code := c.Query("code")
		if code == "" {
			renderLoginError(c, logger, "no authorization code in callback")
			return
		}

		// Step 2.b: exchange the code for a token — I11: verifier is
		// always threaded through explicitly via oauth2.VerifierOption,
		// never omitted, and this line is only reachable once the state
		// cookie has already produced one.
		token, err := oauthConfig(cfg).Exchange(c.Request.Context(), code, oauth2.VerifierOption(verifier))
		if err != nil {
			renderLoginError(c, logger, "token exchange failed: "+err.Error())
			return
		}

		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok || rawIDToken == "" {
			renderLoginError(c, logger, "token response carried no id_token")
			return
		}

		sub, err := idVerifier.Verify(c.Request.Context(), rawIDToken)
		if err != nil {
			renderLoginError(c, logger, "id_token failed verification: "+err.Error())
			return
		}

		// Step 2.c: no JIT (DATA_MODEL.md's "Owner provisioning" note,
		// I10's human sibling) — an unrecognized sub is an error page,
		// never a new row.
		user, err := users.GetUserBySSOSubject(c.Request.Context(), sub)
		if err != nil {
			if !errors.Is(err, identity.ErrNotFound) {
				logger.Error("bff: looking up user by sso_subject failed", "error", err)
			}
			renderLoginError(c, logger, "sub has no matching owner account (no JIT)")
			return
		}

		// I12: a match resolving to role="agent" is an error page, never a
		// session — the exact check this invariant exists to satisfy.
		if user.Role == "agent" {
			renderLoginError(c, logger, "sub resolved to role=agent, refusing to start an owner session (I12)")
			return
		}
		if user.Role != "owner" || !user.Active {
			renderLoginError(c, logger, "sub resolved to a non-owner or inactive user")
			return
		}

		sessionValue, err := signer.NewSessionCookie(user.ID)
		if err != nil {
			renderLoginError(c, logger, "signing session cookie failed")
			return
		}
		setCookie(c, sessionCookieName, sessionValue, int(sessionTTL.Seconds()))

		c.Redirect(http.StatusFound, "/")
	}
}
