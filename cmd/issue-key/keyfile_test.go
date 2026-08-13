package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireBash skips a test outright on a platform with no /bin/bash — the
// resolver script itself (and therefore these tests) is bash-specific, the
// same as my-task's own.
func requireBash(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("resolver script is bash-specific")
	}
}

// resolverEnv builds a clean environment for exec.Command'ing the resolver
// script: PATH (needed to locate bash via the script's own #!/usr/bin/env
// bash shebang), HOME, and exactly the MY_TEMPLATE_CREW/TYP_CREW_NAME
// values a test wants (both cleared unless overridden). Built as a map, so
// there is no duplicate-entry ambiguity to worry about — never
// os.Environ() appended-to directly, since this host (a typ-fleet crew
// machine) may well already have TYP_CREW_NAME set in the ambient
// environment, which would otherwise silently leak into a test that only
// means to test the argument/HOME it passes explicitly.
func resolverEnv(t *testing.T, home string, overrides map[string]string) []string {
	t.Helper()
	vars := map[string]string{"HOME": home, "MY_TEMPLATE_CREW": "", "TYP_CREW_NAME": ""}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "PATH=") {
			vars["PATH"] = strings.TrimPrefix(e, "PATH=")
		}
	}
	for k, v := range overrides {
		vars[k] = v
	}

	env := make([]string, 0, len(vars))
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	return env
}

// TestI14_ResolverRefusesPresentButEmptyArgument is I14's core test: the
// ported guard (`[ "$#" -gt 0 ] && [ -z "$1" ]`) must refuse a
// present-but-empty argument — the exact mistake `key "$UNSET_VAR"`
// produces — rather than silently falling through to the environment the
// way bash's `${1:-...}` would. This calls the actual generated script via
// exec.Command, not a reimplementation of its logic, so a future edit that
// accidentally reintroduces the ${1:-...} fallthrough would fail this
// test.
func TestI14_ResolverRefusesPresentButEmptyArgument(t *testing.T) {
	requireBash(t)

	home := t.TempDir()
	base := filepath.Join(home, ".my-template")
	scriptPath, err := ensureKeyResolver(base)
	require.NoError(t, err)

	cmd := exec.Command(scriptPath, "") // present, but empty — the exact mistake this guard exists for
	cmd.Env = resolverEnv(t, home, nil)
	out, runErr := cmd.CombinedOutput()

	require.Error(t, runErr, "an empty-but-present argument must be refused, not silently treated as absent")
	assert.Contains(t, string(out), "an argument was given but is empty",
		"must refuse with the specific empty-argument diagnosis, not a generic error")
}

// TestI14_ResolverAcceptsNoArgumentAndFallsBackToEnv is the guard's
// counterpart: truly no argument at all must NOT be refused — only
// present-but-empty is a mistake. Falls back through MY_TEMPLATE_CREW.
func TestI14_ResolverAcceptsNoArgumentAndFallsBackToEnv(t *testing.T) {
	requireBash(t)

	home := t.TempDir()
	base := filepath.Join(home, ".my-template")
	scriptPath, err := ensureKeyResolver(base)
	require.NoError(t, err)

	_, err = writeKeyFile(base, "agent-a", "tpl_fallback_via_env")
	require.NoError(t, err)

	cmd := exec.Command(scriptPath) // no argument at all
	cmd.Env = resolverEnv(t, home, map[string]string{"MY_TEMPLATE_CREW": "agent-a"})
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "tpl_fallback_via_env", strings.TrimSpace(string(out)))
}

// TestI14_IssueAndRotateLeaveWorkingKeyFileForResolver confirms the other
// half of I14: writeKeyFile's output is something the resolver script can
// actually read back — not just that the file exists, but that invoking
// the real resolver with the handle as its argument returns exactly the
// raw key that was written, mode 0600.
func TestI14_IssueAndRotateLeaveWorkingKeyFileForResolver(t *testing.T) {
	requireBash(t)

	home := t.TempDir()
	base := filepath.Join(home, ".my-template")

	keyPath, err := writeKeyFile(base, "agent-a", "tpl_first_issued_key")
	require.NoError(t, err)

	info, err := os.Stat(keyPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "key file must be mode 0600 (I14)")

	scriptPath, err := ensureKeyResolver(base)
	require.NoError(t, err)

	cmd := exec.Command(scriptPath, "agent-a")
	cmd.Env = resolverEnv(t, home, nil)
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "tpl_first_issued_key", strings.TrimSpace(string(out)))

	// Simulate rotate: writeKeyFile called again for the same handle with a
	// new value overwrites the same path — this is the whole mechanism
	// that makes "re-resolve" a one-liner after I13's no-grace-period
	// rotation instead of a manual key hunt.
	_, err = writeKeyFile(base, "agent-a", "tpl_second_rotated_key")
	require.NoError(t, err)

	cmd = exec.Command(scriptPath, "agent-a")
	cmd.Env = resolverEnv(t, home, nil)
	out, err = cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "tpl_second_rotated_key", strings.TrimSpace(string(out)),
		"the resolver must pick up a rotated key on its very next invocation with no other change")
}

// TestI14_ResolverMissingKeyFileFailsClearly — a handle with no key file
// yet (or a typo'd one) must fail with a clear message, not silently
// succeed with empty output.
func TestI14_ResolverMissingKeyFileFailsClearly(t *testing.T) {
	requireBash(t)

	home := t.TempDir()
	base := filepath.Join(home, ".my-template")
	scriptPath, err := ensureKeyResolver(base)
	require.NoError(t, err)

	cmd := exec.Command(scriptPath, "no-such-handle")
	cmd.Env = resolverEnv(t, home, nil)
	out, err := cmd.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(out), "no key file for crew")
}

// TestEnsureKeyResolver_IdempotentWriteIfMissingOrStale exercises the
// "write if missing or stale" rule directly: a first call creates it, a
// second call with unchanged content is a no-op (verified by intentionally
// dirtying the file with different content in between and confirming a
// third call restores the canonical script), and content is always
// exactly resolverScript afterward.
func TestEnsureKeyResolver_IdempotentWriteIfMissingOrStale(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".my-template")

	path, err := ensureKeyResolver(base)
	require.NoError(t, err)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, resolverScript, string(content))

	// Calling again with the file already up to date must be a no-op —
	// content stays exactly resolverScript, no error.
	path2, err := ensureKeyResolver(base)
	require.NoError(t, err)
	assert.Equal(t, path, path2)

	// A stale/edited copy gets regenerated back to resolverScript.
	require.NoError(t, os.WriteFile(path, []byte("#!/usr/bin/env bash\necho this-is-stale\n"), 0o755))
	_, err = ensureKeyResolver(base)
	require.NoError(t, err)
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, resolverScript, string(content), "a stale resolver script must be regenerated back to the current content")
}

func TestWriteKeyFile_CreatesKeysDirAndFileWithMode0600(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".my-template")

	path, err := writeKeyFile(base, "some-handle", "tpl_the_raw_value")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "keys", "some-handle"), path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "tpl_the_raw_value", string(content))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
