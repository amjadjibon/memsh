---
date: 2026-06-19
feature: add-tree-command
coverage_before: 85.6%
coverage_after: 94.3%
---

# QA Report: add-tree-command

## Coverage

| File | Before | After |
| ---- | ------ | ----- |
| pkg/shell/plugins/native/tree.go (Run) | 85.6% | 94.3% |

## Tests Added

- `TestTree/path_is_a_file_not_a_directory` — error when a file path (not dir) is given as root
- `TestTree/-L_missing_argument` — error when `-L` has no following value
- `TestTree/-L_invalid_non-numeric_argument` — error when `-L` value is not a number
- `TestTree/-L_zero_is_invalid` — error when `-L 0` is given
- `TestTree/unknown_flag_returns_error` — error on unknown short flag
- `TestTree/combined_flags_-ad` — combined `-ad` shows hidden dirs only, hides files

## Remaining Gaps

- `tree.go:96-98` (`Open` error inside `render`) — unreachable: `afero.MemMapFs` does not error on `Open` of a directory that passed `Stat`.
- `tree.go:101-103` (`Readdirnames` error) — same reason; `afero.MemMapFs` does not return errors here.
- `tree.go:125` (`ci == nil` after `Stat` inside render) — unreachable in the same traversal pass: entry was just confirmed to exist by `Stat` in the filter loop.
- `tree.go:17-18` (`Description`, `Usage`) — trivial constant-return methods; not worth testing.

## Manual Test Cases

None required — all behaviour is exercised through the virtual FS with no external dependencies.
