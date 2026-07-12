---
goal: Close residual findings from security-fixes code review
version: 1.0
date_created: 2026-07-13
last_updated: 2026-07-13
owner: memsh
status: 'Planned'
tags: [bug, security]
---

# Security Fixes — Residual Review Items

![Status: Planned](https://img.shields.io/badge/status-Planned-blue)

Close Medium findings from `docs/security-fixes/REVIEW.md` so the security-hardening branch can ship: correct loopback detection, trustworthy rate limiting, tighter session attachment, FS locks on snapshot/complete, and git SSH policy wiring.

## 1. Requirements & Constraints

- **REQ-001**: All MED findings from `docs/security-fixes/REVIEW.md` resolved
- **SEC-001**: Rate limiting cannot be trivially bypassed via spoofed XFF without opt-in
- **SEC-002**: Session attach does not create sessions from arbitrary client IDs
- **CON-001**: Keep backward compat for web UI (client can still use a known existing session ID)
- **CON-002**: No new dependencies; stdlib only

## 2. Implementation Steps

> After completing all tasks, `git add -u` and commit. No `Co-authored-by:`. Tick `[x]` as each task completes.

### Phase 1: Residual security review fixes

**Goal**: Fix MED-001–MED-006 and related LOWs in one commit on `security-fixes`.

**Depends on**: Existing hardening commit on branch

- [ ] TASK-001: Fix `isLoopbackAddr` so empty host / `0.0.0.0` / `::` are non-loopback (`cmd/serve.go`)
- [ ] TASK-002: Prune rate-limit buckets; only honor XFF when `--trust-proxy` is set (`internal/server/middleware.go`, `http.go`, `cmd/serve.go`)
- [ ] TASK-003: Session attach-only for existing IDs; create only via `Create`/`"new"`; add `Store.GetExisting` (`internal/session/store.go`, `internal/server/server.go`)
- [ ] TASK-004: Hold `ExecMu` on snapshot get and complete FS access (`internal/server/server.go`)
- [ ] TASK-005: Wire git SSH through policy dialer or reject non-HTTP remotes when dialer present (`pkg/shell/plugins/native/git/transport.go`)
- [ ] TASK-006: Prefer policy evaluate without preflight TCP dial if available; otherwise document double-dial tradeoff and use evaluate-only helper
- [ ] TASK-007: Use `network.ServerPolicy()` from serve base opts or remove dead export
- [ ] TASK-008: Add tests for loopback helper, rate limit XFF, session attach-only, Replace max, SSH New without key

**Completion criteria**: `make lint && go test ./... -count=1` pass; MED items from REVIEW.md addressed.

**git commit**: `git add -u && git commit -m "fix: close residual security review findings"`

**Agent Prompt**:
```
You are a sub-agent implementing Phase 1 of security-fixes.

Context: Branch already hardens nc/ssh/git/sessions. This phase closes residual Medium findings from docs/security-fixes/REVIEW.md.

Branch: security-fixes  |  Base: main

Tasks:
- TASK-001: Fix isLoopbackAddr for empty host / 0.0.0.0 / ::
- TASK-002: Rate limit prune + --trust-proxy for XFF
- TASK-003: Session attach-only (GetExisting); create via Create/"new"
- TASK-004: ExecMu on snapshot get and complete
- TASK-005: Git SSH policy dial or reject non-HTTP remotes
- TASK-006: Avoid preflight TCP dial when possible
- TASK-007: Wire or drop ServerPolicy
- TASK-008: Tests for the above

Key files:
- cmd/serve.go
- internal/server/middleware.go, http.go, server.go
- internal/session/store.go
- pkg/shell/plugins/native/git/transport.go
- pkg/network/policy.go, dialer.go

Completion criteria: make lint && go test ./... -count=1 pass

When done: git add -u && git commit -m "fix: close residual security review findings" — no Co-authored-by
Write a one-paragraph summary of changes and commit SHA.
Do NOT push, open PRs, or modify PLAN.md.
```

## 3. Testing

- [ ] TEST-001: Unit test isLoopbackAddr cases including `:8080`, `0.0.0.0:8080`, `127.0.0.1:8080`
- [ ] TEST-002: Rate limit without trust-proxy ignores XFF
- [ ] TEST-003: handleRun with unknown session ID does not create when using attach-only
- [ ] TEST-004: Store Replace at max returns false
- [ ] TEST-005: internal/ssh New fails without API key

## 4. Risks & Assumptions

- **RISK-001**: Attach-only sessions break clients that relied on first request creating arbitrary IDs — mitigation: web UI already generates random IDs and sends them; first Get creates… wait, attach-only means first request with client ID must create OR we use GetOrCreate only for valid hex. ASSUMPTION: web generates random 16-byte hex; allow create on first use only if ID matches `^[a-f0-9]{16,64}$`, else require "new". Simpler: GetOrCreate stays for client IDs that look random (16+ hex), reject weak IDs. REVIEW said attach-only if missing 404. Web always sets session before first run - so first Get would 404 if we don't create. So we need GetOrCreate for first request OR web must call Create first.

**ASSUMPTION-001**: Keep GetOrCreate for client-supplied IDs that match high-entropy hex (`^[a-f0-9]{16,64}$`); reject short/guessable IDs with 400. `"new"` always mints. This balances web UX with hijack resistance.

- **ASSUMPTION-002**: Rejecting git SSH remotes (only allow http/https) is acceptable if wiring full SSH transport is large; prefer reject with clear error when dialer present and scheme is ssh.
