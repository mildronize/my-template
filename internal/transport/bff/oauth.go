package bff

import (
	"strings"

	"golang.org/x/oauth2"

	"github.com/mildronize/my-template/internal/platform"
)

// oauthConfig builds the oauth2.Config login_handler.go and
// callback_handler.go both use — one place, so AuthCodeURL and Exchange
// are guaranteed to agree on client credentials and endpoints (task-4.md
// step 1: "build an oauth2.Config from SSO_ISSUER/AUTH_AUDIENCE/client
// credentials").
//
// Endpoint URLs are derived from cfg.SSOIssuer the same way
// internal/identity/jwt.go derives its JWKS URL from it (issuer +
// "/.well-known/jwks.json") — Hydra serves /oauth2/auth and /oauth2/token
// off the same base as its issuer/JWKS, so there is no separate
// "Hydra public URL" config value to add here. scripts/register.sh's own
// probe (against HYDRA_PUBLIC_URL) is a registration-time concern, not a
// runtime one this service's own config needs to duplicate.
//
// AuthStyle is pinned to AuthStyleInHeader (HTTP Basic) rather than left
// at AuthStyleAutoDetect, matching scripts/register.sh's
// --token-endpoint-auth-method client_secret_basic registration exactly —
// sso-consumer-contract.md's own "Better Auth: two defaults that disagree
// with this contract" note is the reason to pin this rather than trust
// auto-detection: "when a library default disagrees with this contract,
// change the library, not the client registration."
func oauthConfig(cfg *platform.Config) *oauth2.Config {
	issuer := strings.TrimRight(cfg.SSOIssuer, "/")
	redirectURL := strings.TrimRight(cfg.AuthAudience, "/") + "/callback"

	return &oauth2.Config{
		ClientID:     cfg.SSOClientID,
		ClientSecret: cfg.SSOClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   issuer + "/oauth2/auth",
			TokenURL:  issuer + "/oauth2/token",
			AuthStyle: oauth2.AuthStyleInHeader,
		},
		RedirectURL: redirectURL,
		// scripts/register.sh registers this client with --scope
		// openid,offline; this service's own login flow only ever needs
		// openid (it reads the id_token's sub and nothing else, per
		// sso-consumer-contract.md §4's "sso-app emits nothing beyond sub
		// today") — no refresh_token is requested since the session cookie
		// itself is this surface's whole renewal story (DATA_MODEL.md: no
		// sessions table, no refresh flow to keep alive).
		Scopes: []string{"openid"},
	}
}

// configured reports whether enough of cfg is set for the owner-login flow
// to attempt anything at all. Unlike SSOIssuer/AuthAudience's dormant JWT
// seam (which the server silently runs without), an unconfigured BFF still
// mounts GET /login and GET /callback — GETTING-STARTED.md's own
// walkthrough promises the server and the public API keep working with no
// Hydra client registered yet — but both handlers check this first and
// return a clear, honest error instead of building a malformed
// oauth2.Config and failing confusingly partway through a redirect or an
// exchange.
func configured(cfg *platform.Config) bool {
	return cfg.SSOIssuer != "" && cfg.SSOClientID != "" && cfg.SSOClientSecret != "" && cfg.AuthAudience != ""
}
