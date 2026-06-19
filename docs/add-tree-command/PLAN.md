---
goal: Add a `tree` native plugin command that renders directory contents as an ASCII tree
version: 1.0
date_created: 2026-06-19
last_updated: 2026-06-19
owner: amjadjibon
status: 'Planned'
tags: [feature]
---

# Add `tree` Command

![Status: Planned](https://img.shields.io/badge/status-Planned-blue)

Add a `tree` command to memsh that displays directory contents as an indented ASCII tree, operating entirely on the virtual `afero.MemMapFs`. Supports depth limiting, hidden-file filtering, directory-only mode, and full-path output — matching common `tree(1)` behaviour.

## 1. Requirements & Constraints

- **REQ-001**: `tree [path]` prints an ASCII directory tree rooted at `path` (defaults to cwd).
- **REQ-002**: `-L <n>` limits recursion to `n` levels deep.
- **REQ-003**: `-a` includes hidden entries (names starting with `.`).
- **REQ-004**: `-d` prints directories only.
- **REQ-005**: `-f` prints the full absolute path for each entry instead of just the name.
- **REQ-006**: Entries within each directory are sorted alphabetically.
- **REQ-007**: A summary line is printed at the end: `N directories, M files`.
- **CON-001**: All file I/O must go through `plugins.ShellCtx(ctx).FS` — no real OS filesystem access.
- **CON-002**: Output must go to `interp.HandlerCtx(ctx).Stdout` so it works correctly in pipes and redirects.
- **PAT-001**: Follow the existing native plugin struct pattern: `TreePlugin{}` with `Name/Description/Usage/Run` methods, registered in `defaultNativePlugins()` under the filesystem section.
- **PAT-002**: Flag parsing follows the combined short-flag loop convention: `-ad`, `-La 2` are valid; `--` stops flag parsing.
- **GUD-001**: Filename `pkg/shell/plugins/native/tree.go`; test file `tests/tree_test.go`.

## 2. Implementation Steps

> **Agent instructions**: After completing all tasks in a phase, `git add -u` (plus explicit paths for new files) and commit. No `Co-authored-by:` trailers. Tick checkboxes `[x]` as each task is completed.

### Phase 1: Implement `tree` plugin and tests

**Goal**: Ship the complete `tree` command — implementation, registration, and tests — in a single cohesive phase.

- [ ] TASK-001: Create `pkg/shell/plugins/native/tree.go`. Define `TreePlugin{}` struct implementing `plugins.Plugin` and `plugins.PluginInfo`. `Name()` returns `"tree"`, `Description()` returns `"list directory contents in a tree-like format"`, `Usage()` returns `"tree [-a] [-d] [-f] [-L depth] [path]"`.

- [ ] TASK-002: Implement `TreePlugin.Run`. Parse args with the combined short-flag loop: `-a` (showHidden), `-d` (dirsOnly), `-f` (fullPath), `-L <n>` (maxDepth, default -1 = unlimited); positional arg sets the root path (resolved via `sc.ResolvePath`; defaults to `sc.Cwd`). Return `tree: invalid argument '<v>' to '-L'` if the value is not a positive integer.

- [ ] TASK-003: Implement recursive tree rendering. Print the root path on the first line. For each entry in a directory (sorted, hidden filtered, dirs-only filtered): compute the tree connector (`├── ` for non-last, `└── ` for last entry), print the connector + name (or full path if `-f`), then recurse into sub-directories with the appropriate indent prefix (`│   ` for non-last parent, `    ` for last parent). Stop recursing when `maxDepth >= 0` and current depth equals `maxDepth`. Count total directories and files encountered during traversal.

- [ ] TASK-004: Print the summary line after the tree: `\n<N> directories, <M> files` (root directory is not counted in the directory total; files hidden by `-d` or not matching `-a` are not counted).

- [ ] TASK-005: Register `native.TreePlugin{}` in `defaultNativePlugins()` in `pkg/shell/defaults.go`, inside the `// filesystem` block, after `native.FindPlugin{}`.

- [ ] TASK-006: Create `tests/tree_test.go`. Write `TestTree` with sub-tests:
  - `basic tree` — root with two files and one sub-directory; verify connector symbols and entry names appear in output.
  - `depth limit -L 1` — deep structure; verify entries beyond depth 1 are absent.
  - `hidden files -a` — directory containing `.hidden`; verify it appears with `-a` and is absent without.
  - `dirs only -d` — mixed files and dirs; verify only directory names appear, files absent.
  - `full paths -f` — verify output contains absolute paths instead of bare names.
  - `summary line` — verify output ends with `N directories, M files`.
  - `non-existent path` — verify `Run` returns a non-nil error containing the path name.

**Completion criteria**: `go test ./tests -run TestTree -v` passes with all seven sub-tests green; `go test ./...` continues to pass; `go vet ./...` reports no issues.

**git commit**: `git add pkg/shell/plugins/native/tree.go pkg/shell/defaults.go tests/tree_test.go && git commit -m "feat: add tree command plugin with depth, hidden, dirs-only, full-path flags"` — no `Co-authored-by:` trailer

---

## 3. Testing

- [ ] TEST-001: `go test ./tests -run TestTree -v` — all seven sub-tests pass.
- [ ] TEST-002: `go test ./...` — full suite still green (no regressions).
- [ ] TEST-003: `go vet ./...` — no issues reported.

## 4. Risks & Assumptions

- **ASSUMPTION-001**: `afero.MemMapFs` `Open().Readdirnames(-1)` returns all entries for a directory without error; the same pattern is used in `ls.go` and is considered reliable.
- **ASSUMPTION-002**: The tree connectors use UTF-8 box-drawing characters (`├`, `└`, `│`) — the same style as the standard `tree(1)` command. Tests check for these characters being present.
- **ASSUMPTION-003**: The `-L` flag takes the next argument as its value (not combined, e.g. `-L2` is not supported; `-L 2` is). This matches standard `tree(1)` behaviour and avoids ambiguity with the combined-flag loop.
- **RISK-001**: `afero.MemMapFs` does not guarantee directory entry order — mitigation: always `sort.Strings` entry names before rendering, as done in `ls.go`.
