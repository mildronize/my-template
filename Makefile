# BIN_DIR is a repo-local install target for pinned dev tools (sqlc,
# goose, oapi-codegen), kept out of $(go env GOPATH)/bin so a fork's
# pinned versions never collide with another repo's on the same machine.
BIN_DIR := $(CURDIR)/bin

.PHONY: tools
tools:
	GOBIN=$(BIN_DIR) go install github.com/sqlc-dev/sqlc/cmd/sqlc
	GOBIN=$(BIN_DIR) go install github.com/pressly/goose/v3/cmd/goose
	GOBIN=$(BIN_DIR) go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

# generate re-runs both codegen tools against their sources (db/queries +
# db/migrations for sqlc, openapi.yaml for oapi-codegen) so the committed
# generated output (internal/db, internal/api/openapi.gen.go) can be
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
.PHONY: generate
generate:
	@for f in internal/db/*.go; do \
		[ -f "$$f" ] || continue; \
		grep -q '^// Code generated' "$$f" && rm -f "$$f"; \
	done
	$(BIN_DIR)/sqlc generate
	$(BIN_DIR)/oapi-codegen -generate types,gin,spec -package api -o internal/api/openapi.gen.go openapi.yaml

.PHONY: build
build:
	go build ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test ./...

.PHONY: fmt-check
fmt-check:
	gofmt -l .

.PHONY: run
run:
	go run ./cmd/server
