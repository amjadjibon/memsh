package git

import (
	"fmt"
	"io"

	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git config
// ---------------------------------------------------------------------------

func cmdGitConfig(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("git config: %w", err)
	}

	// Minimal: git config user.name / user.email get/set
	if len(args) == 1 {
		// Get.
		switch args[0] {
		case "user.name":
			fmt.Fprintln(w, cfg.User.Name)
		case "user.email":
			fmt.Fprintln(w, cfg.User.Email)
		default:
			fmt.Fprintf(errW, "git config: key '%s' not found\n", args[0])
			return interp.ExitStatus(1)
		}
		return nil
	}
	if len(args) == 2 {
		// Set.
		switch args[0] {
		case "user.name":
			cfg.User.Name = args[1]
		case "user.email":
			cfg.User.Email = args[1]
		default:
			fmt.Fprintf(errW, "git config: unsupported key '%s'\n", args[0])
			return interp.ExitStatus(1)
		}
		return repo.Storer.SetConfig(cfg)
	}

	// List all.
	fmt.Fprintf(w, "user.name=%s\n", cfg.User.Name)
	fmt.Fprintf(w, "user.email=%s\n", cfg.User.Email)
	return nil
}
