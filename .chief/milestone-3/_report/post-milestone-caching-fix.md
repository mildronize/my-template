# Post-milestone fix: SPA cache headers and `/assets/` 404 behavior

Not part of any numbered milestone-3 task — a standalone hardening fix
found during post-milestone review, confirmed against a running binary
before any code changed. Milestone-3 was already complete and merged into
`milestone-2/close-parity-gap` when this was found; this fix landed on top
of that, on the same branch.

## Finding

`cmd/server/spa.go`'s `newSPAHandler` serves the Vite-built SPA via
`http.FileServer` over an `fs.FS` backed by `embed.FS`. Two real gaps,
both confirmed live before any fix:

1. **No `Cache-Control` header anywhere.** `http.FileServer` over an
   `embed.FS`-backed `fs.FS` sets no `Cache-Control`, and — since
   `embed.FS` carries no meaningful mtimes — no `Last-Modified`/`ETag`
   either. `curl -i http://localhost:PORT/` showed no `Cache-Control` in
   the response at all, on `index.html` or on fingerprinted assets under
   `/assets/`.
2. **A request for a deleted/renamed asset under `/assets/` answered
   `200 text/html` instead of `404`.** `curl -i
   http://localhost:PORT/assets/nope-deleted-hash.js` returned `200`,
   `Content-Type: text/html`, byte-identical in shape to the real
   `index.html` response — the old fallback logic couldn't distinguish
   "no server-side route, hand off to react-router" (correct for
   `/settings`, `/`) from "this was a real asset path and it's genuinely
   gone" (should always be a real 404 — asset paths are content-hashed, a
   miss there is never a legitimate client-side route).

**Why this matters**: a stale cached `index.html` in a browser (nothing
prevented that) references asset filenames from an older build. If a
redeploy changes those hashes, the stale `index.html` requests a bundle
that no longer exists — and pre-fix, that request got `200 text/html`
back instead of a clean `404`, so a browser trying to parse HTML as
JavaScript produced an obscure console error instead of a readable
failure, and the browser's own cache-miss/reload recovery never got a
chance to kick in.

## Fix

Three parts, all in `cmd/server/spa.go`:

1. **`index.html` gets `Cache-Control: no-cache`** — not `no-store`.
   `no-cache` still lets the browser keep a cached copy but forces
   revalidation before using it: cheap conditional requests, but never a
   silently-served stale shell. Applied whenever the response body is
   `index.html`'s content — both a direct `GET /` and any client-side
   route (e.g. `/settings`) that falls through to the SPA fallback.
2. **`/assets/` misses now return a real `404`, never the SPA fallback.**
   A new `assetsPrefix = "/assets/"` constant gates a dedicated branch:
   under that prefix, a hit serves the file (see part 3), a miss calls
   `http.NotFound` directly — no fallthrough to `index.html`. Every other
   path keeps the pre-existing real-file-or-index.html fallback logic
   unchanged.
3. **Real assets under `/assets/` get `Cache-Control: public,
   max-age=31536000, immutable`.** Confirmed safe before adding: a fresh
   `npm run build` was run and `web/dist/assets/` inspected directly —
   both emitted files (`index-C3ywW6A5.js`, `index-B4x7aGNN.css`) carry a
   Vite content hash in the filename. `web/vite.config.ts` has no custom
   `rollupOptions.output.assetFileNames` overriding Vite's default output
   naming, so every file Vite ever routes through `dist/assets` — not
   just these two — gets a content hash. A new build always produces a
   new filename, so aggressive caching here carries no staleness risk.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` — all clean.
- `go test ./...` — all packages pass, including `cmd/server`'s existing
  suite (`bff_negative_check_test.go`, `main_test.go`) unchanged. Neither
  existing test asserted the absence of a `Cache-Control` header or the
  old assets-fall-through-to-index.html behavior, so nothing needed
  updating for compatibility.
- New regression tests added, `cmd/server/spa_test.go`:
  - `TestNewSPAHandler_IndexHTMLGetsNoCacheHeader` — `/` and `/settings`
    both get `Cache-Control: no-cache`.
  - `TestNewSPAHandler_AssetsMissAnswersRealNotFound` — a missing
    `/assets/` path answers a genuine `404`, not `index.html`'s content.
  - `TestNewSPAHandler_RealAssetGetsLongLivedCacheHeader` — a real
    `/assets/` hit gets the long-lived, immutable cache header.
- Real binary, real build, live curl checks (server run on a scratch port
  in `/tmp`, never touching the live instance already serving from this
  branch's head on port 8080 — confirmed same pid before and after):

  ```
  $ curl -si http://localhost:18734/
  HTTP/1.1 200 OK
  Cache-Control: no-cache
  Content-Type: text/html; charset=utf-8
  ...

  $ curl -si http://localhost:18734/settings
  HTTP/1.1 200 OK
  Cache-Control: no-cache
  Content-Type: text/html; charset=utf-8
  ...

  $ curl -si http://localhost:18734/assets/index-C3ywW6A5.js
  HTTP/1.1 200 OK
  Cache-Control: public, max-age=31536000, immutable
  Content-Type: text/javascript; charset=utf-8
  Content-Length: 475352
  ...

  $ curl -si http://localhost:18734/assets/nope-deleted-hash.js
  HTTP/1.1 404 Not Found
  Content-Type: text/plain; charset=utf-8
  X-Content-Type-Options: nosniff
  Content-Length: 19

  404 page not found
  ```

- `docker compose up` sanity check: built and ran a temporary,
  port-remapped standalone compose file (host port `18735`, never
  `8080`, project name `spa-verify` so it never collided with anything
  real) against the actual `Dockerfile`/build context. Container built,
  started, `GET /healthz` → `200 {"status":"ok"}`, `GET /` → `200` with
  `Cache-Control: no-cache`. Torn down afterward (`down -v`, image
  removed) — no trace left behind.
- The live instance already serving this branch's head on port 8080 was
  never started, stopped, or otherwise touched at any point during this
  fix — confirmed by checking its pid before and after all of the above.

## What was NOT done

- No service worker, manifest, or other caching infrastructure — just the
  two headers described above, proportionate to the actual finding.
- No deployment of any kind — this is a code change only, verified
  locally/in scratch. Deployment timing is Clara's call.
- Nothing outside `cmd/server/spa.go` and its new test file was touched.

## Commits pushed (branch `milestone-2/close-parity-gap`)

- `85c82ef` — `fix(post-milestone-3): SPA index.html no-cache, /assets/ 404s instead of falling through`
- (this file) — `fix(post-milestone-3): document the SPA caching/404 fix`
