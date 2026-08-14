package bff

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/bffapi"
)

// decodeUserList decodes GET /api/bff/users' response body.
func decodeUserList(t *testing.T, body []byte) bffapi.UserList {
	t.Helper()
	var got bffapi.UserList
	require.NoError(t, json.Unmarshal(body, &got))
	return got
}

// TestBFFHandler_ListUsers_BothRoles_ExcludesInactive is GET
// /api/bff/users' own test — the assignee picker's data source (มายด์'s
// ask). Seeds the agent identity through identity.Service.
// IssueAPIKeyForHandle (the real cmd/issue-key path, keys_handler_test.go's
// own standing rule for this package: a raw repo.CreateUser(..., "agent",
// ...) insert would test a state production reaches by a different,
// unverified path). Asserts both halves independently: the active agent
// IS present with the right id/handle/role, and a separately-seeded
// inactive user is NOT — a check that only asserted one half could pass
// by accident (e.g. returning every user regardless of active).
func TestBFFHandler_ListUsers_BothRoles_ExcludesInactive(t *testing.T) {
	router, ownerSession, owner, conn, identitySvc, _ := newBFFRouterForOwnerSharedDB(t)

	issued, err := identitySvc.IssueAPIKeyForHandle(context.Background(), "an-agent")
	require.NoError(t, err)

	inactive := seedUser(t, conn, "agent", "inactive-sub-"+t.Name(), false)

	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/users", ownerSession, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	list := decodeUserList(t, rec.Body.Bytes())

	byID := map[string]bffapi.User{}
	for _, u := range list.Users {
		byID[u.Id] = u
	}

	ownerRow, ok := byID[owner.ID]
	require.True(t, ok, "the session owner itself must be a listed, assignable user")
	assert.Equal(t, owner.Handle, ownerRow.Handle)
	assert.Equal(t, bffapi.UserRole("owner"), ownerRow.Role)
	assert.True(t, ownerRow.Active)

	agentRow, ok := byID[issued.User.ID]
	require.True(t, ok, "the real, issued agent must be a listed, assignable user")
	assert.Equal(t, "an-agent", agentRow.Handle)
	assert.Equal(t, bffapi.UserRole("agent"), agentRow.Role)

	_, stillListed := byID[inactive.ID]
	assert.False(t, stillListed, "an inactive user must not be offered as an assignee")
}

// TestBFFHandler_ListUsers_Unauthenticated_Returns401 — owner-session
// only, same tier as every other endpoint on this surface (there is no
// equivalent on the public API at all: an agent has no business
// enumerating users).
func TestBFFHandler_ListUsers_Unauthenticated_Returns401(t *testing.T) {
	router, _, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/users", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
