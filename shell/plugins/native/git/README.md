# git plugin

A `git` command implementation for memsh backed entirely by the virtual
`afero.MemMapFs` filesystem. No real OS filesystem is touched; all repository
objects and working-tree files live in memory.

## How it works

### afero → billy bridge (`afero_billy.go`)

go-git requires a [`billy.Filesystem`](https://github.com/go-git/go-billy)
for both the object store (`.git/`) and the working tree. `aferoFS` adapts
`afero.Fs` to that interface:

| billy interface | afero call |
|----------------|-----------|
| `billy.Basic` | `Create`, `Open`, `OpenFile`, `Stat`, `Rename`, `Remove` |
| `billy.Dir` | `ReadDir` → `afero.ReadDir`, `MkdirAll` |
| `billy.Symlink` | `Lstat` → `Stat` (no-op); `Symlink` no-op; `Readlink` identity |
| `billy.TempFile` | `afero.TempFile` |
| `billy.Chroot` | returns a new `aferoFS` rooted at the sub-path |

`openStorage(fs, root)` creates a `filesystem.Storage` rooted at
`<root>/.git` using this bridge and an LRU object cache.

`openRepo(fs, cwd)` walks up from `cwd` to find `.git`, then opens the
repository with a storage and worktree both backed by the virtual FS.

## Implemented commands

| Command | Flags / notes |
|---------|--------------|
| `git init [<dir>]` | Creates `.git/` in the virtual FS |
| `git clone <url> [<dir>]` | `-b`/`--branch`, `--depth[=N]`, `--single-branch`, `-n`/`--no-checkout`; clones into virtual FS via go-git HTTP transport |
| `git add <path>…` | `.`, `-A`, `--all`, or explicit relative/absolute paths |
| `git rm <path>…` | `--cached` (accepted, index always updated) |
| `git status` | Shows branch, staged changes, unstaged changes, untracked files |
| `git commit -m <msg>` | `--allow-empty`, `--author "Name <email>"` |
| `git log` | `--oneline`, `-n <N>`, `-<N>` |
| `git diff` | `--cached` / `--staged` for index diff; default shows working-tree diff |
| `git branch [<name>]` | Create branch; `-d`/`-D` delete; list with `*` prefix on current |
| `git checkout <branch\|file>` | `-b` creates branch; file path restores from HEAD |
| `git reset [<ref>]` | `--soft`, `--mixed` (default), `--hard` |
| `git show [<rev>]` | Defaults to HEAD; shows commit metadata and patch |
| `git config <key> [<value>]` | `user.name`, `user.email` get/set; bare invocation lists both |
| `git -C <path> <cmd>` | Repeatable; changes the working directory for the operation |

## Missing commands

### High priority (core workflow)

| Command | Notes |
|---------|-------|
| `git remote` | `add`, `remove`, `set-url`, `get-url`, `-v`; needed before fetch/push |
| `git fetch [<remote>]` | Download objects/refs; go-git `Fetch` API available |
| `git pull [<remote>] [<branch>]` | `fetch` + `merge`; go-git `Pull` API available |
| `git push [<remote>] [<branch>]` | Upload refs; go-git `Push` API available |
| `git merge <branch>` | Join two branches; go-git `Merge` API available |
| `git stash` | Currently a stub — needs `push`, `pop`, `list`, `drop`, `apply` |
| `git tag` | Create/list/delete lightweight and annotated tags |

### History inspection

| Command | Notes |
|---------|-------|
| `git blame <file>` | Annotate each line with commit hash + author |
| `git reflog` | History of HEAD movements (go-git stores reflogs) |
| `git shortlog` | Summarised commit count per author |
| `git describe` | Human-readable name from the nearest tag |
| `git ls-files` | Show tracked / staged / untracked files from the index |
| `git ls-tree <rev> [<path>]` | List tree object at a revision |

### Modern porcelain alternatives

| Command | Notes |
|---------|-------|
| `git switch [-c] <branch>` | Branch switching (git 2.23+; replaces `checkout -b`) |
| `git restore [--staged] <path>` | File restore (git 2.23+; replaces `checkout <file>`) |

### Patch / history editing

| Command | Notes |
|---------|-------|
| `git cherry-pick <rev>` | Apply a single commit onto HEAD |
| `git revert <rev>` | Create a commit that inverts a previous one |
| `git rebase <branch>` | Reapply commits on top of another base |
| `git apply <patch>` | Apply a unified-diff patch file |
| `git format-patch <range>` | Write commits as `.patch` files |
| `git am <patch>` | Apply a mailbox of patches |

### Plumbing / object inspection

| Command | Notes |
|---------|-------|
| `git cat-file -t/-p <obj>` | Inspect type/content of any git object |
| `git hash-object [-w] <file>` | Compute (and optionally store) a blob hash |
| `git ls-remote <url>` | List refs on a remote without cloning |

## Known limitations

- **Symlinks**: `afero.MemMapFs` has no symlink support. `Lstat` falls back
  to `Stat`, `Symlink` is a no-op, and `Readlink` returns the input path
  unchanged. Repositories that use symlinks will silently skip them.
- **File modes**: `MemMapFs` does not persist execute bits; mode information
  stored in git objects may not round-trip exactly.
- **Authentication**: `git clone` and (future) `fetch`/`push` use go-git's
  default HTTP transport. Public HTTPS repos work; SSH and token auth are not
  yet wired up.
- **`cd` in shell**: `mvdan.cc/sh` intercepts `cd` before the exec handler,
  so `git -C <path>` is the recommended way to target a virtual repo path
  when the shell's real cwd differs from the virtual repo root.
