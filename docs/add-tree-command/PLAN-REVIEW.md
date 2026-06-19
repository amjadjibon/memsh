---
date: 2026-06-19
plan: docs/add-tree-command/PLAN.md
plan_version: 1.0
reviewer: Claude
verdict: Ready
---

# Plan Review: add-tree-command

## Verdict

**Ready** — single-phase plan is well-scoped, tasks are concrete, and all assumptions are documented.

## Findings

### [SUGGEST-001] PAT-002 contradicts ASSUMPTION-003 on `-L` parsing
**Phase**: 1 (TASK-002)
**Issue**: PAT-002 says `-La 2` (combined flag) is valid, but ASSUMPTION-003 says `-L2` is not supported and `-L` takes the _next_ argument. These are contradictory: if `-L` is in a combined short-flag cluster like `-La`, there is no "next argument" for the depth value — parsing will fail or silently break. The plan should state which wins: combined-flag loop OR `-L` always standalone.
**Fix**: Clarify in REQ or PAT that `-L` must appear alone (not combined with other flags), mirroring how `find -maxdepth` is handled in `find.go`. Remove `-La 2` from the PAT-002 example.

---

### [SUGGEST-002] Summary line counts are ambiguous when `-d` is active
**Phase**: 1 (TASK-004)
**Issue**: TASK-004 says "files hidden by `-d`… are not counted." So `tree -d` on a dir with 3 files and 2 subdirs would print `2 directories, 0 files`. Standard `tree(1)` does _not_ count files at all in `-d` mode (omits the file count entirely or shows 0). Either behaviour is fine, but the test in TASK-006 doesn't include a `dirs only -d` sub-test that verifies the summary — only that files are absent from the tree output.
**Fix**: Add a `dirs only summary` assertion in TASK-006 verifying the exact summary format when `-d` is active.

---

## What's Good

- **Single-phase is the right call**: implementation + registration + tests in one phase is appropriate here — the feature has no migration, no new dependency, and no staged rollout concern.
- **Concrete task decomposition**: TASK-001 through TASK-006 each have a single, unambiguous action with exact file paths and function names — no vague "refactor" language.
- **Risk documentation is accurate**: RISK-001 (sort order) and ASSUMPTION-001 (`Readdirnames`) directly reference the `ls.go` precedent, which is exactly the right way to anchor an assumption.

## Machine-Readable Verdict

```yaml
verdict: Ready
block: 0
revise: 0
suggest: 2
blocking_ids: []
```
