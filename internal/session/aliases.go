package session

import (
	"context"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

// AliasesFile is the virtual-FS path used to persist aliases across requests.
const AliasesFile = "/.memsh_session_aliases"

// SaveAliases writes the current alias table to AliasesFile in the virtual FS
// so the next shell can restore it via source.
func SaveAliases(ctx context.Context, sh *shell.Shell) {
	_ = sh.Run(ctx, "alias > "+AliasesFile)
}

// RestoreAliases sources AliasesFile if it exists.
func RestoreAliases(ctx context.Context, sh *shell.Shell, fs afero.Fs) {
	if ok, _ := afero.Exists(fs, AliasesFile); ok {
		_ = sh.Run(ctx, "source "+AliasesFile)
	}
}
