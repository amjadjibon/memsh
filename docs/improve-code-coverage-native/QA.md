---
date: 2026-07-16
feature: improve-code-coverage-native
coverage_before: 3.4%
coverage_after: 73.3%
---

# QA Report: improve-code-coverage-native

## Coverage

| Package | Before | After |
| ------- | ------ | ----- |
| `pkg/shell/plugins/native` | 3.4% | 73.3% |
| Repo overall (`go test ./...`) | ~46% (native package barely counted) | 49.3% |

`pkg/shell/plugins/native` had almost no *in-package* tests — its commands were
only exercised indirectly through `tests/*_test.go`, which don't count toward
the package's own coverage number since Go attributes coverage to the test
binary built for each package directory.

## Approach

1. Ported ~40 existing `tests/*_test.go` integration suites (awk, grep, jq,
   yq, tar, sqlite, ssh, uuid, watch, etc.) into
   `pkg/shell/plugins/native/*_test.go` as `package native_test`, importing
   both `pkg/shell` and the sibling `tests` package for `NewTestShell`. This
   alone raised coverage from 3.4% → 55.7% by re-using already-correct test
   scripts, just from a directory whose test binary actually counts.
2. Wrote new tests for ~40 previously-untested plugins: `bc`, `cd`, `chmod`,
   `clear`/`reset`, `cp`, `cut`, `date`, `df`, `du`, `env`/`printenv`, `exit`,
   `expr`, `head`/`tail`, `help`/`man`, `ln`, `ls`, `mkdir`, `mv`, `rm`,
   `rmdir`, `sed`, `sort`, `source`/`.`, `stat`, `tee`, `touch`, `uniq`,
   `which`, `xargs`, `yes`.

## Tests Added

Roughly 150 new test cases across 45 new files (see `git status` for the
full list), each covering the happy path plus 1-3 error/edge cases per
command (missing operand, invalid flag, missing file, etc.).

## Significant Finding: several native plugins are dead code

While writing tests for `cd`, `pwd`, `read`, `echo`, `printf`, `true`,
`source`, `.`, and `help`, I found that **mvdan.cc/sh's `interp.Runner`
intercepts these command names unconditionally** via its internal
`IsBuiltin()` check in `Runner.call()` (`interp/runner.go`), before our
`Shell.execHandler` — and therefore the matching native plugin — ever runs.
This contradicts the comment in `AGENTS.md` ("the plugin name `cd` works
because the exec handler runs first"): verified empirically that even
`shell.WithBuiltin("cd", spy)` never fires for a literal `cd` command.

Consequences:
- `CdPlugin`, `PwdPlugin`, `ReadPlugin`, `EchoPlugin`, `PrintfPlugin`,
  `TruePlugin`, `SourcePlugin`, `DotPlugin`, `HelpPlugin`, and `TimePlugin`
  (the last via mvdan's `TimeClause` keyword, not `IsBuiltin`) can **never
  run when their name is the literal leading word of a command** — mvdan's
  own builtin of the same name always wins. It happens to work correctly in
  practice because mvdan's builtins are already wired to the virtual FS via
  the interpreter's open/readdir handlers, so nothing is visibly broken —
  but the native plugin code is unreachable dead weight.
- These plugins *are* reachable when invoked dynamically as an argument to
  another command that calls `sc.Exec` directly (e.g. `echo hi | xargs echo
  -ne`, `echo hi | xargs time echo -n`), since that path goes through
  `Shell.execArgs` → `Shell.execHandler`, skipping mvdan's `IsBuiltin` gate
  entirely. I used this `xargs` trick to get real coverage on `cd`, `source`,
  `.`, `echo`, `printf`, `pwd`, and `time` (see their `_test.go` files for
  comments explaining why).
- `read.go` (`ReadPlugin.Run`) uses `interp.HandlerCtx(ctx)` and could only be
  reached the same way, but `xargs` already drains stdin gathering items
  before it would get a chance to read further input, so I could not devise a
  clean test for it without invasive changes. Left uncovered.
- `TruePlugin.Run` (`true.go`) is a one-line `return nil` shadowed the same
  way; not worth a synthetic unit test for a single trivial statement (YAGNI).

This is not something I fixed — it's a pre-existing architectural quirk, out
of scope for a test-only QA pass — but worth a maintainer decision: either
delete these unreachable plugins (they add no behavior since mvdan's own
builtins already cover the same commands against the virtual FS) or restructure
so real invocations reach them, e.g. by not registering builtin-shadowed names
as native plugins at all, and updating `AGENTS.md`'s incorrect claim about `cd`.

## Remaining Gaps

- `pkg/shell/plugins/native/git` — coverage unchanged at 2.9%; out of scope
  for this pass (separate subpackage, would need its own QA pass).
- `read.go` — unreachable via any test short of restructuring the codebase
  (see finding above).
- `true.go`'s `TruePlugin.Run` — unreachable, trivial one-liner, low value.
- Some flag-parsing branches in already-tested files (e.g. combined
  short-flag forms like `-n2` on `head`/`tail`) are still untested; the happy
  path and primary error paths are covered, but exhaustive flag-combination
  coverage was left for a future pass.

## Manual Test Cases

None — all new tests are automated and run in `go test ./pkg/shell/plugins/native/...`.
