---
date: 2026-06-19
branch: tree-command/phase-1
reviewer: Claude
verdict: Approve
---

# Code Review: add-tree-command

## Verdict

**Approve** — implementation is correct and secure; two low-severity simplification opportunities noted.

## Summary

Reviewed `pkg/shell/plugins/native/tree.go` (155 lines), the one-line registration in `pkg/shell/defaults.go`, and `tests/tree_test.go` (255 lines). The plugin follows all established patterns correctly — virtual-FS-only I/O, `interp.HandlerCtx` for output, combined short-flag loop, compile-time interface assertion. No correctness bugs, no security issues. Two low findings: a redundant `Stat` call per entry and a subtle in-place slice filter that trades clarity for a minor allocation saving.

## Findings

### [LOW-001] Double `Stat` call per entry in the render loop *(Low)*
**File**: `pkg/shell/plugins/native/tree.go:112-125`
**Category**: Simplicity / Performance
**Issue**: The filter loop calls `sc.FS.Stat(childPath)` to check `ci.IsDir()` and get the `ci` value, but then discards it. The render loop immediately calls `sc.FS.Stat(childPath)` again on the same path to get `ci` for the directory check and recursion decision. Every entry incurs two stat calls.
**Fix**: Store `(name, isDir bool)` pairs in `filtered` instead of just names, carrying the `IsDir()` result from the filter loop into the render loop:
```go
type treeEntry struct{ name string; isDir bool }
var filtered []treeEntry
// filter loop: filtered = append(filtered, treeEntry{name, ci.IsDir()})
// render loop: use entry.isDir directly — no second Stat needed
```

---

### [LOW-002] In-place slice filter reuses `names` backing array *(Low)*
**File**: `pkg/shell/plugins/native/tree.go:107`
**Category**: Simplicity
**Issue**: `filtered := names[:0]` creates a zero-length slice sharing `names`'s backing array, then appends into it while `range names` reads ahead. This is safe — appends only overwrite positions already visited — but it's a non-obvious pattern that will surprise maintainers unfamiliar with the Go in-place filter idiom.
**Fix**: Use a plain `var filtered []string` (or `make([]string, 0, len(names))`). The allocation is small and only happens once per directory level; clarity is worth more than the saved allocation here.

---

## What's Good

- **Flag parsing is correct**: `-L` is handled as a standalone flag before the combined-flag loop — exactly the right approach for a flag that consumes the next argument, consistent with how `find.go` handles `-name` and `-type`.
- **Tests are behavioural, not implementation-coupled**: assertions check observable output (connector characters, path strings, summary counts) rather than internal state, so they'll survive refactors.
- **Error messages are actionable**: `tree: 'path': No such file or directory` and `tree: invalid argument 'X' to '-L'` mirror `tree(1)` output, which makes shell scripts that parse error messages portable.

## Pre-Merge Checklist

**Always:**
- [x] All Critical and High findings resolved
- [x] No secrets or credentials in committed files
- [x] Tests cover the changed behaviour and at least one unhappy path
- [x] Resources (files, connections, streams) closed in all code paths

## Machine-Readable Verdict

```yaml
verdict: Approve
critical: 0
high: 0
medium: 0
low: 2
info: 0
blocking_ids: []
```
