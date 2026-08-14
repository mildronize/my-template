// Package internal — Done-when 6's own check: the `my-template-api`
// skill doc (`.claude/skills/my-template-api/`) is the only thing an
// agent reading this repo actually consults to call the API — a doc that
// still tells an agent to call `DELETE /todos/:id` after tasks 3/4
// removed it from the code makes the removal undone for the only readers
// who act on it. This file is that doc's own coverage check, the same
// floor-first shape as every other absence-assertion built this
// milestone (I15's function-count floor, I20's file-count floor,
// dbquery's grant-must-be-exercised check): an absence check with no
// floor can pass by finding nothing to check at all — a moved skill
// directory, a renamed file, a doc that fails to load — and "DELETE not
// found" would be trivially true for a reason that has nothing to do
// with whether the removal actually stuck.
package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skillDocFiles is the exact set of files this check reads — not a glob,
// so a file silently added to (or removed from) the skill directory
// without updating this list is visible in a diff, not swallowed by a
// pattern match.
var skillDocFiles = []string{
	filepath.Join("SKILL.md"),
	filepath.Join("references", "endpoints.md"),
	filepath.Join("references", "errors.md"),
}

// skillDocDir resolves the agent-facing API skill's own directory,
// relative to the repo root, by scanning .claude/skills/ for exactly one
// "*-api" directory rather than hardcoding "my-template-api" — a fork
// renames that directory per GETTING-STARTED.md Step 3, and a hardcoded
// name here broke this file's own coverage check the moment a fork did
// what the doc told it to (found by the first real fork; domainModuleNames
// in architecture_test.go is the same discover-don't-hardcode shape, one
// level down). Failing loudly on zero or on more than one match, the same
// as require.NoErrorf below does for a missing file — an ambiguous match
// is not the same claim as a resolved one.
func skillDocDir(t *testing.T, root string) string {
	t.Helper()
	skillsDir := filepath.Join(root, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	require.NoErrorf(t, err, "reading %s", skillsDir)

	var matches []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), "-api") {
			matches = append(matches, entry.Name())
		}
	}
	require.Lenf(t, matches, 1, "expected exactly one *-api directory under %s, found %v", skillsDir, matches)
	return filepath.Join(skillsDir, matches[0])
}

// readSkillDocs reads every skillDocFiles entry and returns their
// combined content plus each file's own individual content, failing
// loudly (not skipping) if the directory or any named file is missing —
// the same "a missing directory is not the same claim as scanned and
// found nothing" distinction I20's own static check draws.
func readSkillDocs(t *testing.T, root string) (combined string, perFile map[string]string) {
	t.Helper()
	dir := skillDocDir(t, root)

	info, err := os.Stat(dir)
	require.NoErrorf(t, err, "%s must exist for this check to mean anything", dir)
	require.Truef(t, info.IsDir(), "%s exists but is not a directory", dir)

	perFile = make(map[string]string, len(skillDocFiles))
	for _, rel := range skillDocFiles {
		path := filepath.Join(dir, rel)
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "reading %s", path)
		content := string(data)
		require.GreaterOrEqualf(t, len(content), 200,
			"%s is suspiciously short (%d bytes) — a doc this thin was probably not actually "+
				"loaded/found, not genuinely reviewed and trimmed; the absence check below would pass "+
				"trivially against near-empty content", path, len(content))
		perFile[rel] = content
		combined += content
	}
	return combined, perFile
}

// TestDoneWhen6_SkillDocDeleteTodosRemoved is Done-when 6's own check:
// the skill doc no longer tells an agent to call the removed
// DELETE /api/v1/todos/:id.
//
// Deliberately NOT a bare substring search for "DELETE" — that would
// false-positive on DELETE /api/v1/keys/:id, a real, still-current
// endpoint this same doc correctly documents, and on this very file's
// own prose explaining the removal ("there is no DELETE .../todos/:id
// any more"). Instead it looks for the two literal SHAPES a live
// endpoint entry actually takes in this doc — SKILL.md's table row and
// endpoints.md's section header — which only reappear if someone
// re-documents the endpoint as available, not if someone merely
// mentions its absence in a sentence.
func TestDoneWhen6_SkillDocDeleteTodosRemoved(t *testing.T) {
	root := repoRoot(t)
	combined, perFile := readSkillDocs(t, root)
	_ = perFile

	assert.NotContainsf(t, combined, "| `DELETE` | `/api/v1/todos/:id` |",
		"SKILL.md's endpoints table must not list DELETE /api/v1/todos/:id as a live row — "+
			"that endpoint was removed in milestone-4 (Done-when 6)")
	assert.NotContainsf(t, combined, "## `DELETE /todos/:id`",
		"references/endpoints.md must not carry a DELETE /todos/:id section header — "+
			"that endpoint was removed in milestone-4 (Done-when 6)")

	// The legitimate DELETE /keys/:id entries must still be there — this
	// test proves the removed endpoint is gone, not that DELETE itself
	// was purged from the doc wholesale (which would be a different,
	// wrong fix: it's a real, current, still-documented endpoint).
	assert.Containsf(t, combined, "`/api/v1/keys/:id`",
		"the doc's own real DELETE /api/v1/keys/:id entry must still be documented — "+
			"this check must not have been satisfied by deleting DELETE mentions wholesale")
}

// TestDoneWhen6_SkillDocDocumentsTheNewEventEndpoints is the positive
// half Clara asked for explicitly: an absence check alone cannot see "a
// doc that removed DELETE and documented nothing new" — a doc could pass
// the test above by simply deleting the DELETE section and adding
// nothing in its place, which would be half a fix, not a fix.
func TestDoneWhen6_SkillDocDocumentsTheNewEventEndpoints(t *testing.T) {
	root := repoRoot(t)
	combined, _ := readSkillDocs(t, root)

	for _, must := range []string{
		"/todos/:id/events", // the new endpoints exist in the doc at all
		"status_changed",    // at least one event type is named
		"commented",
		"assigned",
		"field_changed",
		"clientRequestId", // I19's idempotency key is documented as required
	} {
		assert.Containsf(t, combined, must,
			"the skill doc must document %q — a doc that removed DELETE without documenting "+
				"the new event endpoints is only half fixed", must)
	}
}

// TestDoneWhen6_SkillDocFieldListUpdated is the other half of "documented
// something new": the field list (status/assignee/priority/dueDate)
// replacing the old done/owner-scoped shape.
func TestDoneWhen6_SkillDocFieldListUpdated(t *testing.T) {
	root := repoRoot(t)
	combined, _ := readSkillDocs(t, root)

	for _, must := range []string{"assigneeId", "priority", "dueDate", "createdBy"} {
		assert.Containsf(t, combined, must,
			"the skill doc must document the %q field — the milestone-4 field list "+
				"(status/assignee/priority/dueDate/createdBy) replacing done/ownerId", must)
	}
}
