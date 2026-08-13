package bff

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
	"github.com/mildronize/my-template/internal/transport/publicapi"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver for these tests
)

// repoRootForTests resolves the module root from this test file's own
// location (mirrors internal/transport/publicapi's own testutil), so
// tests work regardless of the directory `go test` is invoked from.
func repoRootForTests(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine this test file's own location")
	// this file lives at <root>/internal/transport/bff/<this file>.go
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
}

// newTestDB opens a fresh temp-file SQLite database and applies every
// migration under db/migrations via goose, mirroring internal/domain/
// todo's and internal/transport/publicapi's own testutils — this
// package's tests are integration tests against a real schema, not fakes.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, goose.SetDialect("sqlite3"))
	migrationsDir := filepath.Join(repoRootForTests(t), "db", "migrations")
	require.NoError(t, goose.Up(conn, migrationsDir))

	return conn
}

// testLogger is a slog.Logger that discards output — these tests assert
// on HTTP responses and DB state, not log lines, and a real logger would
// just be noise in `go test -v`.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// seedUser inserts a users row directly (bypassing internal/identity's own
// repo, the same reasoning internal/domain/todo's testutil gives for its
// own raw-INSERT fixture helper: these tests shouldn't need to trust a
// second package's write path just to set up a fixture, and a plain
// INSERT keeps the I4 table-ownership boundary these tests partly exist to
// demonstrate).
func seedUser(t *testing.T, conn *sql.DB, role, ssoSubject string, active bool) identity.User {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.NewString()
	handle := "user-" + id[:8]
	_, err := conn.Exec(
		`INSERT INTO users (id, handle, role, active, sso_subject, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, handle, role, active, ssoSubject, now, now,
	)
	require.NoError(t, err)
	return identity.User{ID: id, Handle: handle, Role: role, Active: active, SSOSubject: ssoSubject, CreatedAt: now, UpdatedAt: now}
}

// seedTodo inserts a todos row directly, owned by ownerID.
func seedTodo(t *testing.T, conn *sql.DB, ownerID, title string) string {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.NewString()
	_, err := conn.Exec(
		`INSERT INTO todos (id, owner_id, title, done, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, ownerID, title, false, now, now,
	)
	require.NoError(t, err)
	return id
}

// --- fake Hydra: JWKS + token endpoint, no live issuer anywhere ---------
//
// Same reasoning internal/identity/jwt_test.go already established for its
// own fake JWKS server (task-4.md's verification section: "No live Hydra
// call in any automated test — mock the OIDC endpoints"). This fake serves
// exactly the two endpoints internal/transport/bff's own code touches:
// /.well-known/jwks.json (identity.NewJWTVerifier's own construction) and
// /oauth2/token (callback_handler.go's oauthConfig(cfg).Exchange). Nothing
// here ever calls /oauth2/auth — the I11 tests below assert the *shape* of
// the URL login_handler.go builds without a browser ever following it.

// newRSAKeyAndJWKS mirrors internal/identity/jwt_test.go's own helper of
// the same name (unexported, package-private there — not reusable across
// the package boundary, hence this local copy).
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

// signIDToken builds an RS256-signed JWT shaped like a Hydra id_token:
// iss=issuer, aud=[clientID] (an id_token's audience is the OAuth
// client_id per OIDC — see callback_handler.go's own doc comment), sub as
// given.
func signIDToken(t *testing.T, priv *rsa.PrivateKey, kid, issuer, clientID, sub string, exp time.Time) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{clientID}).
		Subject(sub).
		Expiration(exp).
		Build()
	require.NoError(t, err)

	hdrs := jws.NewHeaders()
	require.NoError(t, hdrs.Set(jws.KeyIDKey, kid))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), priv, jws.WithProtectedHeaders(hdrs)))
	require.NoError(t, err)
	return string(signed)
}

// fakeIDP is a fake Hydra serving JWKS + the token endpoint used by
// oauthConfig(cfg).Exchange. Its sub can be changed between requests
// (test cases that need a specific sub in the id_token, e.g. an agent's
// sso_subject) and it counts token-endpoint hits so the I11 tests can
// assert the exchange was never attempted.
type fakeIDP struct {
	srv       *httptest.Server
	priv      *rsa.PrivateKey
	kid       string
	clientID  string
	sub       atomic.Value // string
	tokenHits atomic.Int64
}

func newFakeIDP(t *testing.T, clientID string) *fakeIDP {
	t.Helper()
	priv, jwksJSON := newRSAKeyAndJWKS(t, "bff-test-key")
	f := &fakeIDP{priv: priv, kid: "bff-test-key", clientID: clientID}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksJSON)
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		f.tokenHits.Add(1)
		require.NoError(t, r.ParseForm())

		sub, _ := f.sub.Load().(string)
		idToken := signIDToken(t, f.priv, f.kid, f.srv.URL, clientID, sub, time.Now().Add(time.Hour))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// setSub controls what `sub` the next token-endpoint response's id_token
// carries.
func (f *fakeIDP) setSub(sub string) { f.sub.Store(sub) }

// testConfig builds a *platform.Config pointed at f, with a real
// SessionSecret (never empty in a test — see session.go's NewSigner doc
// comment on why an empty key is never acceptable) and a placeholder
// AuthAudience (only used to build the redirect_uri; nothing in these
// tests dereferences it as a real URL).
func (f *fakeIDP) testConfig() *platform.Config {
	return &platform.Config{
		SSOIssuer:       f.srv.URL,
		SSOClientID:     f.clientID,
		SSOClientSecret: "test-client-secret",
		AuthAudience:    "https://app.example.test",
		SessionSecret:   "test-session-secret-not-for-production",
	}
}

// newIDVerifier builds the same identity.JWTVerifier
// callback_handler.go's own idVerifier parameter expects — audienced to
// f's client id, exactly like cmd/server/main.go's wireBFF builds it.
func newIDVerifier(t *testing.T, f *fakeIDP) identity.JWTVerifier {
	t.Helper()
	v, err := identity.NewJWTVerifier(t.Context(), f.srv.URL, f.clientID)
	require.NoError(t, err)
	return v
}

// newTestRouter wires internal/transport/bff's routes directly (bypassing
// cmd/server/main.go's wireBFF, which lives in package main and isn't
// importable from here) onto a bare gin.Engine — these tests exist to
// prove this package's own handlers, not main.go's composition, which
// Done-when 3's own test (cmd/server/main_test.go) covers separately.
//
// identitySvc (milestone-3/task-2, new) backs the /api/bff group's
// KeysServer (ListKeys/RevokeKey) the same way todoSvc backs its
// TodoServer — nil is an acceptable value for any test that only
// exercises the todo or me endpoints, since KeysServer.Service is never
// dereferenced unless a keys route is actually hit.
func newTestRouter(cfg *platform.Config, signer *Signer, idVerifier identity.JWTVerifier, repo *identity.Repo, todoSvc *todo.Service, identitySvc *identity.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	logger := testLogger()
	r.GET("/login", NewLoginHandler(cfg, signer, logger))
	r.GET("/callback", NewCallbackHandler(cfg, signer, idVerifier, repo, logger))
	r.GET("/", RequireSession(signer, repo, logger), NewViewHandler(todoSvc))

	// /api/bff mirrors cmd/server/main.go's wireBFF composition exactly
	// (RejectActorFields -> RequireJSONSession -> bff-openapi.yaml's
	// request validator -> the composed ServerInterface) so these tests
	// exercise the real middleware chain, not a simplified stand-in.
	bffValidator, err := bffapi.RequestValidator()
	if err != nil {
		panic(fmt.Sprintf("bff_testutil_test.go: building bff-openapi request validator: %v", err))
	}
	apiBFF := r.Group("/api/bff")
	apiBFF.Use(publicapi.RejectActorFields(), RequireJSONSession(signer, repo, logger), bffValidator)
	bffapi.RegisterHandlers(apiBFF, testBFFServer{
		MeServer:   MeServer{},
		KeysServer: NewKeysServer(identitySvc),
		TodoServer: NewTodoServer(todoSvc),
	})

	return r
}

// testBFFServer mirrors cmd/server/main.go's own unexported bffServer
// composite (that type lives in package main and isn't importable from
// here) — same embedding, same reasoning: no method names collide across
// MeServer/KeysServer/TodoServer, so plain embedding satisfies
// bffapi.ServerInterface with no hand-written delegation.
type testBFFServer struct {
	MeServer
	*KeysServer
	*TodoServer
}

var _ bffapi.ServerInterface = testBFFServer{}

// doBFFJSONRequest issues an HTTP request against the /api/bff JSON
// surface, presenting sessionValue (if non-empty) as this package's own
// signed session cookie (session.go's sessionCookieName) — the BFF
// surface's own auth mechanism, distinct from internal/transport/
// publicapi's doJSONRequest (which presents an Authorization: Bearer
// header instead), mirroring that helper's shape for everything else
// (JSON body encoding, response recording).
func doBFFJSONRequest(t *testing.T, router *gin.Engine, method, path, sessionValue string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if sessionValue != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionValue})
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// decodeBFFError decodes a bffapi.Error-shaped response body — generic
// across every domain on this surface, mirrors internal/transport/
// publicapi's own decodeError.
func decodeBFFError(t *testing.T, rec *httptest.ResponseRecorder) bffapi.Error {
	t.Helper()
	var got bffapi.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}
