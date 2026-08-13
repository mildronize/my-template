// Package bffapi holds the bff-openapi.yaml-generated server interface
// (bffapi.gen.go) plus the small amount of hand-written glue needed to
// turn it into a working gin request validator (this file) — the BFF's
// own mirror of internal/api's validator.go, one per spec file per
// _contract/API.md's "Two specs, not one" decision. It is not one of
// ARCHITECTURE.md's "todo"/"identity" domain modules; internal/transport/
// bff composes its own ServerInterface implementation on top of this
// package the same way internal/transport/publicapi composes one on top
// of internal/api.
package bffapi

import (
	"net/http"
	"regexp"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
)

// paramNameRe mirrors internal/api/validator.go's own — see that file's
// doc comment for the exact kin-openapi message shape this pulls a field
// name out of.
var paramNameRe = regexp.MustCompile(`^parameter "([^"]+)" in`)

// RequestValidator builds the gin middleware that rejects any request
// violating bff-openapi.yaml's shape (missing required field, wrong type)
// before it ever reaches handler code — the BFF-surface equivalent of
// internal/api.RequestValidator. Authentication is deliberately NOT a
// declared OpenAPI `security` requirement here either (bff-openapi.yaml's
// own info.description), so this validator's own AuthenticationFunc is a
// no-op; session resolution stays internal/transport/bff's job
// (bff.RequireJSONSession), run as separate gin middleware around this
// one, mounted before it so an unauthenticated request never spends a
// shape-validation pass first.
func RequestValidator() (gin.HandlerFunc, error) {
	swagger, err := GetSpec()
	if err != nil {
		return nil, err
	}

	return ginmiddleware.OapiRequestValidatorWithOptions(swagger, &ginmiddleware.Options{
		// bff-openapi.yaml's `servers: [{url: /api/bff}]` entry is a
		// path-only prefix (no host), same reasoning as internal/api's own
		// validator.
		SilenceServersWarning: true,
		ErrorHandler:          writeValidationError,
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
	}), nil
}

// writeValidationError renders a validation failure in the same
// {error:{code,message,hint}} shape every response on this surface uses
// (_contract/API.md's Error shape, reused from the public API) — mirrors
// internal/api/validator.go's own writeValidationError, using this
// package's own generated Error type rather than gin-middleware's default
// {"msg": "..."} body.
func writeValidationError(c *gin.Context, message string, statusCode int) {
	code := "validation_error"
	if statusCode == http.StatusNotFound {
		code = "not_found"
	}

	var hint *string
	if m := paramNameRe.FindStringSubmatch(message); m != nil {
		hint = &m[1]
	}

	body := Error{}
	body.Error.Code = code
	body.Error.Message = message
	body.Error.Hint = hint
	c.AbortWithStatusJSON(statusCode, body)
}
