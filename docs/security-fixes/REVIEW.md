---
date: 2026-07-13
branch: main (uncommitted security fixes)
reviewer: Claude
verdict: Request Changes
---

# Code Review: security-fixes

## Verdict

**Request Changes** — Core sandbox and auth hardening is solid, but a few correctness/security gaps remain (loopback detection, rate-limit map growth, residual session hijack surface, incomplete git SSH transport).

## Summary

Reviewed the uncommitted security-hardening diff (~360 LOC across server, session, shell, git/nc/ssh plugins, web UI). Critical sandbox escapes (`nc -l`, unauthenticated SSH, policy-bypassing dials) are addressed well. Remaining issues are medium: mis-classified bind addresses in the non-loopback warning, unbounded rate-limit state, client-chosen session IDs still creatable, and git SSH not using the policy dialer for the actual transport.

## Findings

### [MED-001] `isLoopbackAddr` treats `:port` / empty host as loopback *(Medium)*
**File**: `cmd/serve.go` (`isLoopbackAddr`)
**Category**: Security / Correctness
**Issue**: `net.SplitHostPort(":8080")` yields `host == ""`, and the function returns `true` for empty host. Binding `:8080` listens on **all interfaces**, not loopback, so the “non-loopback without --api-key” warning is suppressed when operators use the most common insecure form.
**Fix**:
```go
if host == "" || host == "0.0.0.0" || host == "::" {
    return false
}
```

---

### [MED-002] Rate-limit map grows without eviction *(Medium)*
**File**: `internal/server/middleware.go` (`RateLimitMiddleware`)
**Category**: Correctness / Performance
**Issue**: `buckets map[string]*bucket` never deletes expired or idle entries. Unique spoofed IPs (or real high-churn clients) grow the map without bound → memory DoS. Combined with MED-003 this is easy to amplify.
**Fix**: Periodically prune entries with `now.After(b.reset)`, or use a fixed-size LRU / single global token bucket for v1.

---

### [MED-003] `X-Forwarded-For` trusted unconditionally *(Medium)*
**File**: `internal/server/middleware.go` (`clientIP`)
**Category**: Security
**Issue**: Any client can set `X-Forwarded-For: <random>` and bypass the per-IP rate limit when memsh is not behind a trusted reverse proxy. Default deployment is often direct-to-process.
**Fix**: Only honor `X-Forwarded-For` when an explicit flag (e.g. `--trust-proxy`) is set; otherwise use `RemoteAddr` only.

---

### [MED-004] Client-chosen session IDs still create sessions *(Medium)*
**File**: `internal/server/server.go` (`handleRun`), `internal/session/store.go` (`Get`)
**Category**: Security
**Issue**: `X-Session-ID: new` mints a strong ID (good), but any other client-supplied string still `Get`s which **creates** that session. `GET /sessions` still lists all IDs. Predictable IDs remain hijackable; “new” alone does not close the original HIGH-001 surface.
**Fix**: Prefer: (1) create only via `Create()` / `"new"`, (2) attach only to existing IDs (404 if missing), (3) optionally require high-entropy ID format if client supply is kept for compat.

---

### [MED-005] Snapshot/complete read session FS without `ExecMu` *(Medium)*
**File**: `internal/server/server.go` (`handleSnapshotGet`, `handleComplete`)
**Category**: Correctness / Concurrency
**Issue**: `/run` and cron hold `entry.ExecMu`, but snapshot export and completion walk the same `afero.MemMapFs` unlocked. Concurrent mutation can race or yield torn snapshots.
**Fix**: `entry.ExecMu.Lock()` around snapshot take and complete FS reads (or a shared RLock if you split read/write locks later).

---

### [MED-006] Git SSH remotes only preflight-dial; transport not policy-wired *(Medium)*
**File**: `pkg/shell/plugins/native/git/transport.go`
**Category**: Security
**Issue**: `withPolicyHTTPTransport` installs custom clients for `http`/`https` only. For `git@host:` / `ssh://`, `checkRemoteURL` opens one policy dial then closes it; go-git’s default SSH transport dials again **outside** `NetworkDialContext` (metering bypass; any future per-connection controls missed). HTTP path is correct.
**Fix**: Install a custom go-git SSH transport with `DialContext` → `NetworkDialContext`, or reject non-HTTP remotes when a dialer is present until SSH is wired.

---

### [LOW-001] `checkRemoteURL` double-dials and burns network quota *(Low)*
**File**: `pkg/shell/plugins/native/git/transport.go:75-79`
**Category**: Performance
**Issue**: Preflight TCP connect counts as a metered request, then clone/fetch dials again. Can trip `net-max-requests` early and add latency.
**Fix**: Expose `network.Dialer` / policy `EvaluateAddress` without connecting for preflight; rely on transport dial for enforcement.

---

### [LOW-002] `network.ServerPolicy` is unused *(Low)*
**File**: `pkg/network/policy.go`
**Category**: Simplicity
**Issue**: Added helper is never called; serve already gets `DenyPrivateRanges` via `parseNetworkPolicy`. Dead surface for readers.
**Fix**: Use `ServerPolicy()` in serve/MCP base opts, or drop the export until needed.

---

### [LOW-003] Duplicated API-key hash compare *(Low)*
**File**: `internal/server/middleware.go` (`secureCompare`), `internal/ssh/server.go` (`secureAPIKeyEqual`)
**Category**: Simplicity
**Issue**: Same SHA-256 + `ConstantTimeCompare` helper in two packages.
**Fix**: Shared `internal/auth` or `pkg/security` helper.

---

### [LOW-004] Missing unit tests for new server paths *(Low)*
**File**: `internal/server/`, `internal/session/`
**Category**: Test Quality
**Issue**: Diff adds `Create`, `Replace` bool, rate limit, SSH API-key required, session mutex, generic errors — only `nc` tests were extended. No tests for rate limit, Replace at max, `X-Session-ID: new`, or SSH New() without key.
**Fix**: Add focused table tests for store Create/Replace limits and middleware rate limit / API key compare; one `internalssh.New` error case without key.

---

### [INFO-001] Generic `"command failed"` is intentional *(Info)*
**File**: `internal/server/server.go`
**Category**: Observation
**Issue**: Client-visible shell errors are collapsed; details go to server logs. Safer for multi-tenant, worse for API debugging. Document in serve help / API docs.

## What's Good

- `nc -l` fail-closed + opt-in `WithAllowHostListen` is the right default for a virtual shell.
- SSH refuses to start without API key; password compare is length-safe; default bind is localhost.
- HTTP `ssh` client and git HTTP remotes go through `NetworkDialContext` / custom go-git HTTP client under a global mutex.
- Per-session `ExecMu` shared with cron closes a real FS race; `Replace` respects max sessions.

## Pre-Merge Checklist

**Always:**
- [ ] All Critical and High findings resolved
- [x] No secrets or credentials in committed files
- [x] `.gitignore` covers new artifact/config types
- [ ] Tests cover changed behaviour and at least one unhappy path
- [x] All async calls awaited or errors handled
- [x] Resources closed in all code paths

**If auth, sessions, or user data:**
- [ ] Tokens in `httpOnly` cookies, not `localStorage` (API key still in sessionStorage — pre-existing)
- [x] Rate limiting present (needs MED-002/003 fixes before relying on it)
- [x] No sensitive data in error responses for command failures

## Machine-Readable Verdict

```yaml
verdict: Request Changes
critical: 0
high: 0
medium: 6
low: 4
info: 1
blocking_ids: []
```
