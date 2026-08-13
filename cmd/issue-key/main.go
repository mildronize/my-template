// Command issue-key is the identity module's CLI key lifecycle script
// (task-2.md issuance, task-5.md rotation) — the only place a tpl_ API key
// is minted or rotated. There is deliberately no POST /api/v1/keys and no
// key-rotation HTTP endpoint either (_contract/API.md's explicit "do not
// add a rotate endpoint for symmetry with GET/DELETE" line): both issuance
// and rotation are CLI-only, mirroring my-task's `agent:add`/`agent:
// rotate`.
//
// Usage:
//
//	go run ./cmd/issue-key <handle>           # issue
//	go run ./cmd/issue-key -rotate <handle>   # rotate (I13)
//
// issue: if handle doesn't already exist as a users row, one is created
// (role='agent', active, no sso_subject — API-key-only identity per
// DATA_MODEL.md) before a key is issued.
//
// rotate: handle must already exist — there is nothing to rotate for a
// handle nothing was ever issued to. A new key is issued FIRST, and only
// once that succeeds are handle's other live keys disabled (I13 — the
// reverse of my-task's actual disable-then-issue `cmdRotate` ordering,
// which produces a real gap where the caller holds zero valid keys).
// Deliberately no grace period beyond that reorder — see
// _rules/_contract/INVARIANTS.md I13.
//
// Both paths print the raw key to stdout exactly once — it is never
// stored anywhere (I8) and cannot be recovered later, only rotated again
// or revoked — AND (I14) write it to ~/.my-template/keys/<handle> (mode
// 0600) and ensure ~/.my-template/bin/key (a resolver script ported from
// my-task's own, see keyfile.go) exists, so recovering after a rotation is
// `$(~/.my-template/bin/key)`, not a manual copy-paste out of a terminal.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
)

func main() {
	rotate := flag.Bool("rotate", false, "rotate handle's key instead of issuing a fresh one (I13: issues the new key before disabling the old one(s))")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: issue-key [-rotate] <handle>")
	}
	flag.Parse()

	handle := flag.Arg(0)
	if handle == "" {
		flag.Usage()
		os.Exit(2)
	}

	var err error
	if *rotate {
		err = runRotate(handle)
	} else {
		err = run(handle)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// newIdentityService loads config, opens (and migrates) the database, and
// wires an *identity.Service — the setup shared by both run (issue) and
// runRotate. Migrating here too (not just cmd/server) means this command
// works standalone against a fresh volume — e.g. the first thing run in
// the docker-compose flow, before the server container has started.
func newIdentityService() (*identity.Service, func() error, error) {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}

	db, err := platform.OpenDB(cfg.DatabasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening database: %w", err)
	}

	if err := platform.Migrate(db); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("applying migrations: %w", err)
	}

	repo := identity.NewRepo(db)
	// No JWT verifier needed — this command only ever issues/rotates API
	// keys.
	svc := identity.NewService(repo, repo, nil, slog.Default())
	return svc, db.Close, nil
}

// persistKeyAndResolver is I14's whole point: both run and runRotate call
// this right after they have a raw key in hand, so a key file and a
// working resolver are guaranteed to exist by the time either command's
// stdout output is the only thing left for an operator to read.
func persistKeyAndResolver(handle, rawKey string) error {
	base, err := myTemplateBaseDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}
	if _, err := writeKeyFile(base, handle, rawKey); err != nil {
		return fmt.Errorf("writing key file: %w", err)
	}
	if _, err := ensureKeyResolver(base); err != nil {
		return fmt.Errorf("ensuring resolver script: %w", err)
	}
	return nil
}

func run(handle string) error {
	svc, closeDB, err := newIdentityService()
	if err != nil {
		return err
	}
	defer closeDB()

	result, err := svc.IssueAPIKeyForHandle(context.Background(), handle)
	if err != nil {
		return fmt.Errorf("issuing key for %q: %w", handle, err)
	}

	if err := persistKeyAndResolver(handle, result.RawKey); err != nil {
		return fmt.Errorf("issued the key but failed to persist it for the resolver: %w", err)
	}

	fmt.Println()
	fmt.Printf("Issued a new key for %q (user id: %s, role: %s).\n", handle, result.User.ID, result.User.Role)
	fmt.Println("Copy it now — it will not be shown again:")
	fmt.Println()
	fmt.Println(result.RawKey)
	fmt.Println()
	fmt.Printf("Prefix: %s  Expires: %s\n", result.APIKey.KeyPrefix, result.APIKey.ExpiresAt.Format("2006-01-02"))

	return nil
}

func runRotate(handle string) error {
	svc, closeDB, err := newIdentityService()
	if err != nil {
		return err
	}
	defer closeDB()

	result, err := svc.Rotate(context.Background(), handle)
	if err != nil {
		return fmt.Errorf("rotating key for %q: %w", handle, err)
	}

	if err := persistKeyAndResolver(handle, result.RawKey); err != nil {
		return fmt.Errorf("rotated the key but failed to persist it for the resolver: %w", err)
	}

	fmt.Println()
	fmt.Printf("Rotated the key for %q (user id: %s, role: %s) — %d old key(s) disabled.\n", handle, result.User.ID, result.User.Role, result.RevokedCount)
	fmt.Println("Copy the new key now — it will not be shown again. The old key(s) are already")
	fmt.Println("invalid (I13: no grace period by design) — anything still holding the old")
	fmt.Println("value in a shell variable must re-run the resolver, not reuse it:")
	fmt.Println()
	fmt.Println(result.RawKey)
	fmt.Println()
	fmt.Printf("Prefix: %s  Expires: %s\n", result.APIKey.KeyPrefix, result.APIKey.ExpiresAt.Format("2006-01-02"))

	return nil
}
