package shell

import (
	"sort"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/amjadjibon/memsh/pkg/shell/plugins/native"
	nativegit "github.com/amjadjibon/memsh/pkg/shell/plugins/native/git"
)

// defaultNativePlugins returns the built-in native Plugin implementations
// registered on every new Shell unless overridden by a WithPlugin option.
func defaultNativePlugins() []plugins.Plugin {
	return []plugins.Plugin{
		// filesystem
		native.CdPlugin{},
		native.PwdPlugin{},
		native.LsPlugin{},
		native.BasenamePlugin{},
		native.DirnamePlugin{},
		native.RealpathPlugin{},
		native.MkdirPlugin{},
		native.RmPlugin{},
		native.RmdirPlugin{},
		native.TouchPlugin{},
		native.CpPlugin{},
		native.MvPlugin{},
		native.LnPlugin{},
		native.ChmodPlugin{},
		native.StatPlugin{},
		native.DiffPlugin{},
		native.FindPlugin{},
		native.TreePlugin{},
		native.DuPlugin{},
		native.DfPlugin{},
		native.MktempPlugin{},

		// text processing
		native.CatPlugin{},
		native.TacPlugin{},
		native.HeadPlugin{},
		native.TailPlugin{},
		native.TeePlugin{},
		native.EchoPlugin{},
		native.PrintfPlugin{},
		native.SortPlugin{},
		native.UniqPlugin{},
		native.CutPlugin{},
		native.TrPlugin{},
		native.SedPlugin{},
		native.WcPlugin{},
		native.GrepPlugin{},
		native.AwkPlugin{},
		native.ColumnPlugin{},
		native.XargsPlugin{},
		native.RevPlugin{},
		native.PastePlugin{},
		native.NlPlugin{},
		native.FoldPlugin{},
		native.ShufPlugin{},

		// data tools
		native.JqPlugin{},
		native.UUIDPlugin{},
		native.Base32Plugin{},
		native.StringsPlugin{},
		native.YqPlugin{},
		native.Base64Plugin{},
		native.XxdPlugin{},
		native.HexdumpPlugin{},
		native.BcPlugin{},
		native.ExprPlugin{},

		// scripting
		// Note: lua, go and sqlite3 are NOT here — they embed interpreters that
		// reach the host OS. See gatedInterpreterPlugins.
		native.GojaPlugin{},

		// archive
		native.TarPlugin{},
		native.Gzip(),
		native.Gunzip(),
		native.Zip(),
		native.Unzip(),

		// checksums
		native.MD5Sum(),
		native.SHA1Sum(),
		native.SHA224Sum(),
		native.SHA256Sum(),
		native.SHA384Sum(),
		native.SHA512Sum(),

		// network
		native.CurlPlugin{},
		native.NcPlugin{},
		native.SSHPlugin{},
		native.LsofPlugin{},

		// version control
		nativegit.GitPlugin{},

		// environment
		native.EnvPlugin{},
		native.PrintenvPlugin{},
		native.EnvsubstPlugin{},

		// shell / session
		native.TruePlugin{},
		native.FalsePlugin{},
		native.WatchPlugin{},
		native.TimePlugin{},
		native.ReadPlugin{},
		native.SourcePlugin{},
		native.DotPlugin{},
		native.SeqPlugin{},
		native.DatePlugin{},
		native.SleepPlugin{},
		native.YesPlugin{},
		native.TimeoutPlugin{},
		native.CrontabPlugin{},

		// user info
		native.WhoamiPlugin{},
		native.IdPlugin{},
		native.HostnamePlugin{},
		native.UnamePlugin{},

		// terminal / help
		native.ClearPlugin{},
		native.ResetPlugin{},
		native.Less(),
		native.More(),
		native.TputPlugin{},
		native.SttyPlugin{},
		native.HelpPlugin{},
		native.ManPlugin{},
		native.WhichPlugin{},
		native.ExitPlugin{},
		native.QuitPlugin{},
	}
}

// BuiltinPluginNames returns the names of built-in commands available without
// external plugins (native Go implementations registered at startup).
func BuiltinPluginNames() []string {
	plugins := defaultNativePlugins()
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name()
	}
	return names
}

// gatedInterpreterPlugins returns interpreter-backed plugins that are NOT
// registered by default because they carry host authority that memsh cannot
// take away from them:
//
//   - lua     gopher-lua opens the full Lua stdlib; os.execute runs real
//     commands and io.open reads/writes the real filesystem.
//   - go      mvm binds the entire Go stdlib, including os/exec and syscall,
//     so interpreted code can spawn real processes.
//   - sqlite3 operates on a real OS temp file, so ATTACH DATABASE (and
//     VACUUM INTO) read and write arbitrary real paths.
//
// Unlike the WASM plugin path — where a module can only reach what wazero
// imports into it — these run in-process and are not confined by the virtual
// filesystem. Enable them with WithUnsafeInterpreters(true) only when the
// script being run is as trusted as the host process itself.
func gatedInterpreterPlugins() []plugins.Plugin {
	return []plugins.Plugin{
		native.LuaPlugin{},
		native.GoPlugin{},
		native.SQLitePlugin{},
	}
}

// GatedInterpreterNames returns the command names withheld unless
// WithUnsafeInterpreters(true) is set.
func GatedInterpreterNames() []string {
	gated := gatedInterpreterPlugins()
	names := make([]string, len(gated))
	for i, p := range gated {
		names[i] = p.Name()
	}
	sort.Strings(names)
	return names
}

// isGatedInterpreter reports whether name is a gated interpreter command.
func isGatedInterpreter(name string) bool {
	for _, p := range gatedInterpreterPlugins() {
		if p.Name() == name {
			return true
		}
	}
	return false
}
