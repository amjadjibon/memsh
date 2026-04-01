package shell

import (
	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/amjadjibon/memsh/shell/plugins/native"
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
