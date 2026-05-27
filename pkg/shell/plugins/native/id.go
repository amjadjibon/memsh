package native

import (
	"context"
	"fmt"
	"os/user"
	"strconv"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// IdPlugin implements the `id` command.
// It reports the current user's UID, GID, and groups, preferring values from
// the virtual shell environment (UID, GID, USER, GROUPS) and falling back to
// the real OS user.
type IdPlugin struct{}

func (IdPlugin) Name() string        { return "id" }
func (IdPlugin) Description() string { return "print real and effective user/group IDs" }
func (IdPlugin) Usage() string       { return "id [-u] [-g] [-G] [-n] [-r]" }

func (IdPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	showUID := false
	showGID := false
	showGroups := false
	showName := false

	for _, a := range args[1:] {
		switch a {
		case "-u", "--user":
			showUID = true
		case "-g", "--group":
			showGID = true
		case "-G", "--groups":
			showGroups = true
		case "-n", "--name":
			showName = true
		case "-r", "--real":
			// no-op: we only track real IDs
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(hc.Stderr, "id: invalid option -- %q\n", a)
				return interp.ExitStatus(1)
			}
		}
	}

	// Resolve identity — prefer virtual env vars, fall back to OS.
	uid, gid, username, groupName := resolveIdentity(sc)

	// Single-field modes.
	switch {
	case showUID:
		if showName {
			fmt.Fprintln(hc.Stdout, username)
		} else {
			fmt.Fprintln(hc.Stdout, uid)
		}
		return nil
	case showGID:
		if showName {
			fmt.Fprintln(hc.Stdout, groupName)
		} else {
			fmt.Fprintln(hc.Stdout, gid)
		}
		return nil
	case showGroups:
		if showName {
			fmt.Fprintln(hc.Stdout, groupName)
		} else {
			fmt.Fprintln(hc.Stdout, gid)
		}
		return nil
	}

	// Default: uid=N(name) gid=N(name) groups=N(name)
	fmt.Fprintf(hc.Stdout, "uid=%s(%s) gid=%s(%s) groups=%s(%s)\n",
		uid, username, gid, groupName, gid, groupName)
	return nil
}

// resolveIdentity returns uid, gid, username, groupName from the virtual shell
// environment, falling back to the real OS user when env vars are absent.
func resolveIdentity(sc plugins.ShellContext) (uid, gid, username, groupName string) {
	uid = sc.Env("UID")
	gid = sc.Env("GID")
	username = sc.Env("USER")
	if username == "" {
		username = sc.Env("LOGNAME")
	}

	// Fall back to real OS user.
	if uid == "" || username == "" {
		if u, err := user.Current(); err == nil {
			if uid == "" {
				uid = u.Uid
			}
			if gid == "" {
				gid = u.Gid
			}
			if username == "" {
				username = u.Username
			}
		}
	}

	// Derive numeric defaults when still empty.
	if uid == "" {
		uid = "0"
	}
	if gid == "" {
		gid = "0"
	}
	if username == "" {
		username = "unknown"
	}

	// Resolve group name: try OS lookup, else reuse username or numeric gid.
	groupName = sc.Env("GROUP")
	if groupName == "" {
		if gidInt, err := strconv.Atoi(gid); err == nil {
			if g, err := user.LookupGroupId(strconv.Itoa(gidInt)); err == nil {
				groupName = g.Name
			}
		}
	}
	if groupName == "" {
		groupName = username
	}

	return uid, gid, username, groupName
}

var _ plugins.PluginInfo = IdPlugin{}
