// Package api holds the openapi.yaml-generated server interface
// (openapi.gen.go) plus the small amount of hand-written glue needed to
// turn it into a working gin request validator (this file). It is not one
// of ARCHITECTURE.md's "todo"/"identity" domain modules — cmd/server
// composes both of those into this package's ServerInterface.
package api

import (
	"net/http"
	"regexp"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
)

// paramNameRe pulls the quoted parameter/field name out of kin-openapi's
// "parameter %q in %s has an error: ..." message shape (openapi3filter's
// RequestError.Error(), see its errors.go) — best-effort, matching
// _contract/API.md's "hint names the field" convention for the cases
// where there is a single field to name. When the message doesn't match
// (e.g. malformed JSON with no single field to blame), Hint is left
// empty rather than guessed.
var paramNameRe = regexp.MustCompile(`^parameter "([^"]+)" in`)

// RequestValidator builds the gin middleware that rejects any request
// violating openapi.yaml's shape (missing required field, wrong type)
// before it ever reaches handler code (_contract/API.md, GOAL.md
// Done-when 7). Authentication is deliberately NOT a declared OpenAPI
// `security` requirement — see openapi.yaml's info.description — so this
// validator's own AuthenticationFunc is a no-op; credential resolution
// stays entirely internal/identity's job (I1, I2, I5), run as separate
// gin middleware around this one.
func RequestValidator() (gin.HandlerFunc, error) {
	swagger, err := GetSpec()
	if err != nil {
		return nil, err
	}

	return ginmiddleware.OapiRequestValidatorWithOptions(swagger, &ginmiddleware.Options{
		// openapi.yaml's `servers: [{url: /api/v1}]` entry is a path-only
		// prefix (no host) so the validator's router matches against the
		// real mounted path (/api/v1/...); it carries none of the
		// Host-header ambiguity gin-middleware's warning is about.
		SilenceServersWarning: true,
		ErrorHandler:          writeValidationError,
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
	}), nil
}

// writeValidationError renders a validation failure in the same error
// envelope shape as every other error response in this service
// (_contract/API.md's Error shape), rather than gin-middleware's default
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
