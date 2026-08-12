package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRSAKeyAndJWKS generates a fresh RSA key pair and returns both the
// private key (for signing test tokens) and the JSON body of a JWKS
// containing only its public half under kid.
func newRSAKeyAndJWKS(t *testing.T, kid string) (*rsa.PrivateKey, []byte) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pub, err := jwk.Import(priv.Public())
	require.NoError(t, err)
	require.NoError(t, pub.Set(jwk.KeyIDKey, kid))
	require.NoError(t, pub.Set(jwk.AlgorithmKey, jwa.RS256()))

	set := jwk.NewSet()
	require.NoError(t, set.AddKey(pub))

	body, err := json.Marshal(set)
	require.NoError(t, err)
	return priv, body
}

// newJWKSServer serves whatever JSON body current currently holds at
// /.well-known/jwks.json, counting how many times it's been fetched.
func newJWKSServer(t *testing.T, current *atomic.Value, fetches *atomic.Int64) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(current.Load().([]byte))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func signRS256Token(t *testing.T, priv *rsa.PrivateKey, kid, issuer, audience, sub string, exp time.Time) []byte {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{audience}).
		Subject(sub).
		Expiration(exp).
		Build()
	require.NoError(t, err)

	hdrs := jws.NewHeaders()
	require.NoError(t, hdrs.Set(jws.KeyIDKey, kid))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), priv, jws.WithProtectedHeaders(hdrs)))
	require.NoError(t, err)
	return signed
}

// signHS256Token builds a JWT with the exact same claim shape as
// signRS256Token, but signed with HS256 and an attacker-chosen secret,
// declaring alg=HS256 and a kid that matches a *real* RSA key in the
// target JWKS. This is the classic RS256->HS256 algorithm-confusion
// attack shape: a verifier that trusted the token's own `alg` header
// would attempt HS256 verification against whatever key material it
// associates with that kid.
func signHS256Token(t *testing.T, secret []byte, kid, issuer, audience, sub string, exp time.Time) []byte {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{audience}).
		Subject(sub).
		Expiration(exp).
		Build()
	require.NoError(t, err)

	hdrs := jws.NewHeaders()
	require.NoError(t, hdrs.Set(jws.KeyIDKey, kid))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256(), secret, jws.WithProtectedHeaders(hdrs)))
	require.NoError(t, err)
	return signed
}

// TestI6_JWTAlgorithmPinnedToRS256 — I6: validation always specifies
// RS256; the token's own `alg` header is never trusted to choose it.
func TestI6_JWTAlgorithmPinnedToRS256(t *testing.T) {
	priv, jwksJSON := newRSAKeyAndJWKS(t, "key-1")

	var current atomic.Value
	current.Store(jwksJSON)
	var fetches atomic.Int64
	srv := newJWKSServer(t, &current, &fetches)

	ctx := context.Background()
	verifier, err := NewJWTVerifier(ctx, srv.URL, "https://service.example")
	require.NoError(t, err)

	good := signRS256Token(t, priv, "key-1", srv.URL, "https://service.example", "sso|agent-1", time.Now().Add(time.Hour))
	sub, err := verifier.Verify(ctx, string(good))
	require.NoError(t, err, "a legitimately RS256-signed token must verify")
	assert.Equal(t, "sso|agent-1", sub)

	// Same claims, same kid (a real, matchable key) — but signed with
	// HS256 and an attacker-chosen secret, with the header declaring
	// alg=HS256. If this service read the algorithm from the token's own
	// header, it would try HS256 here. Because RS256 is pinned explicitly
	// (I6), it must instead attempt RS256 verification against the RSA
	// public key regardless of the header, and fail.
	bad := signHS256Token(t, []byte("attacker-controlled-secret"), "key-1", srv.URL, "https://service.example", "sso|agent-1", time.Now().Add(time.Hour))
	_, err = verifier.Verify(ctx, string(bad))
	assert.Error(t, err, "a token declaring alg=HS256 must not verify even against a real key's kid")
}

// TestI7_JWKSCachedNotPinnedToOneKey — I7: the key set is refetched from
// the issuer's JWKS endpoint (cached, not fetched per-request), and a
// specific key is never hardcoded/pinned — Hydra's signing key
// regenerates on every SSO rebuild, and this verifier must follow it.
func TestI7_JWKSCachedNotPinnedToOneKey(t *testing.T) {
	privA, jwksA := newRSAKeyAndJWKS(t, "key-a")
	privB, jwksB := newRSAKeyAndJWKS(t, "key-b")

	var current atomic.Value
	current.Store(jwksA)
	var fetches atomic.Int64
	srv := newJWKSServer(t, &current, &fetches)

	ctx := context.Background()
	verifierIface, err := NewJWTVerifier(ctx, srv.URL, "https://service.example")
	require.NoError(t, err)
	verifier, ok := verifierIface.(*jwxVerifier)
	require.True(t, ok)

	require.Equal(t, int64(1), fetches.Load(), "NewJWTVerifier builds the cache once at startup, not lazily per Verify call")

	tokenA := signRS256Token(t, privA, "key-a", srv.URL, "https://service.example", "sso|agent-a", time.Now().Add(time.Hour))

	for range 3 {
		sub, err := verifier.Verify(ctx, string(tokenA))
		require.NoError(t, err)
		assert.Equal(t, "sso|agent-a", sub)
	}
	assert.Equal(t, int64(1), fetches.Load(), "repeated Verify calls must be served from the cache, not refetched each time")

	// Rotate the issuer's JWKS to an entirely different key — key-a is
	// gone. No code here names either key; the verifier must follow
	// whatever the cache currently holds after a refresh.
	current.Store(jwksB)
	_, err = verifier.cache.Refresh(ctx, verifier.jwksURL)
	require.NoError(t, err)

	tokenB := signRS256Token(t, privB, "key-b", srv.URL, "https://service.example", "sso|agent-b", time.Now().Add(time.Hour))
	sub, err := verifier.Verify(ctx, string(tokenB))
	require.NoError(t, err, "a token signed by the newly-rotated-in key must verify")
	assert.Equal(t, "sso|agent-b", sub)

	_, err = verifier.Verify(ctx, string(tokenA))
	assert.Error(t, err, "a rotated-out key must stop verifying — a verifier that pinned it would wrongly still accept this")
}

func TestJWTVerifier_WrongIssuerRejected(t *testing.T) {
	priv, jwksJSON := newRSAKeyAndJWKS(t, "key-1")
	var current atomic.Value
	current.Store(jwksJSON)
	var fetches atomic.Int64
	srv := newJWKSServer(t, &current, &fetches)

	ctx := context.Background()
	verifier, err := NewJWTVerifier(ctx, srv.URL, "https://service.example")
	require.NoError(t, err)

	token := signRS256Token(t, priv, "key-1", "https://not-the-configured-issuer", "https://service.example", "sso|agent-1", time.Now().Add(time.Hour))
	_, err = verifier.Verify(ctx, string(token))
	assert.Error(t, err)
}

func TestJWTVerifier_WrongAudienceRejected(t *testing.T) {
	priv, jwksJSON := newRSAKeyAndJWKS(t, "key-1")
	var current atomic.Value
	current.Store(jwksJSON)
	var fetches atomic.Int64
	srv := newJWKSServer(t, &current, &fetches)

	ctx := context.Background()
	verifier, err := NewJWTVerifier(ctx, srv.URL, "https://service.example")
	require.NoError(t, err)

	token := signRS256Token(t, priv, "key-1", srv.URL, "https://not-my-audience", "sso|agent-1", time.Now().Add(time.Hour))
	_, err = verifier.Verify(ctx, string(token))
	assert.Error(t, err)
}

func TestJWTVerifier_ExpiredTokenRejected(t *testing.T) {
	priv, jwksJSON := newRSAKeyAndJWKS(t, "key-1")
	var current atomic.Value
	current.Store(jwksJSON)
	var fetches atomic.Int64
	srv := newJWKSServer(t, &current, &fetches)

	ctx := context.Background()
	verifier, err := NewJWTVerifier(ctx, srv.URL, "https://service.example")
	require.NoError(t, err)

	token := signRS256Token(t, priv, "key-1", srv.URL, "https://service.example", "sso|agent-1", time.Now().Add(-time.Hour))
	_, err = verifier.Verify(ctx, string(token))
	assert.Error(t, err)
}
