---
date: 2026-07-13
branch: security-fixes
reviewer: Claude
verdict: Approve
---

# Code Review: security-fixes (iteration 1 residual)

## Verdict

**Approve** — Residual Medium findings from the prior review pass are addressed; no Critical/High issues remain in the fix set.

## Summary

Reviewed the residual-fix commit on `security-fixes` after the initial sandbox hardening. Loopback detection, rate-limit memory/XFF trust, session ID entropy, ExecMu on snapshot/complete, and git SSH rejection are in place. Tests cover the new paths. Remaining notes are Low/Info only.

## Findings

### [LOW-001] Session Get still creates after GetExisting miss *(Low)*
**File**: `internal/server/server.go` (handleRun default branch)
**Category**: Simplicity
**Issue**: Flow is GetExisting then Get (create). Correct for web first-use of a pre-generated hex ID, but two store lookups. Acceptable; optional micro-optimize later with GetOrCreateValidated.
**Fix**: Optional — leave as-is for clarity.

### [INFO-001] Git SSH remotes rejected rather than dial-wired *(Info)*
**File**: `pkg/shell/plugins/native/git/transport.go`
**Category**: Observation
**Issue**: Intentional: clear error forces HTTPS so policy dialer applies. Documented in error string.

## What's Good

- `isLoopbackAddr` correctly treats empty host / `0.0.0.0` / `::` as non-loopback
- Rate limit prunes expired buckets; XFF only with `TrustProxy`
- Hex session IDs + `"new"` mint path; weak IDs return 400
- Snapshot/complete take `ExecMu`; tests for rate limit, ValidSessionID, Replace max

## Pre-Merge Checklist

**Always:**
- [x] All Critical and High findings resolved
- [x] No secrets or credentials in committed files
- [x] Tests cover changed behaviour and at least one unhappy path
- [x] Resources closed in all code paths

## Machine-Readable Verdict

```yaml
verdict: Approve
critical: 0
high: 0
medium: 0
low: 1
info: 1
blocking_ids: []
```
