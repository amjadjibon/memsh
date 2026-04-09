package native

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/amjadjibon/memsh/pkg/cron"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// CrontabPlugin implements the `crontab` command against the virtual FS.
//
//	crontab -l         print /.crontab, or a "no crontab" message if absent
//	crontab -r         remove /.crontab silently
//	crontab <file>     validate and install <file> as /.crontab
//	crontab            read stdin, validate, and install as /.crontab
type CrontabPlugin struct{}

func (CrontabPlugin) Name() string        { return "crontab" }
func (CrontabPlugin) Description() string { return "manage the virtual crontab (/.crontab)" }
func (CrontabPlugin) Usage() string       { return "crontab [-l|-r|<file>]" }

func (CrontabPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	// Default: no args — read from stdin.
	if len(args) < 2 {
		data, err := io.ReadAll(hc.Stdin)
		if err != nil {
			return fmt.Errorf("crontab: read stdin: %w", err)
		}
		return installCrontab(sc.FS, string(data))
	}

	switch args[1] {
	case "-l":
		return listCrontab(hc.Stdout, sc.FS)
	case "-r":
		return removeCrontab(sc.FS)
	default:
		if strings.HasPrefix(args[1], "-") {
			return fmt.Errorf("crontab: invalid option -- %q", args[1])
		}
		// Treat as a file path in the virtual FS.
		path := sc.ResolvePath(args[1])
		data, err := afero.ReadFile(sc.FS, path)
		if err != nil {
			return fmt.Errorf("crontab: %s: %w", args[1], err)
		}
		return installCrontab(sc.FS, string(data))
	}
}

// listCrontab prints the contents of /.crontab, or an informational message
// when no crontab has been installed yet.
func listCrontab(w io.Writer, fs afero.Fs) error {
	data, err := afero.ReadFile(fs, cron.CrontabFile)
	if err != nil {
		_, _ = fmt.Fprintln(w, "# no crontab for user")
		return nil
	}
	_, err = fmt.Fprint(w, string(data))
	return err
}

// removeCrontab deletes /.crontab from the virtual FS (no-op if absent).
func removeCrontab(fs afero.Fs) error {
	exists, err := afero.Exists(fs, cron.CrontabFile)
	if err != nil {
		return fmt.Errorf("crontab: %w", err)
	}
	if !exists {
		return nil
	}
	return fs.Remove(cron.CrontabFile)
}

// installCrontab validates data as a crontab file and writes it to /.crontab.
func installCrontab(fs afero.Fs, data string) error {
	if _, err := cron.ParseCrontab(data); err != nil {
		return fmt.Errorf("crontab: %w", err)
	}
	return afero.WriteFile(fs, cron.CrontabFile, []byte(data), 0o644)
}

// compile-time interface check.
var _ plugins.PluginInfo = CrontabPlugin{}
