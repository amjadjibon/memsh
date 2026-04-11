# Improvements

## High Priority

- [ ] **Split `git/git.go` into per-subcommand files**
  - File: `pkg/shell/plugins/native/git/git.go` (1167 lines, 13 subcommands)
  - Each `cmdGit*` function is self-contained — move each into its own file:
    `clone.go`, `commit.go`, `log.go`, `diff.go`, `branch.go`, `checkout.go`,
    `reset.go`, `show.go`, `stash.go`, `config.go`, etc.
  - Keep the dispatch `switch` in `git.go`
  - No behavior changes

- [ ] **Replace `eino` compose graph with a plain loop in `internal/agent/`**
  - File: `internal/agent/agent.go`
  - ~200 lines of graph nodes, edges, branches, state handlers, checkpoint store,
    and interrupt mechanics implement one thing: a tool-calling loop
  - Replace with a `for` loop: call model → check tool calls → run tools → repeat
  - Drop the `github.com/cloudwego/eino` dependency from the agent package entirely

- [ ] **Move Ruby-specific WASM hacks into plugin config**
  - File: `pkg/shell/plugin.go` (lines 166–202)
  - Two `if name == "ruby"` blocks inject `-W0` and `RUBYOPT` inside the generic runner
  - Add a `WASIConfig` struct to the plugin registry so each WASM plugin declares
    its own extra args and env vars; generic runner stays generic

- [ ] **Replace bubble sort in `session.Store.List()`**
  - File: `internal/session/store.go` (lines 147–151)
  - O(n²) nested loops — replace with `slices.SortFunc` or `sort.Slice`

---

## Medium Priority

- [ ] **Fix unbounded linear scan in `allocFd`**
  - File: `pkg/shell/plugin.go` (`allocFd` function)
  - Loops from fd=3 upward until a gap is found — degrades with many open files
  - Replace with a free-list or an incrementing counter with a cleanup pass

- [ ] **Document plugin loading priority inside `plugin.go`**
  - File: `pkg/shell/plugin.go`
  - Five priority levels (WithPluginBytes → native → defaultPlugins → virtual FS → real FS)
    are only documented in CLAUDE.md
  - Add a comment block at the top of the file listing the levels in order

- [ ] **Reduce repetitive error handling in `composeGraph`**
  - File: `internal/agent/agent.go` (`composeGraph` function)
  - 10+ sequential `if err != nil { return nil, fmt.Errorf("add node %s: %w", ...) }` blocks
  - If `eino` is kept: extract a `mustAddNode` helper
  - Preferred fix: item #2 above (replace the graph entirely)

- [ ] **Add tests for `internal/agent/`**
  - No `*_test.go` files exist in the agent package
  - At minimum: construct an agent with a mock model and verify the tool-call loop
    terminates correctly on both tool-call and non-tool-call responses

---

## Low Priority

- [ ] **Give `session.Store.reap()` a shutdown path**
  - File: `internal/session/store.go` (`reap` method and `New` function)
  - Background goroutine runs forever with no way to stop it
  - Accept a `context.Context` in `New()` and stop the ticker when the context
    is cancelled to avoid goroutine leaks on server shutdown

- [ ] **Validate conflicting `WithDisabledPlugins` and `WithPluginFilter` options**
  - File: `pkg/shell/options.go` and `pkg/shell/shell.go` (`New`)
  - Both options affect plugin loading but conflicting configs (e.g. filter to
    `["python"]` then disable `"python"`) fail silently
  - Document precedence explicitly or add a validation check in `New()`

- [ ] **Add timeout to `sourceURL`**
  - File: `pkg/shell/shell.go` (`sourceURL` function, line ~283)
  - Uses bare `net/http.Get` with no timeout — a slow remote URL blocks the session
  - Replace with `http.NewRequestWithContext(ctx, "GET", url, nil)` so the shell's
    context cancellation propagates

- [ ] **Group entries in `defaultNativePlugins()` with section comments**
  - File: `pkg/shell/defaults.go`
  - 80+ plugin registrations with no visual grouping
  - Add blank-line comments (`// filesystem`, `// text processing`, `// scripting`,
    `// data tools`, `// network`, `// archive`, `// utilities`) to make the list
    scannable
