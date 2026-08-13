# Task 3 Report

## Task
Fix the confirmed JWKS kid-miss defect in already-shipped `internal/
identity/jwt.go`: force exactly one `Cache.Refresh()` + retry on a lookup
miss, bounded so a bad-kid probe can't force unbounded issuer calls.
`docs/DEPLOY-REQUIREMENTS.md` update superseding §7.4's restart
expectation for this specific case.

## Outcome
done

## Decision

- **Issue:** where to put the retry — inside `rs256KeyProvider.FetchKeys`
  (scoped precisely to a key-lookup miss) or by catching `jwt.Parse`'s
  error in `Verify` and retrying the whole parse (can't distinguish a
  genuine kid-miss from other parse failures — wrong issuer, expired
  token, bad signature — that shouldn't trigger a Hydra refresh at all).
- **Chosen:** `FetchKeys`. Also simplified `Verify` by removing its now-
  redundant upfront `cache.Lookup` call.

## Notes

- **Independently re-verified (chief-agent/Luna):** ran the two new
  `TestI7_...` tests directly, both pass. Read
  `TestI7_UnknownKidFailsWithBoundedRefresh`'s source myself rather than
  trusting "bounded" from the report — it asserts an `atomic.Int64`
  counter on the fake JWKS server's actual request count equals exactly 2
  (1 startup fetch + 1 forced refresh), not an inference from pass/fail.
  This is a real test of the DoS-prevention property, not just of the
  happy path.
- `go test ./...` — exactly the expected I11–I14 gap, nothing else;
  `invariants_test.go` untouched.
- Commit: `22b253f` on `milestone-2/close-parity-gap`.

No blockers, no escalation.
