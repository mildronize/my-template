package bff

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mildronize/my-template/internal/identity"
)

// UserRole is User.Role's fixed two-value enum (bff-openapi.yaml's
// UserRole schema) — replaces internal/bffapi.UserRole (deleted this
// ticket).
type UserRole string

// User is GET /api/bff/users' row shape — the assignee picker's data
// source (มายด์'s ask: "the assignee form ... to be a drop[down] of the
// assignee, not freeform text, see in my-task"). Replaces
// internal/bffapi.User — same field names/JSON tags as before.
type User struct {
	Id     string   `json:"id"`
	Handle string   `json:"handle"`
	Role   UserRole `json:"role" enum:"owner,agent"`
	Active bool     `json:"active"`
}

// UserList is GET /api/bff/users' response envelope.
type UserList struct {
	Users []User `json:"users"`
}

// ListUsersOutput is ListUsers' huma.Register output.
type ListUsersOutput struct {
	Body UserList
}

func toBFFUser(u identity.User) User {
	return User{
		Id:     u.ID,
		Handle: u.Handle,
		Role:   UserRole(u.Role),
		Active: u.Active,
	}
}

// RegisterUsersRoutes registers GET /api/bff/users on api (ticket-7) —
// every active user, either role, ordered by handle
// (identity.Service.ListActiveUsers' own doc comment). Session-gated
// only, no further scoping: unlike /keys, there is no "which users belong
// to this caller" question to ask — every active user is a real candidate
// assignee for every todo (todos are shared, GOAL.md's Ownership model
// decision). api must already have humamw.RequireSession attached
// (cmd/server/main.go).
func RegisterUsersRoutes(api huma.API, svc *identity.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "listUsers",
		Method:      http.MethodGet,
		Path:        "/api/bff/users",
		Summary:     "List every active user, either role (the assignee picker's data source).",
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, _ *struct{}) (*ListUsersOutput, error) {
		if _, err := actorFromContext(ctx); err != nil {
			return nil, err
		}

		users, err := svc.ListActiveUsers(ctx)
		if err != nil {
			return nil, internalServerError()
		}

		resp := UserList{Users: make([]User, 0, len(users))}
		for _, u := range users {
			resp.Users = append(resp.Users, toBFFUser(u))
		}
		return &ListUsersOutput{Body: resp}, nil
	})
}
