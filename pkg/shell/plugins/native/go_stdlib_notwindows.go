//go:build !windows

package native

// Full stdlib bindings including ext (archive, compress, crypto, syslog…).
// Excluded on Windows because stdlib/ext/log_syslog.go uses log/syslog which
// does not exist on Windows.
import _ "github.com/mvm-sh/mvm/stdlib/all"
