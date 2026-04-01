package web

import _ "embed"

// TerminalHTML is the single-file terminal UI served at GET /.
//
//go:embed terminal.html
var TerminalHTML []byte
