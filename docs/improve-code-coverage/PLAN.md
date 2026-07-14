---
goal: Improve code coverage
version: 1.0
date_created: 2026-07-15
last_updated: 2026-07-15
owner: maintainers
status: 'Planned'
tags: [chore]
---

# Improve Code Coverage

![Status: Planned](https://img.shields.io/badge/status-Planned-blue)

Add focused tests around small, stable packages that currently report 0% coverage. This raises meaningful coverage without changing runtime behavior or introducing flaky integration dependencies.

## 1. Requirements & Constraints

- **REQ-001**: Add tests that cover deterministic behavior in low-coverage packages.
- **REQ-002**: Keep implementation changes test-only unless a test exposes a real bug.
- **CON-001**: Use existing Go test tooling and avoid new dependencies.
- **CON-002**: Do not rely on the user's real home directory contents.

## 2. Implementation Steps

> After each phase: `git add -u` and commit. No `Co-authored-by:`. Tick `[x]` as each task completes.

### Phase 1: Add Unit Coverage For Stable Helpers

**Goal**: Cover deterministic helper packages and shell plugin context helpers with targeted unit tests.

- [ ] TASK-001: Add `internal/paths/paths_test.go` covering path resolution and directory creation with `t.Setenv("HOME", tmp)`.
- [ ] TASK-002: Add `internal/config/config_test.go` covering defaults, valid TOML, invalid TOML, disabled WASM, plugin allowlist, and disabled plugins.
- [ ] TASK-003: Add `pkg/cron/cron_test.go` covering `ParseCronExpr`, Sunday `7` normalization, `CronMatches`, `ParseCrontab`, comments, and parse errors.
- [ ] TASK-004: Add `pkg/shell/plugins/plugin_test.go` covering missing and injected shell context behavior.

**Completion criteria**: `go test ./internal/paths ./internal/config ./pkg/cron ./pkg/shell/plugins ./... -coverprofile=coverage.out` passes and overall coverage is higher than the 5.9% baseline.

**git commit**: `git add -u && git add internal/paths/paths_test.go internal/config/config_test.go pkg/cron/cron_test.go pkg/shell/plugins/plugin_test.go && git commit -m "test: cover stable helper packages"`

**Agent Prompt**:
```
You are a sub-agent implementing Phase 1 of improve-code-coverage.

Context: The repository baseline coverage is 5.9%. This phase adds deterministic unit tests for low-risk helper packages without changing production behavior.
Branch: improve-code-coverage  |  Base: main

Tasks:
- TASK-001: Add internal/paths/paths_test.go covering path resolution and directory creation with t.Setenv("HOME", tmp).
- TASK-002: Add internal/config/config_test.go covering defaults, valid TOML, invalid TOML, disabled WASM, plugin allowlist, and disabled plugins.
- TASK-003: Add pkg/cron/cron_test.go covering ParseCronExpr, Sunday 7 normalization, CronMatches, ParseCrontab, comments, and parse errors.
- TASK-004: Add pkg/shell/plugins/plugin_test.go covering missing and injected shell context behavior.

Key files:
- internal/paths/paths.go — path helpers under HOME.
- internal/config/config.go — config loader and shell option builder.
- pkg/cron/cron.go — cron parsing helpers.
- pkg/shell/plugins/plugin.go — plugin context helpers.

Completion criteria: go test ./internal/paths ./internal/config ./pkg/cron ./pkg/shell/plugins ./... -coverprofile=coverage.out passes and overall coverage is higher than the 5.9% baseline.

When done: git add -u && git add internal/paths/paths_test.go internal/config/config_test.go pkg/cron/cron_test.go pkg/shell/plugins/plugin_test.go && git commit -m "test: cover stable helper packages" — no Co-authored-by.
Reply with a one-paragraph summary and commit SHA.
Do NOT push, open PRs, or modify PLAN.md.
```

## 3. Testing

- [ ] TEST-001: Run `go test ./internal/paths ./internal/config ./pkg/cron ./pkg/shell/plugins`.
- [ ] TEST-002: Run `go test ./... -coverprofile=coverage.out` and compare to the 5.9% baseline.

## 4. Risks & Assumptions

- **RISK-001**: Coverage can be inflated with shallow tests — mitigation: assert observable behavior and error paths, not just constructors.
- **RISK-002**: Tests touching HOME can pollute the developer environment — mitigation: use `t.Setenv("HOME", t.TempDir())` everywhere.
- **ASSUMPTION-001**: Improving coverage means adding meaningful tests, not altering coverage settings.
- **ASSUMPTION-002**: The safest initial targets are low-coverage helper packages with deterministic behavior and no external services.
- **ASSUMPTION-003**: Overall coverage baseline from `go test ./... -coverprofile=coverage.out` is 5.9%.
