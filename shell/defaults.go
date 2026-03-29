package shell

import (
	"embed"
	"path/filepath"
	"strings"
)

//go:embed plugins/*.wasm
var builtinPluginsFS embed.FS

// defaultPlugins is populated at init from the embedded plugins directory.
// Any .wasm file placed in shell/plugins/ is automatically registered.
var defaultPlugins map[string][]byte

func init() {
	defaultPlugins = make(map[string][]byte)
	entries, _ := builtinPluginsFS.ReadDir("plugins")
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".wasm" {
			continue
		}
		data, _ := builtinPluginsFS.ReadFile("plugins/" + e.Name())
		name := strings.TrimSuffix(e.Name(), ".wasm")
		defaultPlugins[name] = data
	}
}

// BuiltinPluginNames returns the names of all built-in WASM plugins embedded in the binary.
func BuiltinPluginNames() []string {
	names := make([]string, 0, len(defaultPlugins))
	for name := range defaultPlugins {
		names = append(names, name)
	}
	return names
}
