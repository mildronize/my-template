package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spaTestDistFS builds a synthetic dist filesystem with a real asset under
// assets/ (standing in for a Vite content-hashed file like
// index-XXXX.js) alongside index.html, so these tests can exercise both
// the /assets/ hit path and the /assets/ miss path without depending on a
// real `npm run build` having run first -- same reasoning as
// main_test.go's testDistFS.
func spaTestDistFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte("<html>test fixture</html>")},
		"assets/index-abc123.js": &fstest.MapFile{Data: []byte("console.log('fixture');")},
	}
}

// TestNewSPAHandler_IndexHTMLGetsNoCacheHeader guards this fix's Part 1:
// index.html must always carry Cache-Control: no-cache, both when "/" is
// requested directly and when a client-side route (e.g. /settings, which
// has no server-side handler) falls through to index.html so
// react-router can take over client-side. Before this fix, no
// Cache-Control header was set at all -- a browser could hold onto a
// stale index.html indefinitely, referencing asset hashes a later
// redeploy had already deleted.
func TestNewSPAHandler_IndexHTMLGetsNoCacheHeader(t *testing.T) {
	handler, err := newSPAHandler(spaTestDistFS())
	require.NoError(t, err)

	for _, path := range []string{"/", "/settings"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"),
				"index.html (served directly or via the SPA fallback) must always be revalidated, never served from a stale browser cache")
		})
	}
}

// TestNewSPAHandler_AssetsMissAnswersRealNotFound guards this fix's Part
// 2: a request under /assets/ for a file that doesn't exist must answer a
// genuine 404, never fall through to index.html. Before this fix, any
// path without a matching real file -- including a deleted/renamed
// content-hashed asset -- fell through to the SPA's index.html fallback
// and answered 200 text/html. Since /assets/ is exclusively Vite's
// content-hashed build output (see spa.go's assetsPrefix comment), a miss
// there is never a legitimate client-side route the way /settings is --
// it's always a real miss, and must 404 like one.
func TestNewSPAHandler_AssetsMissAnswersRealNotFound(t *testing.T) {
	handler, err := newSPAHandler(spaTestDistFS())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/assets/nope-deleted-hash.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"a miss under /assets/ must be a real 404, never the SPA's index.html fallback")
	assert.NotContains(t, rec.Body.String(), "test fixture",
		"a 404 under /assets/ must not silently serve index.html's content")
}

// TestNewSPAHandler_RealAssetGetsLongLivedCacheHeader guards this fix's
// Part 3: a real hit under /assets/ gets a long-lived, cache-forever
// Cache-Control header, since every file there is content-hashed by Vite
// (web/vite.config.ts uses Vite's default output naming, confirmed
// against a real `npm run build`) -- a new build always produces a new
// filename, so there is no correctness risk in caching aggressively.
func TestNewSPAHandler_RealAssetGetsLongLivedCacheHeader(t *testing.T) {
	handler, err := newSPAHandler(spaTestDistFS())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"),
		"a real content-hashed asset under /assets/ is safe to cache forever -- a new build always changes the filename")
	assert.Contains(t, rec.Body.String(), "console.log", "the actual asset content must still be served, not just the header")
}
