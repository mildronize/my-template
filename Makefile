# BIN_DIR is a repo-local install target for pinned dev tools (sqlc,
# goose, oapi-codegen), kept out of $(go env GOPATH)/bin so a fork's
# pinned versions never collide with another repo's on the same machine.
BIN_DIR := $(CURDIR)/bin

# GO_PKGS is `./...` minus anything under node_modules. web/ sits inside
# this module's root, and an npm dependency can ship its own .go file
# (e.g. web/node_modules/flatted/golang/pkg/flatted/flatted.go, present
# as of milestone-3) — `go list`/`./...` has no concept of "npm
# dependency" and doesn't skip node_modules the way it skips testdata/
# and dot- or underscore-prefixed directories, so every Go tool that
# resolves packages from the module root would otherwise pick it up.
# Harmless today because that file happens to compile/vet cleanly, but
# `npm install` should never be able to introduce code that affects the
# Go build, vet, or test surface — use $(GO_PKGS) instead of ./... in
# vet/test below. Do not "simplify" this back to ./... — that silently
# reopens the exposure the moment some transitive npm package ships a
# .go file that doesn't compile/vet/pass cleanly.
GO_PKGS = $(shell go list ./... | grep -v '/node_modules/')

# GO_BUILD_PKGS is GO_PKGS with one more filter, needed only for `go
# build`: internal/ itself holds nothing but _test.go files
# (architecture_test.go, invariants_test.go), so it has no non-test Go
# files to build. `go build ./...` silently skips a test-only package
# when the package set comes from pattern expansion, but once GO_PKGS
# materializes that set into an explicit list of import paths, `go
# build` treats each one as a directly-named target and errors with
# "no non-test Go files in ..." instead of skipping it — `go vet` and
# `go test` don't have this quirk (they both want test files), so
# GO_PKGS is fine as-is for vet/test.
GO_BUILD_PKGS = $(shell go list -f '{{if .GoFiles}}{{.ImportPath}}{{end}}' ./... | grep -v '/node_modules/')

.PHONY: tools
tools:
	GOBIN=$(BIN_DIR) go install github.com/sqlc-dev/sqlc/cmd/sqlc
	GOBIN=$(BIN_DIR) go install github.com/pressly/goose/v3/cmd/goose
	GOBIN=$(BIN_DIR) go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

# generate re-runs every codegen source against sqlc (db/queries +
# db/migrations), huma (the public API's own code, dumping openapi.yaml),
# and oapi-codegen (bff-openapi.yaml, still hand-authored input for the
# BFF surface until a later ticket migrates it too) so the committed
# generated output (internal/db, openapi.yaml, internal/bffapi/
# bffapi.gen.go) can be regenerated reproducibly rather than hand-edited.
#
# The internal/db cleanup step below runs first: sqlc emits one .sql.go
# file per db/queries/*.sql file, so deleting a query file (e.g. on fork,
# removing db/queries/todos.sql per docs/GETTING-STARTED.md Step 5)
# leaves its old internal/db/todos.sql.go orphaned — sqlc has no reason
# to touch or remove a file it's not generating anymore, and an orphaned
# file referencing a dropped table breaks the build until removed by
# hand. Every file directly under internal/db is sqlc output per
# sqlc.yaml's `out: "internal/db"` (nothing hand-written lives there) —
# still, only files actually carrying sqlc's own "// Code generated"
# marker are removed, so this stays safe even if that ever stops being
# true.
#
# story-1/ticket-6: `go run ./cmd/server openapi v1` replaces the old
# `oapi-codegen -generate types,gin,spec -package api -o
# internal/api/openapi.gen.go openapi.yaml` line — internal/api is gone
# (contract.md's "File/package layout"), and openapi.yaml itself flips
# from hand-authored input to huma-derived output (contract.md's
# "OpenAPI spec output" section). Deliberately the "v1"-scoped form of the
# "openapi" command, not the bare `go run ./cmd/server openapi` ticket-5
# originally built (main.go's own doc comment on that command explains
# why): apiBFF has zero real operations
# registered until a later ticket migrates the BFF surface too, so dumping
# its half here would overwrite the hand-authored bff-openapi.yaml the
# oapi-codegen line below still needs as real input — the exact landmine
# ticket-5's own report flagged forward ("tickets 6 and 7 each wire their
# own surface's half once that surface no longer depends on the
# hand-authored file"). That line itself is otherwise unchanged: BFF
# still generates its own ServerInterface/Error/RegisterHandlers into its
# own package (internal/bffapi) from a distinct package name, same
# reasoning as always (`_contract/API.md`'s "Two specs, not one") — it
# just no longer has an internal/api sibling to distinguish itself from.
.PHONY: generate
generate:
	@for f in internal/db/*.go; do \
		[ -f "$$f" ] || continue; \
		grep -q '^// Code generated' "$$f" && rm -f "$$f"; \
	done
	$(BIN_DIR)/sqlc generate
	go run ./cmd/server openapi v1
	$(BIN_DIR)/oapi-codegen -generate types,gin,spec -package bffapi -o internal/bffapi/bffapi.gen.go bff-openapi.yaml

# web-build runs Vite's production build (web/dist) — split out of
# `build` below so it's independently invokable (e.g. from the Dockerfile
# stage that builds the frontend separately from the Go binary).
# `npm ci` (not `npm install`) for the same reproducibility reason
# `tools` pins exact tool versions via go.mod/go.sum: web/package-lock.json
# is the source of truth for exactly which dependency versions this
# build uses, and `npm ci` refuses to proceed if it and web/package.json
# have drifted apart, rather than silently resolving something new.
.PHONY: web-build
web-build:
	cd web && npm ci && npm run build

# build's ordering is deliberate, not incidental: cmd/server embeds
# web/dist at Go compile time (web/embed.go's //go:embed directive), so
# whatever's on disk under web/dist *when go build runs* is what ships —
# running `go build` before `npm run build` (or not at all) would bake in
# a stale or placeholder-only SPA silently, no error, no warning
# (.chief/milestone-3/_goal/GOAL.md Done-when 1). web-build must finish
# before go build starts every time, hence one target listing both rather
# than two independent ones a caller might reorder or skip.
.PHONY: build
build: web-build
	go build $(GO_BUILD_PKGS)

.PHONY: vet
vet:
	go vet $(GO_PKGS)

# web-test runs web/'s own Vitest suite (`npm test` → `vitest run`,
# web/package.json) — its own target so it's independently invokable, the
# same reasoning web-build already follows. `npm ci` (not `npm install`)
# runs first, same reproducibility reason as web-build: a fresh clone has
# no web/node_modules yet, and this makes `make test` self-contained
# rather than silently depending on someone having run `npm install` by
# hand first.
#
# `test` depends on it so `make test` runs both suites together
# (milestone-3/task-4, closing GOAL.md's "`make test` and the two-suites
# problem" — a second green light next to `go test ./...` proves nothing
# about the first one unless something actually runs both). JS coverage
# here is deliberately partial — one test per replaced hook (GOAL.md
# Done-when 10), not full coverage of web/ — `make test` passing is a
# claim that neither suite is silently skipped, not a claim about how
# exhaustive either one is.
.PHONY: web-test
web-test:
	cd web && npm ci && npm test

.PHONY: test
test: web-test
	go test $(GO_PKGS)

# fmt-check is a raw filesystem walk (gofmt), not a go list/./...
# package resolution — GO_PKGS above has no effect on it, so it needs
# its own, separate exclusion of web/node_modules. Walking only
# `git ls-files '*.go'` keeps this to files the project actually
# tracks, which has the same effect (node_modules is untracked, so it's
# never walked) plus the added benefit of never flagging build
# artifacts or other untracked .go files that might land in the tree.
.PHONY: fmt-check
fmt-check:
	gofmt -l $$(git ls-files '*.go')

.PHONY: run
run:
	go run ./cmd/server

# smoke runs cmd/smoke against an already-running instance of this
# service — it does NOT start one itself (see cmd/smoke/main.go's own
# usage comment for why, and what it assumes about DATABASE_PATH/.env
# matching that instance). Point it elsewhere with SMOKE_BASE_URL, e.g.
# `SMOKE_BASE_URL=https://host make smoke`; defaults to
# http://localhost:8080, matching Config.Port's own PORT default.
.PHONY: smoke
smoke:
	go run ./cmd/smoke

# e2e is TPL-3's browser suite — unlike smoke (above), it manages its own
# lifecycle end to end: brings up a real local OIDC issuer, builds and
# starts a real instance of this service against it, runs, tears
# everything down. See e2e/README.md for what it proves and doesn't, and
# why it lives in its own e2e/package.json rather than at the repo root
# or inside web/package.json.
.PHONY: e2e
e2e:
	cd e2e && npm ci && npm run test:e2e
