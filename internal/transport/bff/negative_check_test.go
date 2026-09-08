package bff

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bffOperationIDs collects every operationId registered on api's own
// OpenAPI() document — story-1/ticket-7's structural-check source of
// truth, replacing internal/bffapi.ServerInterface's now-deleted
// generated interface (that reflection-based check below used to run
// against). huma has no built-in "list every operation" iterator, so this
// walks api.OpenAPI().Paths' per-method Operation pointers directly —
// mirrors how huma.OpenAPI.Paths/PathItem are actually shaped
// (openapi.go's PathItem: one *Operation field per HTTP method).
func bffOperationIDs(t *testing.T, api huma.API) []string {
	t.Helper()
	var ids []string
	for _, item := range api.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch, item.Trace} {
			if op != nil {
				ids = append(ids, op.OperationID)
			}
		}
	}
	return ids
}

// TestNegativeCheck_NoKeyIssuanceOrRotateEndpointOnBFFSurface is
// milestone-3/_goal/GOAL.md Done-when 5's explicit negative check: this
// surface must never grow a POST /api/bff/keys or any rotate endpoint,
// even though the settings screen it backs visually invites one
// (_contract/API.md: "A settings screen that could issue or rotate would
// need a UI element inviting exactly the action this boundary exists to
// prevent"). Two independent checks, deliberately redundant with each
// other:
//
//  1. A structural check against every operationId huma actually
//     registers on apiBFFH's own OpenAPI() document (bffOperationIDs,
//     above) — story-1/ticket-7's replacement for the pre-ticket-7
//     reflection check against internal/bffapi's oapi-codegen-generated
//     ServerInterface (deleted this ticket, along with the rest of that
//     package — bff-openapi.yaml is now huma-derived output, not
//     hand-authored input, so there is no generated Go interface left to
//     reflect on). Same intent, adapted mechanism per contract.md's
//     Testing Decisions (harness may change to track the transport
//     framework, the check's own assertions do not): if a future edit to
//     internal/transport/bff/keys_handler.go's RegisterKeysRoutes ever
//     adds an issuance/rotate huma.Register call, this check fails by
//     naming exactly which operationId — long before that operation
//     could reach a real request.
//  2. A live HTTP check that POST /api/bff/keys (and a couple of
//     plausible rotate-endpoint shapes) do not answer with any 2xx
//     status on the actual wired router — catching the case where a
//     future change bypasses huma.Register entirely and hand-registers a
//     route directly on the gin engine. Byte-for-byte unchanged from
//     before ticket-7 — this half was always a live-HTTP assertion, not
//     coupled to internal/bffapi at all.
func TestNegativeCheck_NoKeyIssuanceOrRotateEndpointOnBFFSurface(t *testing.T) {
	t.Run("apiBFFH has no issuance or rotate operation", func(t *testing.T) {
		router := gin.New()
		signer := NewSigner([]byte("test-negative-check-secret-not-for-prod"))
		api := newHumaBFFAPI(router, signer, nil)
		RegisterMeRoutes(api)
		RegisterKeysRoutes(api, nil)
		RegisterUsersRoutes(api, nil)
		RegisterTodoRoutes(api, nil)

		operationIDs := bffOperationIDs(t, api)
		require.NotEmpty(t, operationIDs, "sanity: apiBFFH must have at least one operation registered")

		// Every operationId this surface is allowed to carry for keys is
		// exactly this pair — anything else with "key" in its name is an
		// addition this check must catch by name.
		allowedKeyOperations := map[string]bool{
			"listKeys":  true,
			"revokeKey": true,
		}
		for _, id := range operationIDs {
			lower := strings.ToLower(id)
			if strings.Contains(lower, "key") {
				assert.Truef(t, allowedKeyOperations[id],
					"apiBFFH has operationId %q — the only key-related operations this surface may ever have are listKeys and revokeKey (milestone-3/_goal/GOAL.md Done-when 5, _contract/API.md: no issuance, no rotate)", id)
			}
			assert.NotContainsf(t, lower, "rotate",
				"apiBFFH has operationId %q — no rotate endpoint may ever exist on this surface (Done-when 5)", id)
			assert.NotContainsf(t, lower, "issue",
				"apiBFFH has operationId %q — no key-issuance endpoint may ever exist on this surface (Done-when 5)", id)
			assert.NotEqualf(t, "createKey", id,
				"apiBFFH has operationId %q — POST /api/bff/keys must never exist (Done-when 5)", id)
		}
	})

	t.Run("the live router answers no 2xx for POST /api/bff/keys or any rotate-shaped path", func(t *testing.T) {
		router, sessionValue, _ := newBFFRouterForOwner(t)

		candidates := []struct {
			method string
			path   string
		}{
			{http.MethodPost, "/api/bff/keys"},
			{http.MethodPost, "/api/bff/keys/rotate"},
			{http.MethodPost, "/api/bff/keys/some-id/rotate"},
			{http.MethodPatch, "/api/bff/keys/some-id/rotate"},
			{http.MethodPut, "/api/bff/keys/some-id/rotate"},
		}
		for _, c := range candidates {
			rec := doBFFJSONRequest(t, router, c.method, c.path, sessionValue, map[string]string{"handle": "owner"})
			is2xx := rec.Code >= http.StatusOK && rec.Code < http.StatusMultipleChoices
			assert.Falsef(t, is2xx, "expected no 2xx for %s %s (Done-when 5) — got %d", c.method, c.path, rec.Code)
		}
	})
}
