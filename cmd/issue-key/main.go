// Command issue-key is the identity module's CLI key-issuance script
// (task-2.md) — the only place a tpl_ API key is minted. There is
// deliberately no POST /api/v1/keys (_contract/API.md): issuance is
// CLI-only, mirroring my-task's `agent:add`.
//
// Usage: go run ./cmd/issue-key <handle>
//
// If handle doesn't already exist as a users row, one is created
// (role='agent', active, no sso_subject — API-key-only identity per
// DATA_MODEL.md) before the key is issued. The raw key is printed to
// stdout exactly once; it is never stored anywhere (I8) and cannot be
// recovered later — only rotated (run this command again) or revoked.
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
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: issue-key <handle>")
	}
	flag.Parse()

	handle := flag.Arg(0)
	if handle == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(handle); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(handle string) error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	db, err := platform.OpenDB(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	repo := identity.NewRepo(db)
	// No JWT verifier needed — this script only ever issues API keys.
	svc := identity.NewService(repo, repo, nil, slog.Default())

	result, err := svc.IssueAPIKeyForHandle(context.Background(), handle)
	if err != nil {
		return fmt.Errorf("issuing key for %q: %w", handle, err)
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
