package todo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// permission_test.go tests can() in complete isolation — no database, no
// Service, no Repo — exactly the point of keeping it its own small,
// separate function (task-2.md's explicit instruction) rather than
// inlining the check only inside Append.

func TestCan_OwnerUnconditional(t *testing.T) {
	owner := PolicyActor{Role: "owner"}

	assert.True(t, can(owner, EventTypeCommented, ""))
	assert.True(t, can(owner, EventTypeAssigned, ""))
	assert.True(t, can(owner, EventTypeFieldChanged, ""))
	assert.True(t, can(owner, EventTypeStatusChanged, StatusInProgress))
	assert.True(t, can(owner, EventTypeStatusChanged, StatusClosed), "the owner is the one actor who may move a todo to closed (I18)")
}

// TestI18_Can_AgentPaired is Done-when 5's own pairing, at the pure
// permission-function layer: the same agent role, evaluated against both
// a status:closed attempt (must be refused) and a non-closed action
// (must succeed) — proving the refusal is about closed specifically, not
// about the agent role in general. A permission layer that rejected
// everything for an agent would pass a reject-only assertion just as well
// as a correct one; this test cannot pass that way.
func TestI18_Can_AgentPaired(t *testing.T) {
	agent := PolicyActor{Role: "agent"}

	assert.False(t, can(agent, EventTypeStatusChanged, StatusClosed), "an agent must never be allowed to move a todo to closed")
	assert.True(t, can(agent, EventTypeStatusChanged, StatusInProgress), "an agent may change status to anything except closed")
	assert.True(t, can(agent, EventTypeCommented, ""), "an agent may comment")
	assert.True(t, can(agent, EventTypeAssigned, ""), "an agent may assign")
	assert.True(t, can(agent, EventTypeFieldChanged, ""), "an agent may change a field")
}

func TestCan_UnknownRoleFailsClosed(t *testing.T) {
	// A typo, or a future role this function doesn't know about — treated
	// as non-owner, the agent's (more restrictive) rule set, the
	// fail-closed direction (mirrors my-task's own PolicyActor comment).
	stranger := PolicyActor{Role: "sous-chef"}

	assert.False(t, can(stranger, EventTypeStatusChanged, StatusClosed))
	assert.True(t, can(stranger, EventTypeStatusChanged, StatusInProgress))
}

func TestCan_UnknownEventTypeFailsClosedForNonOwner(t *testing.T) {
	agent := PolicyActor{Role: "agent"}
	assert.False(t, can(agent, WriteEventType("something_new"), ""))
}
