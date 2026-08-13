// TPL-1 milestone-3/task-1: serves the Vite-built SPA (web.DistFS,
// embedded at Go build time — see web/embed.go) from bffRouter's NoRoute
// handler (wireBFF, main.go). "NoRoute" is what makes this fit the
// existing router structure without adding a new mux entry: bffRouter
// already owns "/" at the stdlib http.ServeMux level (buildHandler), and
// anything that isn't one of bff's own explicit routes (/login, /callback)
// falls through to NoRoute — which is exactly "any path not already
// claimed by /api/v1 or the existing bff routes" from this task's spec.
package main

import (
	"io/fs"
	"net/http"
	"net/url"
	"strings"
)

// newSPAHandler builds the standard single-page-app fallback handler: a
// request for a path that resolves to a real built asset (e.g.
// /assets/index-XXXX.js) is served as that file; anything else (e.g.
// /settings, or "/" itself) serves index.html verbatim so the SPA's own
// client-side router (react-router, web/src/App.tsx) can take over —
// there is no server-side route for /settings to 404 on, the same reason
// every SPA-behind-a-file-server needs this fallback.
//
// web/dist's real content only exists after `npm run build` has run
// before this binary was compiled (web/embed.go's own comment, Makefile's
// `build` target ordering) — a build that skipped that step still embeds
// something (the tracked dist/.gitkeep placeholder), so this handler
// degrades to a 404 on every request rather than failing to start.
//
// distFS is the already-fs.Sub'd dist filesystem to serve, not the raw
// embed — main.go's wireBFF passes fs.Sub(web.DistFS, "dist") at the call
// site, which is the real embedded SPA build in production. This
// constructor takes an fs.FS rather than reaching into web.DistFS itself
// so a test can substitute a synthetic filesystem (e.g. fstest.MapFS)
// without depending on a real `npm run build` having run first — see
// cmd/server/bff_negative_check_test.go's own note on why that matters.
func newSPAHandler(distFS fs.FS) (http.Handler, error) {
	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRealFile(distFS, r.URL.Path) {
			fileServer.ServeHTTP(w, r)
			return
		}

		indexReq := r.Clone(r.Context())
		indexReq.URL = &url.URL{Path: "/"}
		fileServer.ServeHTTP(w, indexReq)
	}), nil
}

// isRealFile reports whether urlPath names an actual (non-directory) file
// in fsys — true for a built asset like /assets/index-XXXX.js, false for
// a client-side route like /settings that has no file behind it.
func isRealFile(fsys fs.FS, urlPath string) bool {
	p := strings.TrimPrefix(urlPath, "/")
	if p == "" {
		p = "."
	}
	info, err := fs.Stat(fsys, p)
	return err == nil && !info.IsDir()
}
