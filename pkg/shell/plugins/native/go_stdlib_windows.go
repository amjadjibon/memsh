//go:build windows

package native

// Core stdlib bindings only — stdlib/ext is excluded because it contains
// log_syslog.go which uses log/syslog, unavailable on Windows.
import _ "github.com/mvm-sh/mvm/stdlib/core"
