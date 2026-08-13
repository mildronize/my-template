package bff

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/bffapi"
)

// TestNegativeCheck_NoKeyIssuanceOrRotateEndpointOnBFFSurface is
// milestone-3/_goal/GOAL.md Done-when 5's explicit negative check: this
// surface must never grow a POST /api/bff/keys or any rotate endpoint,
// even though the settings screen it backs visually invites one
// (_contract/API.md: "A settings screen that could issue or rotate would
// need a UI element inviting exactly the action this boundary exists to
// prevent"). Two independent checks, deliberately redundant with each
// other:
//
//  1. A structural check against internal/bffapi's generated
//     ServerInterface — the contract bff-openapi.yaml itself produces.
//     If a future edit to bff-openapi.yaml ever adds an issuance/rotate
//     path, `make generate` regenerates this interface with a new method
//     name, and this check fails by naming exactly which one — long
//     before any handler is written to implement it.
//  2. A live HTTP check that POST /api/bff/keys (and a couple of
//     plausible rotate-endpoint shapes) do not answer with any 2xx
//     status on the actual wired router — catching the case where a
//     future change bypasses bff-openapi.yaml/oapi-codegen entirely and
//     hand-registers a route directly on the gin engine.
func TestNegativeCheck_NoKeyIssuanceOrRotateEndpointOnBFFSurface(t *testing.T) {
	t.Run("bffapi.ServerInterface has no issuance or rotate method", func(t *testing.T) {
		iface := reflect.TypeOf((*bffapi.ServerInterface)(nil)).Elem()
		require.Positive(t, iface.NumMethod(), "sanity: bffapi.ServerInterface must have at least one method")

		var methodNames []string
		for i := range iface.NumMethod() {
			methodNames = append(methodNames, iface.Method(i).Name)
		}

		// Every method this generated interface is allowed to carry for
		// keys is exactly this pair — anything else with "Key" in its
		// name is an addition this check must catch by name.
		allowedKeyMethods := map[string]bool{
			"ListKeys":  true,
			"RevokeKey": true,
		}
		for _, name := range methodNames {
			lower := strings.ToLower(name)
			if strings.Contains(lower, "key") {
				assert.Truef(t, allowedKeyMethods[name],
					"bffapi.ServerInterface has method %q — the only key-related methods this surface may ever have are ListKeys and RevokeKey (milestone-3/_goal/GOAL.md Done-when 5, _contract/API.md: no issuance, no rotate)", name)
			}
			assert.NotContainsf(t, lower, "rotate",
				"bffapi.ServerInterface has method %q — no rotate endpoint may ever exist on this surface (Done-when 5)", name)
			assert.NotContainsf(t, lower, "issue",
				"bffapi.ServerInterface has method %q — no key-issuance endpoint may ever exist on this surface (Done-when 5)", name)
			assert.Falsef(t, name == "CreateKey",
				"bffapi.ServerInterface has method %q — POST /api/bff/keys must never exist (Done-when 5)", name)
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
