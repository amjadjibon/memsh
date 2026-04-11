package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/pkg/cron"
	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

// ── ParseCronExpr ─────────────────────────────────────────────────────────────

func TestParseCronExpr(t *testing.T) {
	cases := []struct {
		expr   string
		wantOK bool
	}{
		{"* * * * *", true},
		{"0 0 1 1 *", true},
		{"*/5 * * * *", true},
		{"1-5 * * * *", true},
		{"1,2,3 * * * *", true},
		{"0-59/10 * * * *", true},
		{"0 0-23/2 * * *", true},
		{"0 0 1-15 1,6,12 1-5", true},
	}
	for _, tc := range cases {
		_, err := cron.ParseCronExpr(tc.expr)
		if tc.wantOK && err != nil {
			t.Errorf("ParseCronExpr(%q) unexpected error: %v", tc.expr, err)
		}
		if !tc.wantOK && err == nil {
			t.Errorf("ParseCronExpr(%q) expected error, got nil", tc.expr)
		}
	}
}

func TestParseCronExprInvalid(t *testing.T) {
	cases := []string{
		"60 * * * *",  // minute out of range
		"* 24 * * *",  // hour out of range
		"* * 0 * *",   // dom out of range (min is 1)
		"* * * 13 *",  // month out of range
		"* * * * *",   // valid — sanity; not in this list
		"* * * *",     // too few fields
		"* * * * * *", // too many fields
		"*/0 * * * *", // step of zero
		"abc * * * *", // non-numeric
	}
	// Remove the valid one to avoid a false failure.
	for i, s := range cases {
		if s == "* * * * *" {
			cases = append(cases[:i], cases[i+1:]...)
			break
		}
	}
	for _, expr := range cases {
		_, err := cron.ParseCronExpr(expr)
		if err == nil {
			t.Errorf("ParseCronExpr(%q): expected error, got nil", expr)
		}
	}
}

// ── CronMatches ───────────────────────────────────────────────────────────────

func mustParse(t *testing.T, expr string) cron.CronExpr {
	t.Helper()
	e, err := cron.ParseCronExpr(expr)
	if err != nil {
		t.Fatalf("ParseCronExpr(%q): %v", expr, err)
	}
	return e
}

func TestCronMatchesBasic(t *testing.T) {
	// Exact minute and hour match.
	expr := mustParse(t, "30 14 * * *")
	match := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
	noMatch := time.Date(2024, 3, 15, 14, 31, 0, 0, time.UTC)

	if !cron.CronMatches(expr, match) {
		t.Errorf("expected match at %v", match)
	}
	if cron.CronMatches(expr, noMatch) {
		t.Errorf("expected no match at %v", noMatch)
	}
}

func TestCronMatchesWildcard(t *testing.T) {
	expr := mustParse(t, "* * * * *")
	for _, ts := range []time.Time{
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC),
		time.Date(2024, 12, 31, 23, 59, 0, 0, time.UTC),
	} {
		if !cron.CronMatches(expr, ts) {
			t.Errorf("* * * * * should match %v", ts)
		}
	}
}

func TestCronMatchesStep(t *testing.T) {
	expr := mustParse(t, "*/5 * * * *")
	for _, min := range []int{0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55} {
		ts := time.Date(2024, 1, 1, 0, min, 0, 0, time.UTC)
		if !cron.CronMatches(expr, ts) {
			t.Errorf("*/5 should match minute %d", min)
		}
	}
	for _, min := range []int{1, 3, 7, 11} {
		ts := time.Date(2024, 1, 1, 0, min, 0, 0, time.UTC)
		if cron.CronMatches(expr, ts) {
			t.Errorf("*/5 should NOT match minute %d", min)
		}
	}
}

func TestCronMatchesSundayAlias(t *testing.T) {
	// dow=7 should be treated the same as dow=0 (Sunday).
	expr7 := mustParse(t, "0 0 * * 7")
	expr0 := mustParse(t, "0 0 * * 0")
	// 2024-01-07 is a Sunday.
	sunday := time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC)
	monday := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)

	if !cron.CronMatches(expr7, sunday) {
		t.Error("dow=7 should match Sunday")
	}
	if !cron.CronMatches(expr0, sunday) {
		t.Error("dow=0 should match Sunday")
	}
	if cron.CronMatches(expr7, monday) {
		t.Error("dow=7 should NOT match Monday")
	}
}

func TestCronSpecialKeywords(t *testing.T) {
	cases := []struct {
		keyword string
		ts      time.Time
		want    bool
	}{
		{"@daily", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC), true},
		{"@daily", time.Date(2024, 3, 15, 1, 0, 0, 0, time.UTC), false},
		{"@midnight", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC), true},
		{"@hourly", time.Date(2024, 3, 15, 5, 0, 0, 0, time.UTC), true},
		{"@hourly", time.Date(2024, 3, 15, 5, 1, 0, 0, time.UTC), false},
		{"@weekly", time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC), true},  // Sunday
		{"@weekly", time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), false}, // Monday
		{"@monthly", time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), true},
		{"@monthly", time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC), false},
		{"@yearly", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"@annually", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"@yearly", time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range cases {
		expr := mustParse(t, tc.keyword)
		got := cron.CronMatches(expr, tc.ts)
		if got != tc.want {
			t.Errorf("CronMatches(%q, %v) = %v, want %v", tc.keyword, tc.ts, got, tc.want)
		}
	}
}

// ── ParseCrontab ──────────────────────────────────────────────────────────────

func TestParseCrontab(t *testing.T) {
	input := `
# This is a comment

* * * * * echo hello

# another comment
0 0 * * * /bin/backup.sh
@daily /bin/daily.sh
*/15 * * * * /bin/quarter-hourly.sh
`
	jobs, err := cron.ParseCrontab(input)
	if err != nil {
		t.Fatalf("ParseCrontab: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("expected 4 jobs, got %d", len(jobs))
	}
	if jobs[0].Command != "echo hello" {
		t.Errorf("job[0].Command = %q, want %q", jobs[0].Command, "echo hello")
	}
	if jobs[1].Command != "/bin/backup.sh" {
		t.Errorf("job[1].Command = %q, want %q", jobs[1].Command, "/bin/backup.sh")
	}
	if jobs[2].Command != "/bin/daily.sh" {
		t.Errorf("job[2].Command = %q, want %q", jobs[2].Command, "/bin/daily.sh")
	}
}

// ── CrontabPlugin ─────────────────────────────────────────────────────────────

func TestCrontabPlugin(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()

	// Install via stdin pipe.
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `echo "* * * * * echo hi" | crontab`); err != nil {
		t.Fatalf("pipe to crontab: %v", err)
	}

	// Verify it was saved.
	buf.Reset()
	s2 := NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s2.Run(ctx, `crontab -l`); err != nil {
		t.Fatalf("crontab -l: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, "echo hi") {
		t.Errorf("crontab -l output %q should contain 'echo hi'", out)
	}
}

func TestCrontabPluginList(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	// Fresh FS with no crontab.
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `crontab -l`); err != nil {
		t.Fatalf("crontab -l: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "# no crontab for user" {
		t.Errorf("expected '# no crontab for user', got %q", out)
	}
}

func TestCrontabPluginRemove(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()

	// Install.
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `echo "0 * * * * echo hourly" | crontab`); err != nil {
		t.Fatalf("install crontab: %v", err)
	}

	// Remove.
	buf.Reset()
	s2 := NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s2.Run(ctx, `crontab -r`); err != nil {
		t.Fatalf("crontab -r: %v", err)
	}

	// Confirm gone.
	buf.Reset()
	s3 := NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s3.Run(ctx, `crontab -l`); err != nil {
		t.Fatalf("crontab -l after remove: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "# no crontab for user" {
		t.Errorf("expected '# no crontab for user' after remove, got %q", out)
	}
}
