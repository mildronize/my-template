//go:build tools

// Package tools exists only to pin the exact versions of code-generation
// CLIs this template depends on (sqlc, goose, oapi-codegen) into
// go.mod/go.sum, via `go mod tidy`. The build tag keeps these imports out
// of normal builds; nothing here is ever compiled into the service binary.
//
// Install the pinned versions into ./bin with `make tools` (see Makefile).
package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
	_ "github.com/pressly/goose/v3/cmd/goose"
	_ "github.com/sqlc-dev/sqlc/cmd/sqlc"
)
