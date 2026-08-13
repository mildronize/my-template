// Command seed provisions this template's one seeded row: the owner
// `users` row. Mirrors my-task's `seed.ts` in purpose only, not in what
// it seeds — this template has no fixed statuses/projects the way
// my-task does (this domain doesn't have that concept, DATA_MODEL.md),
// so this command creates exactly one thing and nothing speculative.
//
// The owner row is seeded, never JIT-created (`_rules/_contract/
// DATA_MODEL.md`'s "Owner provisioning" note, `_rules/_contract/
// INVARIANTS.md` I10) — a template's owner is a single, known person per
// deployment, whoever forked it, identified in advance by their real
// Hydra `sub` claim. That claim comes from SEED_OWNER_SSO_SUBJECT
// (internal/platform/config.go); the handle is a fixed literal ("owner"),
// the same way role and active are fixed — nothing about *which* human
// this is varies per fork the way the sso_subject does.
//
// Usage:
//
//	SEED_OWNER_SSO_SUBJECT=<hydra-sub> go run ./cmd/seed
//
// Idempotent by check-then-insert on sso_subject (the unique constraint
// this row's identity actually rests on — DATA_MODEL.md's `users` table),
// not a blind upsert: a first run creates the row; every run after that
// finds it and leaves it alone, reporting so, never erroring and never
// creating a second row.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
)

// ownerHandle is the fixed handle the seeded owner row gets — not
// configurable, the same way role='owner' and active=true aren't: only
// the sso_subject varies per deployment (SEED_OWNER_SSO_SUBJECT), because
// that's the piece that actually identifies *which* human this is.
const ownerHandle = "owner"

func main() {
	cfg, err := platform.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	db, err := platform.OpenDB(cfg.DatabasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	defer db.Close()

	// Migrating here too (not just cmd/server) means this command works
	// standalone against a fresh volume, the same reasoning cmd/issue-key
	// gives for doing the same (see its own main.go).
	if err := platform.Migrate(db); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	repo := identity.NewRepo(db)

	result, err := seedOwner(context.Background(), repo, cfg.SeedOwnerSSOSubject)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if result.alreadyExisted {
		fmt.Printf("Owner already exists (handle=%q, id=%s, sso_subject=%q) — left alone.\n",
			result.user.Handle, result.user.ID, result.user.SSOSubject)
		return
	}
	fmt.Printf("Seeded owner row (handle=%q, id=%s, sso_subject=%q).\n",
		result.user.Handle, result.user.ID, result.user.SSOSubject)
}

// seedResult is what seedOwner needs to report — the resolved row, and
// whether this call created it or found it already there.
type seedResult struct {
	user           identity.User
	alreadyExisted bool
}

// seedOwner is main's testable core: check-then-insert on sso_subject,
// the unique natural key this row's identity actually rests on
// (DATA_MODEL.md's `users` table — `sso_subject` is `unique, nullable`,
// same as `handle`, but `sso_subject` is the one SEED_OWNER_SSO_SUBJECT
// actually varies per fork/deployment, so it's the key this check uses).
//
// Two edge cases handled explicitly, not left to fail confusingly:
//
//   - ssoSubject == "" (SEED_OWNER_SSO_SUBJECT not set): a clear error,
//     never a row created with an empty sso_subject.
//   - a second run for the same subject: reports "already exists, left
//     alone" via alreadyExisted=true, nil error — never a duplicate-key
//     database error, because the existence check runs first and this
//     function returns before ever calling CreateUser again.
func seedOwner(ctx context.Context, repo *identity.Repo, ssoSubject string) (seedResult, error) {
	if ssoSubject == "" {
		return seedResult{}, errors.New(
			"SEED_OWNER_SSO_SUBJECT is not set — refusing to seed an owner row with an empty sso_subject; " +
				"set it to the owner's known Hydra `sub` claim (see _rules/_contract/DATA_MODEL.md's " +
				"\"Owner provisioning\" note)")
	}

	existing, err := repo.GetUserBySSOSubject(ctx, ssoSubject)
	if err == nil {
		return seedResult{user: existing, alreadyExisted: true}, nil
	}
	if !errors.Is(err, identity.ErrNotFound) {
		return seedResult{}, fmt.Errorf("looking up existing owner by sso_subject: %w", err)
	}

	created, err := repo.CreateUser(ctx, ownerHandle, "owner", &ssoSubject)
	if err != nil {
		return seedResult{}, fmt.Errorf("creating owner row: %w", err)
	}
	return seedResult{user: created, alreadyExisted: false}, nil
}
