---
date: 2026-07-24
branch: fix-cd-permission-denied
reviewer: Claude
verdict: Approve
---

# Code Review: fix-cd-permission-denied

## Verdict

**Approve** — small, correct dependency swap plus a matching signature update; no behavior regression.

## Summary

Reviewed the diff removing the `github.com/amjadjibon/sh/v3` fork replace directive (go.mod/go.sum, excluded from line-by-line review as lockfile churn) and the accompanying change to `pkg/shell/shell.go`'s `accessHandler`. Upstream `mvdan.cc/sh` merged `AccessHandler` support (fixing mvdan/sh#1318, cd into virtual directories failing with "permission denied") with `mode` now typed as `interp.AccessMode`, a bitmask, instead of an ad-hoc `uint32` with exact-match semantics. The local handler was updated to match: independent bit checks instead of a `switch`, so callers requesting combined checks (e.g. read+write) are now handled correctly, matching the upstream behavior change noted in the fix commit.

## Findings

None.

## What's Good

- Bitmask handling (`mode&interp.AccessRead != 0`, etc.) correctly generalizes from the old exact-match `switch`, matching upstream's documented behavior fix for combined access checks.
- Verified manually: `mkdir /newdir; cd /newdir; pwd` now succeeds (previously the exact bug in the issue).
- Full test suite (`go test ./...`) passes.

## Pre-Merge Checklist

- [x] All Critical and High findings resolved (none found)
- [x] No secrets in committed files
- [x] Tests cover changed behaviour — existing shell/cd test suite exercises this path; manual repro confirms the fix
- [x] No async/resource-lifecycle concerns (pure sync handler, no I/O beyond existing `s.fs.Stat`)
- [x] Not applicable: auth/uploads

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
