package todo

// permission.go — I18's `can(actor, action)`, kept as its own small,
// separate, independently-testable unit rather than inlined only inside
// the write path (task-2.md's explicit instruction). Mirrors my-task's
// own can() (~/gits/my-task/src/server/lib/policy.ts): role-based, a
// small allow-table, not per-todo-identity-based — an owner never needs
// a per-row check because ownership no longer scopes anything on this
// domain (GOAL.md's Ownership model decision), and an agent's allowance
// depends only on its role and the action it's attempting, never on
// which todo.

// PolicyActor is this permission layer's own minimal shape — role only,
// deliberately not identity.User itself, so can() stays trivially
// unit-testable with zero imports beyond this package (mirrors my-task's
// PolicyActor, policy.ts:41-50, which is also role-only for the same
// reason). service.go's Append converts whatever actor shape its caller
// passes it down to this at the call boundary.
type PolicyActor struct {
	// Role is "owner" | "agent" (DATA_MODEL.md's users.role). Any other
	// value — a typo, or a future role this function doesn't know about —
	// is treated as non-owner and gets the agent's (more restrictive) rule
	// set, the fail-closed direction to err in (matches my-task's own
	// PolicyActor doc comment on this exact point).
	Role string
}

const roleOwner = "owner"

// can reports whether actor may write an event of type eventType. For
// EventStatusChanged specifically, toStatus is the status the write would
// move the todo to — every other event type ignores it.
//
// I18: an owner passes unconditionally. An agent may comment, assign, or
// change any field, and may change status to anything except
// StatusClosed — the one restriction this domain's fixed four-value enum
// exists to have somewhere to bind (GOAL.md's Permission model decision).
// EventCreated is not a case this function needs to handle: WriteEventType
// (service.go) has no value that maps to it, so Append can never call can()
// with it in the first place (I16).
func can(actor PolicyActor, eventType WriteEventType, toStatus Status) bool {
	if actor.Role == roleOwner {
		return true
	}

	switch eventType {
	case EventTypeStatusChanged:
		return toStatus != StatusClosed
	case EventTypeCommented, EventTypeAssigned, EventTypeFieldChanged:
		return true
	default:
		// Fail closed: an event type this function doesn't recognise is
		// refused for a non-owner, never silently allowed.
		return false
	}
}
