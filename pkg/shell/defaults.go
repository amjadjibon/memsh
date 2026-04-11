package shell

import (
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/amjadjibon/memsh/pkg/shell/plugins/native"
	nativegit "github.com/amjadjibon/memsh/pkg/shell/plugins/native/git"
)

// defaultPlugins holds WASM plugins bundled at compile time.
// Currently empty — base64 and wc are native Go plugins in shell/plugins/native/.
// To bundle a .wasm file: add it to shell/plugins/ and restore:
//
//	//go:embed plugins/*.wasm
//	var builtinPluginsFS embed.FS
var defaultPlugins = map[string][]byte{}

// defaultNativePlugins returns the built-in native Plugin implementations
// registered on every new Shell unless overridden by a WithPlugin option.
func defaultNativePlugins() []plugins.Plugin {
	return []plugins.Plugin{
		// filesystem
		native.CdPlugin{},
		native.PwdPlugin{},
		native.LsPlugin{},
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
		native.DuPlugin{},
		native.DfPlugin{},
		native.MktempPlugin{},

		// text processing
		native.CatPlugin{},
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

		// data tools
		native.JqPlugin{},
		native.YqPlugin{},
		native.Base64Plugin{},
		native.XxdPlugin{},
		native.HexdumpPlugin{},
		native.BcPlugin{},
		native.ExprPlugin{},

		// scripting
		native.LuaPlugin{},
		native.GojaPlugin{},
		native.SQLitePlugin{},

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
		native.SSHPlugin{},

		// version control
		nativegit.GitPlugin{},

		// environment
		native.EnvPlugin{},
		native.PrintenvPlugin{},
		native.EnvsubstPlugin{},

		// shell / session
		native.ReadPlugin{},
		native.SourcePlugin{},
		native.DotPlugin{},
		native.SeqPlugin{},
		native.DatePlugin{},
		native.SleepPlugin{},
		native.YesPlugin{},
		native.TimeoutPlugin{},
		native.CrontabPlugin{},

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
