package shell

import _ "embed"

//go:embed plugins/base64.wasm
var base64PluginWasm []byte

// defaultPlugins contains built-in WASM plugins bundled with memsh.
var defaultPlugins = map[string][]byte{
	"base64": base64PluginWasm,
}
