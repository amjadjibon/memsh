---
date: 2026-07-15
feature: improve-code-coverage
coverage_before: 5.9%
coverage_after: 6.6%
---

# QA Report: improve-code-coverage

## Coverage

| Package | Before | After |
| ---- | ------ | ----- |
| `internal/config` | 0.0% | 90.5% |
| `internal/paths` | 0.0% | 72.7% |
| `pkg/cron` | 0.0% | 97.1% |
| `pkg/shell/plugins` | 0.0% | 100.0% |
| total | 5.9% | 6.6% |

## Tests Added

- `TestPathHelpersUseHomeDirectory` — verifies file path helpers resolve under `HOME`.
- `TestDirectoryHelpersCreateDirectories` — verifies directory helpers create expected directories.
- `TestFileHelpersDoNotCreateMemshDir` — verifies file path helpers do not create directories as a side effect.
- `TestLoadReturnsDefaultsWhenConfigMissing` — verifies missing config returns default settings.
- `TestLoadReadsConfigFile` — verifies valid TOML populates shell and plugin config fields.
- `TestLoadReturnsContextForInvalidConfig` — verifies invalid TOML returns path context.
- `TestBuildShellOptsDisablesConfiguredPlugins` — verifies generated shell options disable configured builtins while preserving unrelated commands.
- `TestParseCronExprMatchesExpectedMinute` — verifies cron schedules match only expected minutes.
- `TestParseCronExprNormalizesSundaySeven` — verifies day-of-week `7` is accepted as Sunday.
- `TestParseCronExprAcceptsDescriptors` — verifies descriptor expressions such as `@hourly` parse and match.
- `TestParseCrontabParsesJobsAndSkipsComments` — verifies crontab comments, blank lines, standard entries, and descriptor entries.
- `TestParseCrontabReportsHelpfulErrors` — verifies malformed crontab entries return useful errors.
- `TestShellCtxReturnsZeroValueWhenMissing` — verifies missing shell context returns a zero-value context.
- `TestWithShellContextInjectsContext` — verifies injected shell context is retrievable with fields and callbacks intact.

## Remaining Gaps

- `pkg/shell/plugins/native` remains reported at 0.0% in package coverage because most command behavior is covered through the `tests` integration package rather than package-local unit tests.
- `cmd`, `internal/mcp`, and `internal/repl` remain at 0.0%; these require CLI/server/repl-specific coverage work outside this focused helper-package pass.

## Manual Test Cases

- [x] None required; this was a deterministic unit-test-only coverage pass.

## Verification

- `go test ./internal/paths ./internal/config ./pkg/cron ./pkg/shell/plugins`
- `go test ./... -coverprofile=coverage.out`
- `go tool cover -func=coverage.out` reported `total: (statements) 6.6%`.
