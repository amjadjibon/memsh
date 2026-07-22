---
date: 2026-07-22
branch: fix-codeql-alerts
reviewer: Claude
verdict: Approve
---

# Code Review: fix-codeql-alerts

## Verdict

**Approve** — all nine CodeQL alerts are fixed with minimal, targeted diffs; full test suite passes.

## Summary

Reviewed the fix for CodeQL alerts #12–#20: weak-hash-on-sensitive-data (API key comparisons in
`internal/ssh/server.go` and `internal/server/middleware.go`), log injection from user-controlled
session IDs (`internal/session/store.go`, `internal/server/server.go`), a duplicate-`if`-branches
bug in `git ls-tree -l` (`pkg/shell/plugins/native/git/inspect.go`), and two useless assignments
(`native/uuid.go`, `native/shuf.go`). All fixes are correct, scoped to the flagged lines, and the
`ls-tree` fix is a real behavior bug (long format silently dropped blob size for single-file
lookups) with a new regression test.

## Findings

None.

## What's Good

- The crypto fix (bare SHA-256 → keyed HMAC-SHA256 + `hmac.Equal`) is the right remediation for
  CodeQL's weak-hashing rule without changing the constant-time-comparison behavior the original
  code relied on.
- The `ls-tree -l` fix isn't just a cosmetic dedup — it restores the missing size field to match
  the recursive/directory-listing code paths, and ships with a regression test
  (`tests/git_test.go`) asserting field counts for both `-l` and non-`-l` output.
- Diffs are minimal — each fix touches only the flagged lines, no unrelated refactors.

## Pre-Merge Checklist

- [x] All Critical and High findings resolved (none found)
- [x] No secrets in committed files
- [x] Tests cover changed behaviour (`git ls-tree -l` regression test added; existing
      `TestSecureCompare`/`TestSecureAPIKeyEqual` cover the crypto change)
- [x] All async calls awaited or errors handled; resources closed in all paths (n/a — no new I/O)
- [x] If auth/user data: httpOnly tokens, CSRF protection, rate limits, no sensitive data in
      errors/logs (log lines now quote user-controlled IDs to prevent injection)
- [x] If uploads: n/a

## Machine-Readable Verdict

```yaml
verdict: Approve
critical: 0
high: 0
medium: 0
low: 0
info: 0
blocking_ids: []
```
