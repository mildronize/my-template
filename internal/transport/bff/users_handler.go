package bff

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/identity"
)

// UsersServer adapts identity.Service to internal/bffapi's generated
// ServerInterface's users-shaped subset (ListUsers) — GET /api/bff/users,
// the assignee picker's own data source (มายด์'s ask: "the assignee form
// ... to be a drop[down] of the assignee, not freeform text, see in
// my-task"). Mirrors my-task's own user.ts router (list only, "humans
// and agents in one list") and this package's own KeysServer shape
// (session-gated, no create/update/delete — see that file's own doc
// comment for the same reasoning applied to keys).
type UsersServer struct {
	Service *identity.Service
}

// NewUsersServer builds a UsersServer on top of svc.
func NewUsersServer(svc *identity.Service) *UsersServer {
	return &UsersServer{Service: svc}
}

func toBFFUser(u identity.User) bffapi.User {
	return bffapi.User{
		Id:     u.ID,
		Handle: u.Handle,
		Role:   bffapi.UserRole(u.Role),
		Active: u.Active,
	}
}

// ListUsers implements bffapi.ServerInterface — GET /api/bff/users. Every
// active user, either role, ordered by handle (identity.Service.
// ListActiveUsers' own doc comment) — session-gated only, no further
// scoping: unlike /keys, there is no "which users belong to this caller"
// question to ask, every active user is a real candidate assignee for
// every todo (todos are shared, GOAL.md's Ownership model decision).
func (s *UsersServer) ListUsers(c *gin.Context) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	users, err := s.Service.ListActiveUsers(c.Request.Context())
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	resp := bffapi.UserList{Users: make([]bffapi.User, 0, len(users))}
	for _, u := range users {
		resp.Users = append(resp.Users, toBFFUser(u))
	}
	c.JSON(http.StatusOK, resp)
}
