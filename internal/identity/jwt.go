package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// jwxVerifier is the production JWTVerifier (task-2.md's "jwx/v3 usage
// notes"): a jwk.Cache built once and pointed at the issuer's JWKS
// endpoint (I7 — the cache refetches on its own schedule; a specific key
// is never hardcoded/pinned here), verifying with jwa.RS256 pinned
// explicitly via a custom jws.KeyProvider rather than trusting the
// token's own `alg` header (I6).
type jwxVerifier struct {
	cache    *jwk.Cache
	jwksURL  string
	issuer   string
	audience string
}

// NewJWTVerifier builds the jwk.Cache once — call this once at startup
// (or lazily on first use), never per-request — and registers issuer's
// `/.well-known/jwks.json` with it. Register blocks until the first fetch
// succeeds (or fails), so a returned *jwxVerifier is immediately usable.
// The returned JWTVerifier is safe for concurrent use across requests.
func NewJWTVerifier(ctx context.Context, issuer, audience string) (JWTVerifier, error) {
	if issuer == "" || audience == "" {
		return nil, errors.New("identity: issuer and audience must both be set to build a JWT verifier")
	}

	jwksURL := strings.TrimRight(issuer, "/") + "/.well-known/jwks.json"

	cache, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		return nil, fmt.Errorf("starting JWKS cache: %w", err)
	}
	if err := cache.Register(ctx, jwksURL); err != nil {
		return nil, fmt.Errorf("registering JWKS endpoint %q: %w", jwksURL, err)
	}

	return &jwxVerifier{cache: cache, jwksURL: jwksURL, issuer: issuer, audience: audience}, nil
}

// Verify validates token as an RS256-signed JWT from the configured
// issuer/audience and returns its `sub` claim. iss/aud/exp are all
// validated (task-2.md); RS256 is pinned via rs256KeyProvider below,
// never read from the token's own header.
//
// Key lookup (inside rs256KeyProvider.FetchKeys, invoked by jwt.Parse
// below) forces exactly one Cache.Refresh()+retry on a kid miss (I7's
// milestone-2 correction) rather than trusting whatever jwk.Cache happens
// to have on hand — jwk.Cache only refetches on its own schedule, never
// lazily on a miss, so without this a real key rotation would reject
// every token until the next scheduled refresh (up to 15 minutes,
// silently). rs256KeyProvider holds the cache+URL (not a pre-fetched
// jwk.Set) specifically so it can perform that bounded refresh itself.
func (v *jwxVerifier) Verify(ctx context.Context, token string) (string, error) {
	parsed, err := jwt.Parse([]byte(token),
		jwt.WithKeyProvider(rs256KeyProvider{cache: v.cache, jwksURL: v.jwksURL}),
		jwt.WithValidate(true),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
	)
	if err != nil {
		return "", err
	}

	sub, ok := parsed.Subject()
	if !ok || sub == "" {
		return "", errors.New("token has no subject claim")
	}
	return sub, nil
}

// rs256KeyProvider selects a verification key from the issuer's cached
// JWKS by the token's `kid` header (falling back to the sole key when the
// set has exactly one and the token has none), and always pins jwa.RS256
// as the algorithm to verify with (I6) — regardless of what the token's
// own `alg` header or the JWK's own `alg` field claims. This is what
// defends against algorithm-confusion attacks: an attacker cannot get
// this verifier to attempt anything other than RS256, no matter what the
// token's header says.
//
// It holds the cache + jwksURL rather than a single pre-fetched jwk.Set
// so that FetchKeys can force one bounded refresh on a lookup miss (I7).
type rs256KeyProvider struct {
	cache   *jwk.Cache
	jwksURL string
}

// FetchKeys looks the signature's kid up against whatever JWKS is
// currently cached. If that lookup fails — kid not found, or no kid with
// more than one candidate key — it forces exactly one
// Cache.Refresh(ctx, jwksURL) and retries the same lookup once against
// the refreshed set, then gives up. This is deliberately bounded to one
// refresh per call: a caller sending a request with a random, never-valid
// kid gets exactly one extra issuer hit, not an unbounded chain of them —
// jwt.Parse invokes FetchKeys at most once per signature, so "one refresh
// per FetchKeys call" is "one refresh per Verify call" for the compact
// (single-signature) JWTs this verifier handles.
func (p rs256KeyProvider) FetchKeys(ctx context.Context, sink jws.KeySink, sig *jws.Signature, _ *jws.Message) error {
	set, err := p.cache.Lookup(ctx, p.jwksURL)
	if err != nil {
		return fmt.Errorf("looking up cached JWKS: %w", err)
	}

	kid, hasKid := sig.ProtectedHeaders().KeyID()

	key, lookupErr := lookupRS256Key(set, kid, hasKid)
	if lookupErr != nil {
		// Whatever's cached doesn't have this key — force exactly one
		// refresh (never a loop) and retry against the result, since the
		// miss may simply mean the issuer rotated keys since the cache
		// was last populated (I7).
		refreshed, refreshErr := p.cache.Refresh(ctx, p.jwksURL)
		if refreshErr != nil {
			return fmt.Errorf("%w (forced JWKS refresh also failed: %v)", lookupErr, refreshErr)
		}
		key, lookupErr = lookupRS256Key(refreshed, kid, hasKid)
		if lookupErr != nil {
			return lookupErr
		}
	}

	sink.Key(jwa.RS256(), key)
	return nil
}

// lookupRS256Key finds the candidate verification key within set for a
// signature that declared kid (hasKid distinguishes "no kid header" from
// "empty kid header", both of which fall back to the single-key case).
func lookupRS256Key(set jwk.Set, kid string, hasKid bool) (jwk.Key, error) {
	if hasKid && kid != "" {
		k, found := set.LookupKeyID(kid)
		if !found {
			return nil, fmt.Errorf("no JWKS key found for kid=%q", kid)
		}
		return k, nil
	}
	if set.Len() == 1 {
		k, _ := set.Key(0)
		return k, nil
	}
	return nil, errors.New("token has no kid and JWKS does not have exactly one key")
}
