package publicapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Me is GET /api/v1/me's response shape (_contract/API.md,
// openapi.yaml's Me schema). Replaces the old bespoke meResponse — same
// field names/JSON tags, just exported now that it's a real huma output
// body type (ticket-6) rather than a type only ever built and marshaled
// inside this file.
type Me struct {
	Handle string `json:"handle"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

// GetMeOutput is GetMe's huma.Register output.
type GetMeOutput struct {
	Body Me
}

// RegisterMeRoutes registers GET /api/v1/me on api (ticket-6). GetMe
// predates openapi.yaml (task-2 hand-wired it directly on the gin group
// before that file existed) — task-3 brought it onto the same
// generated-interface, validated path as every other endpoint; this
// ticket brings it onto huma the same way every other operation in this
// package moves. api must already have humamw.RequireActor attached
// (cmd/server/main.go) — this operation only ever reads the actor that
// middleware resolved (I4), it never resolves identity itself.
func RegisterMeRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/v1/me",
		Summary:     "Return the authenticated actor's own identity.",
		Description: "`me` is the only way an actor refers to itself — there is no endpoint that takes another actor's id.",
		Errors:      []int{http.StatusUnauthorized},
	}, handleGetMe)
}

// handleGetMe reads the actor humamw.RequireActor (cmd/server/main.go)
// already resolved onto ctx — it never queries users/api_keys itself
// (I4).
func handleGetMe(ctx context.Context, _ *struct{}) (*GetMeOutput, error) {
	user, err := actorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return &GetMeOutput{Body: Me{
		Handle: user.Handle,
		Role:   user.Role,
		Active: user.Active,
	}}, nil
}
