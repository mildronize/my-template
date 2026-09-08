// Package humamw is the shared, router-agnostic huma plumbing both
// surface migrations (tickets 6, 7) depend on — the I1 X-Actor header
// guard, Bearer/session actor resolution, and the error-envelope override
// that makes every huma-produced error body match
// _rules/_contract/API.md's `{error:{code,message,hint}}` shape instead
// of huma's own default RFC 9457 `application/problem+json` model
// (docs/docs/features/response-errors.md's "Custom Errors" section).
//
// Deliberately its own package, not folded into
// internal/transport/publicapi or internal/transport/bff: the X-Actor
// guard and the error-envelope override are shared by *both* surfaces —
// registered on both apiV1's and apiBFF's huma.Config (their respective
// huma.API instances, cmd/server/main.go) — whereas publicapi/bff each
// still own their own surface-specific gin-based middleware for as long
// as oapi-codegen keeps serving real traffic (tickets 6/7 retire that
// half once routes actually move). Named humamw rather than huma to avoid
// shadowing the github.com/danielgtaylor/huma/v2 import's own default
// package identifier in any file that needs both.
package humamw

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// ErrorBody is the `error` object _rules/_contract/API.md's error shape
// names — the exact three fields every hand-rolled 4xx response in this
// repo already writes (internal/transport/publicapi.ErrorEnvelope).
//
// Deliberately a new type here, not a reuse of publicapi.ErrorEnvelope —
// even though the JSON shape is identical and bff already reuses that
// exact type for its own hand-written errors. Reusing it would mean
// either giving publicapi.ErrorEnvelope huma.StatusError's methods
// (Error()/GetStatus()) directly, which pulls a huma import into
// internal/transport/publicapi before this ticket is supposed to touch
// that package's behavior at all (ticket-5's own scope: "don't touch
// internal/api, internal/bffapi, or the existing route registration...
// nothing user-facing changes in this ticket"), or wrapping it in a
// second type anyway, which is the same duplication with an extra layer.
// contract.md's own "File/package layout" section already marks
// publicapi/middleware.go for a real rewrite in tickets 6/7 once routes
// actually move — reusing its type today would tie this ticket's rollback
// safety to a package this ticket isn't supposed to modify.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// ErrorEnvelope is this package's huma.StatusError implementation —
// installed once, process-wide, as the replacement for huma.NewError (see
// InstallErrorEnvelope below) so every error huma itself produces
// (request-validation failures, huma.Error404NotFound, etc.) serializes
// to `{"error":{"code":...,"message":...,"hint":...}}` instead of huma's
// own default RFC 9457 model. The field is named Err, not Error, because
// a struct cannot declare both a field and a method named Error — see
// GetStatus/Error below for the huma.StatusError contract this type
// satisfies (GetStatus() int + the standard `error` interface).
type ErrorEnvelope struct {
	status int
	Err    ErrorBody `json:"error"`
}

var _ huma.StatusError = (*ErrorEnvelope)(nil)

// Error satisfies the standard `error` interface (part of
// huma.StatusError) — the envelope's own message, matching
// ErrorDetail.Error()'s "just the message" behavior when there's nothing
// else to add.
func (e *ErrorEnvelope) Error() string {
	return e.Err.Message
}

// GetStatus satisfies huma.StatusError — the HTTP status this error
// should be written with.
func (e *ErrorEnvelope) GetStatus() int {
	return e.status
}

// codeForStatus maps an HTTP status to this repo's own error `code`
// string, per _rules/_contract/API.md's status/code table as corrected by
// this story's own contract.md "Error envelope" section. After this
// migration, 400 is used by exactly one path (the X-Actor header guard,
// below) — every generic shape-validation failure huma itself produces
// natively answers 422, not 400 (contract.md's "Behavior note": a
// deliberate, flagged status-code delta from today's gin-middleware
// behavior, not silently absorbed). A status this table doesn't name
// (huma's own request-parsing failures, e.g. 413/408, or a 500 an
// unrecognized error produces) falls back to a snake_cased
// http.StatusText — not a contract-specified mapping (the contract's own
// table only names the five codes this repo's API surface actually
// returns), but a safe, readable default rather than an empty string.
func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "actor_field_present"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "invalid_transition"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusUnprocessableEntity:
		return "validation_error"
	default:
		return strings.ToLower(strings.ReplaceAll(http.StatusText(status), " ", "_"))
	}
}

// InstallErrorEnvelope overrides the package-level huma.NewError (once,
// process-wide — docs/docs/features/response-errors.md:132's documented
// extension point) so every error huma writes — from
// huma.Error404NotFound and friends, to a request-validation failure
// huma's own parsing/binding produces internally — builds an
// *ErrorEnvelope instead of huma's default *huma.ErrorModel. Call this
// once at process startup, before the server starts accepting requests
// (cmd/server/main.go) — huma.NewError is a mutable package-level var,
// not per-instance state, so installing it twice is harmless (the second
// call just replaces the first with an identical function) but installing
// it from more than one goroutine concurrently would race.
//
// hint is populated from the first error detail passed to NewError, if
// any: an huma.ErrorDetailer's own Location (matching
// _rules/_contract/API.md's validation_error example, `"hint": "title"`)
// when the detail implements that interface, otherwise the error's own
// message. Most call sites in this ticket (the X-Actor guard) pass no
// errs at all, so hint is "" for that path — matching today's
// NewErrorEnvelope("actor_field_present", ..., "") call in
// internal/transport/publicapi/middleware.go.
func InstallErrorEnvelope() {
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		hint := ""
		if len(errs) > 0 && errs[0] != nil {
			if d, ok := errs[0].(huma.ErrorDetailer); ok {
				hint = d.ErrorDetail().Location
			} else {
				hint = errs[0].Error()
			}
		}

		env := &ErrorEnvelope{status: status}
		env.Err.Code = codeForStatus(status)
		env.Err.Message = msg
		env.Err.Hint = hint
		return env
	}
}

// writeError writes status/message as this package's ErrorEnvelope JSON
// shape directly onto ctx, via huma.NewError (so it reflects whatever
// InstallErrorEnvelope installed) — used by the router-agnostic
// middlewares below (RejectActorHeader, RequireActor, RequireSession),
// which run as part of huma.Config.Middlewares/OpenAPI.Middlewares —
// registered on the Config *before* the huma.API instance that would
// normally be huma.WriteErr's first argument exists yet
// (docs/docs/features/middleware.md's "Errors" section shows
// huma.WriteErr(api, ctx, ...), which needs that instance). Writing the
// bytes directly via ctx.SetStatus/ctx.SetHeader/ctx.BodyWriter sidesteps
// that ordering problem and always emits application/json, which matches
// this repo's existing behavior (gin's AbortWithStatusJSON never
// content-negotiates either).
func writeError(ctx huma.Context, status int, message string) {
	err := huma.NewError(status, message)
	body, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		// Unreachable in practice — ErrorEnvelope has no field that can
		// fail to marshal — but fail safe rather than panic on a write
		// path.
		body = []byte(`{"error":{"code":"internal_error","message":"failed to encode error response"}}`)
	}
	ctx.SetStatus(status)
	ctx.SetHeader("Content-Type", "application/json")
	_, _ = ctx.BodyWriter().Write(body)
}
