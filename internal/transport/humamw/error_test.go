package humamw

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain installs the process-wide huma.NewError override once for
// every test in this package — every test below (this file's and
// actor_test.go's) relies on huma writing this package's ErrorEnvelope
// shape, not huma's own default RFC 9457 model, the same way
// cmd/server/main.go is expected to call InstallErrorEnvelope() once at
// startup before real requests arrive.
func TestMain(m *testing.M) {
	InstallErrorEnvelope()
	os.Exit(m.Run())
}

// newTestConfig builds a minimal huma.Config good enough for this
// package's own isolated tests — no OpenAPIPath/DocsPath (this package
// never registers real routes; cmd/server/main.go's own configs disable
// both for the same "don't add an observable new route" reason, see that
// file's own comment).
func newTestConfig(title string) huma.Config {
	return huma.Config{
		OpenAPI: &huma.OpenAPI{
			Info: &huma.Info{Title: title, Version: "1.0.0"},
		},
		Formats:       map[string]huma.Format{"application/json": huma.DefaultJSONFormat},
		DefaultFormat: "application/json",
	}
}

// errorBody unmarshals a response recorder's body into the
// {"error":{...}} shape and returns the inner object, failing the test if
// the top-level "error" key is missing or not an object — every test
// asserting envelope shape below shares this helper so a shape regression
// fails loudly rather than via a silent nil-map lookup.
func errorBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed), "response body should be valid JSON: %s", body)
	errObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok, `response body should have an "error" object: %s`, body)
	return errObj
}

// TestErrorEnvelope_JSONShape_HumaNativeError proves the InstallErrorEnvelope
// override actually takes effect for error paths this package's own
// middleware never touches — huma.Error404NotFound is exactly the kind of
// error a real operation handler will return once tickets 6/7 move real
// routes onto huma (contract.md's error-envelope section: "instead of
// huma's default RFC9457 application/problem+json body").
func TestErrorEnvelope_JSONShape_HumaNativeError(t *testing.T) {
	_, api := humatest.New(t, newTestConfig("error envelope test — native"))
	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{id}",
	}, func(_ context.Context, _ *struct {
		ID string `path:"id"`
	}) (*struct{}, error) {
		return nil, huma.Error404NotFound("thing not found")
	})

	resp := api.Get("/things/abc")

	assert.Equal(t, http.StatusNotFound, resp.Code)
	assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))
	errObj := errorBody(t, resp.Body.Bytes())
	assert.Equal(t, "not_found", errObj["code"])
	assert.Equal(t, "thing not found", errObj["message"])
}

// TestErrorEnvelope_JSONShape_ValidationFailure covers contract.md's
// "Behavior note": a generic shape-validation failure (here, a body field
// that fails minLength) now answers huma's native 422, mapped to this
// repo's own validation_error code by codeForStatus — a deliberate,
// flagged status-code delta from today's gin-middleware 400, not silently
// absorbed.
func TestErrorEnvelope_JSONShape_ValidationFailure(t *testing.T) {
	_, api := humatest.New(t, newTestConfig("error envelope test — validation"))
	huma.Register(api, huma.Operation{
		OperationID: "create-thing",
		Method:      http.MethodPost,
		Path:        "/things",
	}, func(_ context.Context, _ *struct {
		Body struct {
			Title string `json:"title" minLength:"1"`
		}
	}) (*struct{}, error) {
		return nil, nil
	})

	resp := api.Post("/things", map[string]any{"title": ""})

	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
	errObj := errorBody(t, resp.Body.Bytes())
	assert.Equal(t, "validation_error", errObj["code"])
}
