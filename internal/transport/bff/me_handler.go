package bff

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Me is GET /api/bff/me's response shape (_contract/API.md,
// bff-openapi.yaml's Me schema) — deliberately the exact same shape as
// internal/transport/publicapi's own GET /api/v1/me response
// (publicapi.Me): one response shape for "who am I," regardless of which
// surface asked. Not literally the same Go type (each surface keeps its
// own independently-authored types per bff-openapi.yaml's own
// info.description "Two specs, not one" decision) — the contract that
// matters is the wire shape, which this matches field for field. Replaces
// the old bffMeResponse (me_handler.go, pre-ticket-7) and internal/bffapi's
// generated Me (deleted this ticket) — same field names/JSON tags as
// before.
type Me struct {
	Handle string `json:"handle"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

// GetMeOutput is GetMe's huma.Register output.
type GetMeOutput struct {
	Body Me
}

// RegisterMeRoutes registers GET /api/bff/me on api (ticket-7) — the
// session-check endpoint backing the SPA's AuthGate-equivalent hook.
// Mirrors internal/transport/publicapi.RegisterMeRoutes exactly, session
// resolution instead of Bearer resolution. api must already have
// humamw.RequireSession attached (cmd/server/main.go) — this operation
// only ever reads the actor that middleware resolved (I4), it never
// resolves identity itself.
func RegisterMeRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/bff/me",
		Summary:     "Session-check endpoint backing the SPA's AuthGate-equivalent hook.",
		Description: "Same response shape as the public API's `GET /api/v1/me`, deliberately — one response shape for \"who am I,\" regardless of which surface asked.",
		Errors:      []int{http.StatusUnauthorized},
	}, handleGetMe)
}

// handleGetMe reads the actor humamw.RequireSession (cmd/server/main.go)
// already resolved onto ctx — it never queries users itself (I4).
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
