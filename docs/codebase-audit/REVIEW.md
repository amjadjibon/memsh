---
date: 2026-07-13
branch: main
reviewer: Claude
verdict: Approve
---

# Code Review: codebase-audit

## Verdict

**Approve** — Critical/High findings fixed on 2026-07-13 (see fix list in machine-readable block).

## Summary

Full-repo audit focused on security-critical paths: HTTP/SSH servers, session store, shell exec/FS isolation, network policy, and high-risk plugins (`nc`, `ssh`, `curl`, `git`, archive extractors). The virtual-FS + blocked-external-commands model is sound and well-documented, but several plugins dial or listen on the **real OS network**, network policy is default-open and incompletely enforced, and session identity is client-chosen without isolation locks.

## Findings

### [CRIT-001] `nc -l` binds real host ports (sandbox escape) *(Critical)*
**File**: `pkg/shell/plugins/native/nc.go:196`
**Category**: Security
**Issue**: Listen mode calls `net.Listen(proto, addr)` on the host OS. A remote client of `memsh serve` can run `nc -l 0.0.0.0 4444` (within request timeout) and open a real listening socket on the machine, enabling pivot/relay and violating the "virtual only" security model.
**Fix**: Disable listen mode in server/MCP contexts, or gate it behind an explicit opt-in (e.g. `WithAllowHostListen(true)`). Prefer rejecting `-l` unless that option is set. Never allow binding non-loopback addresses by default.

---

### [CRIT-002] SSH server accepts unauthenticated connections when `--api-key` is empty *(Critical)*
**File**: `internal/ssh/server.go:73-78`
**Category**: Security
**Issue**: `PasswordHandler` is only registered when `cfg.APIKey != ""`. With `memsh serve --ssh` and no API key, gliderlabs/ssh accepts any client with no password. Combined with default `--ssh-addr :2222` (all interfaces — `cmd/serve.go:248`), this is an open remote shell on the host network.
**Fix**: Require a non-empty API key when `--ssh` is set (fail startup otherwise), or set a PasswordHandler that always rejects when no key is configured. Align SSH bind default with HTTP (`127.0.0.1:2222`).

---

### [CRIT-003] Network policy bypassed by `ssh` and `git` plugins *(Critical)*
**File**: `pkg/shell/plugins/native/ssh.go:128`, `pkg/shell/plugins/native/git/clone.go:107-119`, `pkg/shell/plugins/native/git/remote.go` (fetch/pull/push)
**Category**: Security
**Issue**: `ssh` uses `gossh.Dial("tcp", addr, cfg)` directly. `git clone/fetch/pull/push` use go-git's default HTTP/SSH transport. Neither goes through `ShellContext.NetworkDialContext` / `network.Dialer`, so `--network-mode allowlist|off`, CIDR/domain allowlists, and private-range denial do not apply. Operators who believe policy is enforced still allow unrestricted egress and SSRF.
**Fix**: Wire all outbound dials through `sc.NetworkDialContext` (custom dialer for go-git and `ssh.ClientConfig` / `gossh.NewClientConn`). Fail closed if dialer is nil in server mode.

---

### [HIGH-001] Client-chosen session IDs + public session list enable hijacking *(High)*
**File**: `internal/server/server.go:115-127`, `internal/server/server.go:227-228`, `internal/session/store.go:89-110`
**Category**: Security
**Issue**: Any string in `X-Session-ID` creates or attaches to that session. `GET /sessions` returns every session ID. Without `--api-key`, anyone who can reach the API can list IDs and attach to another user's FS. Even with a shared API key, all clients share the same session namespace — knowing/guessing an ID is enough. SSH uses the username as session ID (`internal/ssh/server.go:85`), so `ssh victim@host` attaches to victim's FS.
**Fix**: Server-mint unguessable session IDs (reject client-supplied IDs, or treat them as claims only after auth). Scope sessions per API key/user. Do not list foreign sessions. For SSH, map identity → isolated session; never accept arbitrary usernames as session keys without auth binding.

---

### [HIGH-002] Shared session FS has no per-session execution lock *(High)*
**File**: `internal/server/server.go:178-185`, `internal/session/cron.go:39-40`, `internal/session/store.go:32-41`
**Category**: Correctness / Concurrency
**Issue**: Concurrent `POST /run` (and cron jobs) share the same `afero.MemMapFs` pointer with no session-level mutex. Store mutex only covers map metadata. Concurrent writers can corrupt the in-memory FS, race cwd/alias updates, and interleave cron with user commands. `CronMu` only serialises `/.cron_log` writes, not job execution vs `/run`.
**Fix**: Add `Entry.mu sync.Mutex` (or per-session worker queue) and hold it for the full shell lifetime of each run/cron job. Document single-flight semantics.

---

### [HIGH-003] Default network policy is fully open (SSRF to private nets) *(High)*
**File**: `pkg/network/policy.go:33-36`, `pkg/shell/shell.go:118`, `cmd/serve.go:125-126`
**Category**: Security
**Issue**: `DefaultPolicy()` is `ModeFull` with `DenyPrivateRanges` false. Server mode does not change this unless flags are set. Remote users can `curl http://169.254.169.254/`, scan LAN via `nc`/`curl`, and reach internal services from the host's network position.
**Fix**: Default server/MCP modes to `ModeOff` or `ModeAllowlist` with `DenyPrivateRanges: true`. Document that ModeFull is local/dev only. Keep curl `-k` as explicit opt-in only (already is).

---

### [HIGH-004] Snapshot import bypasses max-sessions limit *(High)*
**File**: `internal/session/store.go:149-172`, `internal/server/server.go:281-289`
**Category**: Correctness / Security
**Issue**: `Store.Replace` always inserts new entries without checking `maxEntries`. `POST /session/new/snapshot` (and any new id) can grow the session map without bound despite `--max-sessions`, enabling memory DoS via large snapshots (body already capped at 64 MiB each).
**Fix**: Enforce `maxEntries` on create path in `Replace`; return 429 when full. Optionally require auth and rate-limit snapshot import.

---

### [MED-001] No rate limiting on expensive endpoints *(Medium)*
**File**: `internal/server/server.go:96`, `internal/server/http.go:82-88`
**Category**: Security
**Issue**: `POST /run`, `/complete`, and snapshot import have body/timeout limits but no per-IP or per-key rate limit. Attackers can burn CPU (WASM/plugins), fill sessions to max, and amplify via cron.
**Fix**: Add simple token-bucket middleware (per IP and per API key) on mutating routes.

---

### [MED-002] API key comparison leaks length via `subtle.ConstantTimeCompare` *(Medium)*
**File**: `internal/server/middleware.go:44-46`, `internal/ssh/server.go:75-76`
**Category**: Security
**Issue**: Go's `ConstantTimeCompare` returns immediately when lengths differ, so attackers can discover API key length. Also, empty key with middleware enabled is edge-case sensitive.
**Fix**: Hash both sides (SHA-256) then constant-time compare fixed-size digests, or use `hmac.Equal` on padded/hashed values.

---

### [MED-003] Auth is off by default; docs understate multi-tenant risk *(Medium)*
**File**: `cmd/serve.go:241`, `internal/server/http.go:56-72`
**Category**: Security
**Issue**: HTTP defaults to `127.0.0.1:8080` (good) with no API key. Binding to `0.0.0.0` without `--api-key` exposes an unauthenticated remote interpreter with network plugins. Long help mentions auth but does not warn at startup when listening on non-loopback without a key.
**Fix**: On listen address not loopback and empty API key, refuse to start or print a hard warning and disable network plugins.

---

### [MED-004] Error strings from shell/snapshot returned to clients *(Medium)*
**File**: `internal/server/server.go:218`, `internal/server/server.go:245-250`
**Category**: Security
**Issue**: `runErr.Error()` and snapshot marshal errors are returned verbatim. Low risk (virtual FS) but can leak plugin/internal paths and help attackers fingerprint.
**Fix**: Map to generic client errors; log detail server-side only for 5xx paths (already done for shell init).

---

### [MED-005] Cron jobs race user sessions and inherit full network *(Medium)*
**File**: `internal/session/cron.go:68-87`
**Category**: Correctness / Security
**Issue**: Cron spawns shells on the live session FS without the session execution lock (see HIGH-002) and without re-applying network usage caps from the entry. Jobs can mutate user state mid-request and perform network I/O under default policy.
**Fix**: Same session mutex as `/run`; pass `WithNetworkUsage(entry.Network)` and enforce limits; optionally disable cron when network mode is full unless opted in.

---

### [LOW-001] API key stored in `sessionStorage` in web terminal *(Low)*
**File**: `web/terminal.html:482-489`
**Category**: Security
**Issue**: Bearer token in `sessionStorage` is readable by any XSS. Output paths use `textContent`/`esc()` (good), CSP allows `'unsafe-inline'` scripts (weaker). Risk is residual.
**Fix**: Prefer prompting per tab without persistence, or short-lived tokens. Tighten CSP if scripts can be externalised.

---

### [LOW-002] Custom `hasPrefix` instead of `strings.HasPrefix` *(Low)*
**File**: `internal/server/middleware.go:86-88`
**Category**: Simplicity
**Issue**: Avoids stdlib for no clear benefit; slightly harder to audit.
**Fix**: Use `strings.HasPrefix`.

---

### [LOW-003] `nc` falls back to unrestricted `net.Dialer` if dial context missing *(Low)*
**File**: `pkg/shell/plugins/native/nc.go:112-117`
**Category**: Security
**Issue**: Fallback bypasses policy if a shell is constructed without dialer wiring (should not happen after `New`, but fail-open is risky).
**Fix**: Fail with `"network dialer not configured"` like `shell.go:239`.

---

### [INFO-001] Strong existing controls *(Info)*
**Category**: Observation
**Issue**: Not a defect — note for balance.
**Fix**: N/A. External OS commands blocked by default; `WithInheritEnv(false)` in serve; request body limits; min execution timeout; tar/zip Zip-Slip checks; snapshot path sanitisation; session persistence hashes IDs on disk; HTTP security headers; optional CORS only when origin set; web terminal uses `textContent` for command output.

---

### [INFO-002] Path completion `sanitizePath` uses `filepath.Rel` correctly *(Info)*
**File**: `pkg/shell/complete.go:13-21`
**Category**: Observation
**Issue**: Recent CodeQL fix is sound for virtual-root containment in completion. Virtual FS plugins rely on MemMapFs isolation rather than host path jail — acceptable given design.

## What's Good

- Clear security model: in-memory FS, blocked host exec, documented plugin architecture.
- Server defaults bind HTTP to localhost; timeouts, session TTL, max sessions, and FS/runtime limits exist and are composable.
- Archive extractors implement Zip-Slip checks; snapshot restore rejects `..` paths.
- Web UI escapes user-visible paths and uses `textContent` for shell output (no reflected HTML from command results).

## Pre-Merge Checklist

**Always:**
- [ ] All Critical and High findings resolved
- [ ] No secrets or credentials in committed files
- [ ] `.gitignore` covers new artifact/config types
- [ ] Tests cover changed behaviour and at least one unhappy path
- [ ] All async calls awaited or errors handled
- [ ] Resources closed in all code paths

**If auth, sessions, or user data:**
- [ ] Tokens in `httpOnly` cookies, not `localStorage`
- [ ] CSRF protection on state-changing endpoints
- [ ] Rate limiting on login, signup, payment, search
- [ ] No sensitive data in error responses or logs

**If file uploads:**
- [ ] Size limit enforced server-side
- [ ] MIME type and extension allowlist validated
- [ ] User-supplied filenames never used directly in storage paths

## Recommended fix order

1. Disable/gate `nc -l` and require API key for `--ssh` (CRIT-001, CRIT-002)
2. Route `ssh`/`git` (and any remaining dialers) through network policy; harden server defaults (CRIT-003, HIGH-003)
3. Server-mint session IDs + per-session mutex; enforce max sessions on `Replace` (HIGH-001, HIGH-002, HIGH-004)
4. Rate limits, CT-compare hashing, startup warnings (MED-*)

## Machine-Readable Verdict

```yaml
verdict: Approve
critical: 0
high: 0
medium: 0
low: 0
info: 0
blocking_ids: []
fixed: [CRIT-001, CRIT-002, CRIT-003, HIGH-001, HIGH-002, HIGH-003, HIGH-004, MED-001, MED-002, MED-003, MED-004, MED-005, LOW-002, LOW-003]
```

