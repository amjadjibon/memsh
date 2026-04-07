package shell

import (
	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/amjadjibon/memsh/shell/plugins/native"
	nativegit "github.com/amjadjibon/memsh/shell/plugins/native/git"
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
		native.Base64Plugin{},
		native.WcPlugin{},
		native.GrepPlugin{},
		native.FindPlugin{},
		native.AwkPlugin{},
		native.LuaPlugin{},
		native.GojaPlugin{},
		native.JqPlugin{},
		native.YqPlugin{},
		native.CurlPlugin{},
		native.MD5Sum(),
		native.SHA1Sum(),
		native.SHA224Sum(),
		native.SHA256Sum(),
		native.SHA384Sum(),
		native.SHA512Sum(),
		native.TputPlugin{},
		native.SttyPlugin{},
		native.MktempPlugin{},
		native.ColumnPlugin{},
		native.EnvsubstPlugin{},
		native.BcPlugin{},
		native.ExprPlugin{},
		native.XxdPlugin{},
		native.HexdumpPlugin{},
		native.TarPlugin{},
		native.Gzip(),
		native.Gunzip(),
		native.Zip(),
		native.Unzip(),
		nativegit.GitPlugin{},
		native.Less(),
		native.More(),
		native.SSHPlugin{},
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
