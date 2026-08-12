# BIN_DIR is a repo-local install target for pinned dev tools (sqlc,
# goose, oapi-codegen), kept out of $(go env GOPATH)/bin so a fork's
# pinned versions never collide with another repo's on the same machine.
BIN_DIR := $(CURDIR)/bin

.PHONY: tools
tools:
	GOBIN=$(BIN_DIR) go install github.com/sqlc-dev/sqlc/cmd/sqlc
	GOBIN=$(BIN_DIR) go install github.com/pressly/goose/v3/cmd/goose
	GOBIN=$(BIN_DIR) go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

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
