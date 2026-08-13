package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
	"github.com/mildronize/my-template/internal/transport/bff"
)

// discardLogger builds a slog.Logger that writes nowhere — this file's
// tests assert on HTTP responses and DB state, not log lines.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testDistFS stands in for the real npm-built SPA embed (web.DistFS) that
// buildHandler otherwise wires into the SPA fallback. A genuinely fresh
// clone has never run `npm run build`, so web/dist holds only the tracked
// .gitkeep placeholder -- asserting this file's "non-API path still falls
// through to the SPA" subtest against the real embed would make that
// assertion's result depend on out-of-band state nothing guarantees (the
// exact bug newSPAHandler's own doc comment already warns about, from the
// other direction). fstest.MapFS is the standard library's way to supply a
// synthetic filesystem instead, so the SPA fallback is exercised (a real
// index.html is served for an unmatched path) without any dependency on a
// prior build step.
func testDistFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>test fixture</html>")},
	}
}

// TestBFF_UnmatchedAPIBFFPathAnswers404NotTheSPA guards a real interaction
// this task's own manual verification found: wireBFF mounts the embedded
// SPA on router.NoRoute (milestone-3/task-1), which is router-wide, not
// scoped to "/" — without the /api/bff/ prefix carve-out in wireBFF's own
// NoRoute handler, an unmapped path under /api/bff/ (a typo, or
// milestone-3/task-2's own negative-check target, POST /api/bff/keys)
// would silently fall through to the SPA fallback and answer 200
// text/html instead of a real 404, undermining GOAL.md Done-when 5's
// negative check the moment it's checked against this actual assembled
// router (internal/transport/bff's own test router,
// bff_testutil_test.go's newTestRouter, never registers a NoRoute handler
// at all, so its own negative_check_test.go never observes this
// interaction).
func TestBFF_UnmatchedAPIBFFPathAnswers404NotTheSPA(t *testing.T) {
	db := newMainTestDB(t)
	cfg := &platform.Config{
		Port:          8080,
		DatabasePath:  "unused-in-this-test",
		SessionSecret: "test-session-secret",
	}
	handler, err := buildHandler(context.Background(), cfg, db, discardLogger(), testDistFS())
	require.NoError(t, err)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/bff/keys"},
		{http.MethodPost, "/api/bff/keys/rotate"},
		{http.MethodPost, "/api/bff/keys/some-id/rotate"},
		{http.MethodGet, "/api/bff/this-route-does-not-exist"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code, "an unmapped /api/bff/ path must 404, never fall through to the SPA")
			assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"),
				"must answer the JSON error envelope, not the SPA's text/html index.html")
		})
	}

	t.Run("non-API path still falls through to the SPA", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code, "milestone-3/task-1's own SPA fallback must still serve client-side routes")
	})
}

// TestBFF_FullCRUDRoundTrip_ThroughAssembledMainHandler is this task's
// verification that the real create->list->get->update->delete round
// trip (GOAL.md Done-when 2) holds through cmd/server's actual composed
// handler (buildHandler/wireBFF), not only internal/transport/bff's own
// isolated test router (internal/transport/bff/todo_handler_test.go's
// TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie already covers that
// layer) — the same session-seeding shortcut
// (bff.Signer.NewSessionCookie, called directly) reused throughout this
// repo's own tests, driven here through http.Handler.ServeHTTP so it
// exercises main.go's actual route composition end to end.
func TestBFF_FullCRUDRoundTrip_ThroughAssembledMainHandler(t *testing.T) {
	db := newMainTestDB(t)
	cfg := &platform.Config{
		Port:          8080,
		DatabasePath:  "unused-in-this-test",
		SessionSecret: "test-session-secret-for-main-round-trip",
	}
	handler, err := buildHandler(context.Background(), cfg, db, discardLogger(), testDistFS())
	require.NoError(t, err)

	repo := identity.NewRepo(db)
	sub := "main-round-trip-sub"
	owner, err := repo.CreateUser(context.Background(), "main-round-trip-owner", "owner", &sub)
	require.NoError(t, err)

	signer := bff.NewSigner([]byte(cfg.SessionSecret))
	sessionValue, err := signer.NewSessionCookie(owner.ID)
	require.NoError(t, err)

	doReq := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.AddCookie(&http.Cookie{Name: "session", Value: sessionValue})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	decodeField := func(t *testing.T, body []byte, field string) string {
		t.Helper()
		var m map[string]any
		require.NoError(t, json.Unmarshal(body, &m))
		v, ok := m[field].(string)
		require.Truef(t, ok, "response body has no string field %q: %s", field, string(body))
		return v
	}

	createRec := doReq(http.MethodPost, "/api/bff/todos", []byte(`{"title":"main-handler round trip"}`))
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())
	id := decodeField(t, createRec.Body.Bytes(), "id")

	listRec := doReq(http.MethodGet, "/api/bff/todos", nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	assert.Contains(t, listRec.Body.String(), id, "the created todo must actually be listed back")

	getRec := doReq(http.MethodGet, "/api/bff/todos/"+id, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	patchRec := doReq(http.MethodPatch, "/api/bff/todos/"+id, []byte(`{"done":true}`))
	require.Equal(t, http.StatusOK, patchRec.Code)
	assert.Contains(t, patchRec.Body.String(), `"done":true`)

	getAfterPatchRec := doReq(http.MethodGet, "/api/bff/todos/"+id, nil)
	require.Equal(t, http.StatusOK, getAfterPatchRec.Code)
	assert.Contains(t, getAfterPatchRec.Body.String(), `"done":true`, "the update must actually be persisted, not just echoed by the patch response")

	deleteRec := doReq(http.MethodDelete, "/api/bff/todos/"+id, nil)
	require.Equal(t, http.StatusNoContent, deleteRec.Code)

	getAfterDeleteRec := doReq(http.MethodGet, "/api/bff/todos/"+id, nil)
	assert.Equal(t, http.StatusNotFound, getAfterDeleteRec.Code, "the delete must actually have removed the row")
}
