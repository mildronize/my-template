//go:build tools

// Package tools exists only to pin the exact versions of code-generation
// CLIs this template depends on (sqlc, goose) into go.mod/go.sum, via
// `go mod tidy`. The build tag keeps these imports out of normal builds;
// nothing here is ever compiled into the service binary.
//
// oapi-codegen used to be pinned here too, for generating Go server
// interfaces from openapi.yaml/bff-openapi.yaml — story-1's huma
// migration removed its last caller (bff-openapi.yaml's own generation,
// mirroring openapi.yaml's own earlier removal), and both surfaces now
// generate their spec from code (`go run ./cmd/server openapi`) rather
// than generating code from spec, so there is nothing left for
// oapi-codegen to do in this repo.
//
// Install the pinned versions into ./bin with `make tools` (see Makefile).
package tools

import (
	_ "github.com/pressly/goose/v3/cmd/goose"
	_ "github.com/sqlc-dev/sqlc/cmd/sqlc"
)
