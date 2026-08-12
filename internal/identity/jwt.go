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
func (v *jwxVerifier) Verify(ctx context.Context, token string) (string, error) {
	keySet, err := v.cache.Lookup(ctx, v.jwksURL)
	if err != nil {
		return "", fmt.Errorf("looking up cached JWKS: %w", err)
	}

	parsed, err := jwt.Parse([]byte(token),
		jwt.WithKeyProvider(rs256KeyProvider{set: keySet}),
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

// rs256KeyProvider selects a verification key from a jwk.Set by the
// token's `kid` header (falling back to the sole key when the set has
// exactly one and the token has none), and always pins jwa.RS256 as the
// algorithm to verify with (I6) — regardless of what the token's own
// `alg` header or the JWK's own `alg` field claims. This is what defends
// against algorithm-confusion attacks: an attacker cannot get this
// verifier to attempt anything other than RS256, no matter what the
// token's header says.
type rs256KeyProvider struct {
	set jwk.Set
}

func (p rs256KeyProvider) FetchKeys(_ context.Context, sink jws.KeySink, sig *jws.Signature, _ *jws.Message) error {
	var key jwk.Key
	if kid, ok := sig.ProtectedHeaders().KeyID(); ok && kid != "" {
		k, found := p.set.LookupKeyID(kid)
		if !found {
			return fmt.Errorf("no JWKS key found for kid=%q", kid)
		}
		key = k
	} else if p.set.Len() == 1 {
		k, _ := p.set.Key(0)
		key = k
	} else {
		return errors.New("token has no kid and JWKS does not have exactly one key")
	}

	sink.Key(jwa.RS256(), key)
	return nil
}
