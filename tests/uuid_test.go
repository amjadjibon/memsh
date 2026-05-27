package tests

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestUUID(t *testing.T) {
	ctx := context.Background()

	t.Run("generates a valid UUID v4", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "uuid"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !uuidRE.MatchString(out) {
			t.Errorf("invalid UUID format: %q", out)
		}
	})

	t.Run("-n generates multiple UUIDs", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "uuid -n 5"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 5 {
			t.Errorf("expected 5 UUIDs, got %d", len(lines))
		}
		for _, l := range lines {
			if !uuidRE.MatchString(strings.TrimSpace(l)) {
				t.Errorf("invalid UUID format: %q", l)
			}
		}
	})

	t.Run("-v 7 generates UUID v7", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "uuid -v 7"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !uuidRE.MatchString(out) {
			t.Errorf("invalid UUID v7 format: %q", out)
		}
		// UUID v7: version nibble must be '7'
		version := string(out[14])
		if version != "7" {
			t.Errorf("expected version '7', got %q in UUID %q", version, out)
		}
	})

	t.Run("-u outputs uppercase", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "uuid -u"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != strings.ToUpper(out) {
			t.Errorf("expected uppercase UUID, got %q", out)
		}
	})

	t.Run("each uuid is unique", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "uuid -n 10"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		seen := make(map[string]bool)
		for _, l := range lines {
			if seen[l] {
				t.Errorf("duplicate UUID: %q", l)
			}
			seen[l] = true
		}
	})

	t.Run("invalid version returns error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "uuid -v 3"); err == nil {
			t.Error("expected error for unsupported version")
		}
	})
}
