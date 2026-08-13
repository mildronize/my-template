// I14 (_rules/_contract/INVARIANTS.md): `issue` and `rotate` both leave
// behind a key file and a resolver script, so "rotation invalidates
// immediately, re-resolve to recover" (I13's no-grace-period design) is
// actually one command, not a manual copy-paste out of a terminal
// scrollback. This file owns both pieces of that mechanism; main.go calls
// it from both run() (issue) and runRotate().
//
// The resolver script's content (resolverScript, below) is ported
// verbatim from `~/.my-task/bin/key` per _goal/GOAL.md's "Key resolver
// script — port verbatim, don't rewrite from the description" decision —
// only names changed (MY_TASK_CREW -> MY_TEMPLATE_CREW, ~/.my-task/ ->
// ~/.my-template/, and the my-task-specific comment references). The
// empty-argument guard, its exact reasoning, the fallback chain, and the
// 0600-is-a-rule-not-isolation point all carry over unchanged in spirit.
package main

import (
	"os"
	"path/filepath"
)

// myTemplateBaseDir resolves ~/.my-template — the root both the key
// directory and the resolver's bin directory live under. Kept as its own
// function (rather than inlined at each call site) so the two production
// callers (run, runRotate) and tests agree on exactly what "home" means;
// tests never call this directly, they pass an explicit baseDir to
// writeKeyFile/ensureKeyResolver instead, which is what makes both
// functions testable without touching a real developer's actual $HOME.
func myTemplateBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".my-template"), nil
}

// writeKeyFile writes rawKey to <baseDir>/keys/<handle>, mode 0600 (I14).
// Called by both issue and rotate, every time — a rotated key must
// overwrite the same path an earlier issue/rotate wrote, which is exactly
// what makes the resolver script's next invocation pick up the new value
// with no other change needed anywhere.
func writeKeyFile(baseDir, handle, rawKey string) (string, error) {
	keysDir := filepath.Join(baseDir, "keys")
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(keysDir, handle)
	if err := os.WriteFile(path, []byte(rawKey), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// ensureKeyResolver writes <baseDir>/bin/key if it's missing or its
// content has drifted from the current resolverScript, and leaves it
// alone otherwise — my-task's own ensureKeyResolver pattern (task-5.md).
// The script's content is static (it doesn't embed the handle or
// anything else per-invocation — it reads the handle from its own
// argument/environment at run time), so "idempotent, write only if
// missing or stale" is the whole rule: a developer's own local edits to
// this file would otherwise get silently clobbered on every single
// `issue`/`rotate` call if this always wrote unconditionally.
func ensureKeyResolver(baseDir string) (string, error) {
	binDir := filepath.Join(baseDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(binDir, "key")

	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == resolverScript {
		return path, nil // already up to date — nothing to do
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	if err := os.WriteFile(path, []byte(resolverScript), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// resolverScript is ~/.my-task/bin/key, ported verbatim (see this file's
// package comment) — only MY_TASK_CREW -> MY_TEMPLATE_CREW,
// ~/.my-task/ -> ~/.my-template/, and the my-task-specific framing in the
// comments changed. Everything else — the empty-argument-is-a-mistake
// guard and its exact error message, the argument -> MY_TEMPLATE_CREW ->
// TYP_CREW_NAME fallback chain and the reasoning for why the last step is
// host-specific, and the 0600-is-a-rule-not-isolation point — carries over
// on purpose, per _goal/GOAL.md: a resolver written fresh from "resolve
// crew from arg or env" would re-lose the empty-argument guard, which is
// the one part of this script that isn't obvious from a description of
// what it does.
const resolverScript = `#!/usr/bin/env bash
set -euo pipefail
# Resolves this service's agent API key without ever putting the value into
# an agent's own context. Invoke it via shell expansion so the value flows
# straight into the request and is never echoed, logged, or read into an
# LLM's context window (see _rules/_contract/INVARIANTS.md, I14):
#
#   curl -H "Authorization: Bearer $(~/.my-template/bin/key)" ...
#
# Call it with NO ARGUMENT wherever the environment already says which crew
# this is, which is the documented form and the only one a skill should show.
# Resolution order: explicit argument, then MY_TEMPLATE_CREW, then
# TYP_CREW_NAME.
#
# The last one is host-specific on purpose. This service and its companion
# skill know only MY_TEMPLATE_CREW; mapping that to whatever a particular
# host calls a crew belongs in this resolver, which is a host-side artifact
# rather than part of the app. Moving to a host that names crews differently
# is a one-line edit here and touches nothing else. Without it there is
# nowhere per-crew to set MY_TEMPLATE_CREW at all on a host where every crew
# shares one HOME.

# An argument that is PRESENT BUT EMPTY is a mistake, not an omission — it is
# almost always "$SOME_VAR" where the variable is unset. Bash's ${1:-...}
# treats that identically to no argument at all and falls through to the
# environment, so the call succeeds, returns the caller's own key, and looks
# right. That is how a documented-but-wrong invocation once survived
# undetected on my-task's own host: a skill showed key "$MY_TASK_CREW",
# MY_TASK_CREW was set nowhere, and the accidental fallthrough made a
# forbidden form work anyway (found by tiana, reading the guide cold).
# Refuse it here too, before a fork of this template repeats the same
# mistake on a host of its own.
if [ "$#" -gt 0 ] && [ -z "$1" ]; then
  echo "key: an argument was given but is empty — a variable in it is unset." >&2
  echo "     If you meant 'this crew', pass no argument at all: \$(~/.my-template/bin/key)" >&2
  exit 1
fi

CREW="${1-${MY_TEMPLATE_CREW:-${TYP_CREW_NAME:-}}}"
if [ -z "$CREW" ]; then
  echo "usage: key <crew>   (or set MY_TEMPLATE_CREW)" >&2
  exit 1
fi

KEY_FILE="$HOME/.my-template/keys/$CREW"
if [ ! -f "$KEY_FILE" ]; then
  echo "no key file for crew '$CREW' at $KEY_FILE" >&2
  exit 1
fi

# 0600 on the key file above is a rule, not an isolation guarantee: every
# crew on a shared-uid host can read every other crew's key file regardless
# of the mode bit. It exists so a deliberate widening of that mode is
# visible in a diff, not to keep this file private from anything running as
# the same user.
cat "$KEY_FILE"
`
