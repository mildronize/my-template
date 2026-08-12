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
.PHONY: generate
generate:
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
