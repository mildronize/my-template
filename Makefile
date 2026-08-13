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
# build/vet/test below. Do not "simplify" this back to ./... — that
# silently reopens the exposure the moment some transitive npm package
# ships a .go file that doesn't compile/vet/pass cleanly.
GO_PKGS = $(shell go list ./... | grep -v '/node_modules/')

.PHONY: tools
tools:
	GOBIN=$(BIN_DIR) go install github.com/sqlc-dev/sqlc/cmd/sqlc
	GOBIN=$(BIN_DIR) go install github.com/pressly/goose/v3/cmd/goose
	GOBIN=$(BIN_DIR) go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

# generate re-runs both codegen tools against their sources (db/queries +
# db/migrations for sqlc, openapi.yaml + bff-openapi.yaml for
# oapi-codegen) so the committed generated output (internal/db,
# internal/api/openapi.gen.go, internal/bffapi/bffapi.gen.go) can be
# regenerated reproducibly from the pinned tool versions in tools/tools.go
# rather than hand-edited.
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
# Two independent oapi-codegen invocations, one per spec file
# (milestone-3/task-2, `_contract/API.md` "Two specs, not one"): each run
# generates its own ServerInterface/Error/RegisterHandlers into its own
# package — internal/api for the Bearer-authenticated public surface,
# internal/bffapi for the session-authenticated BFF surface. Distinct
# package names (not just distinct output files) so the two independently
# generated sets of symbols of the same names (Error, ServerInterface,
# RegisterHandlers, ...) never collide at the Go package level.
.PHONY: generate
generate:
	@for f in internal/db/*.go; do \
		[ -f "$$f" ] || continue; \
		grep -q '^// Code generated' "$$f" && rm -f "$$f"; \
	done
	$(BIN_DIR)/sqlc generate
	$(BIN_DIR)/oapi-codegen -generate types,gin,spec -package api -o internal/api/openapi.gen.go openapi.yaml
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
	go build $(GO_PKGS)

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
