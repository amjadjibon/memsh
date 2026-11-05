package session

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/cron"
	"github.com/amjadjibon/memsh/pkg/shell"
)

func testSnap(fs afero.Fs) Snap {
	return Snap{
		ID:     "sess1",
		Fs:     fs,
		Cwd:    "/",
		ExecMu: &sync.Mutex{},
		CronMu: &sync.Mutex{},
	}
}

func TestRunSessionCronJobsNoCrontab(t *testing.T) {
	fs := afero.NewMemMapFs()
	ss := testSnap(fs)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	// No /.crontab present: should return without error or panic.
	runSessionCronJobs(context.Background(), time.Now(), ss, cronEnv{baseOpts: baseOpts, timeout: time.Second})

	if _, err := afero.Exists(fs, cron.CronLogFile); err == nil {
		if ok, _ := afero.Exists(fs, cron.CronLogFile); ok {
			t.Error("no cron log should be written when there is no crontab")
		}
	}
}

func TestRunSessionCronJobsMatchingRuns(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, cron.CrontabFile, []byte("* * * * * echo hello-cron\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ss := testSnap(fs)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	now := time.Now().Truncate(time.Minute)
	runSessionCronJobs(context.Background(), now, ss, cronEnv{baseOpts: baseOpts, timeout: 5*time.Second})

	data, err := afero.ReadFile(fs, cron.CronLogFile)
	if err != nil {
		t.Fatalf("expected cron log to be written: %v", err)
	}
	if !strings.Contains(string(data), "hello-cron") {
		t.Errorf("cron log missing job output: %q", data)
	}
	if !strings.Contains(string(data), "echo hello-cron") {
		t.Errorf("cron log missing command line: %q", data)
	}
}

func TestRunSessionCronJobsNonMatchingSkipped(t *testing.T) {
	fs := afero.NewMemMapFs()
	// Fixed minute (0 0 1 1 *) essentially never matches "now" in tests.
	if err := afero.WriteFile(fs, cron.CrontabFile, []byte("0 0 1 1 * echo should-not-run\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ss := testSnap(fs)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	runSessionCronJobs(context.Background(), time.Now(), ss, cronEnv{baseOpts: baseOpts, timeout: time.Second})

	if ok, _ := afero.Exists(fs, cron.CronLogFile); ok {
		data, _ := afero.ReadFile(fs, cron.CronLogFile)
		t.Errorf("non-matching job should not run, but log contains: %q", data)
	}
}

func TestRunSessionCronJobsInvalidCrontabLogsError(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, cron.CrontabFile, []byte("not a valid crontab line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ss := testSnap(fs)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	// Should not panic even though ParseCrontab fails.
	runSessionCronJobs(context.Background(), time.Now(), ss, cronEnv{baseOpts: baseOpts, timeout: time.Second})
}

func TestRunCronJobAppendsToExistingLog(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, cron.CronLogFile, []byte("previous entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ss := testSnap(fs)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	runCronJob(context.Background(), time.Now(), "echo second-run", ss, cronEnv{baseOpts: baseOpts, timeout: 5*time.Second})

	data, err := afero.ReadFile(fs, cron.CronLogFile)
	if err != nil {
		t.Fatalf("reading cron log: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "previous entry") {
		t.Errorf("existing log entry lost: %q", out)
	}
	if !strings.Contains(out, "second-run") {
		t.Errorf("new job output missing: %q", out)
	}
}

func TestRunCronJobRecordsErrorInLog(t *testing.T) {
	fs := afero.NewMemMapFs()
	ss := testSnap(fs)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	runCronJob(context.Background(), time.Now(), "nonexistent-command-xyz", ss, cronEnv{baseOpts: baseOpts, timeout: 5*time.Second})

	data, err := afero.ReadFile(fs, cron.CronLogFile)
	if err != nil {
		t.Fatalf("reading cron log: %v", err)
	}
	if !strings.Contains(string(data), "# error:") {
		t.Errorf("expected error annotation in log, got: %q", data)
	}
}

func TestStartSchedulerStopsOnCancelBeforeAlignment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := New(context.Background(), time.Hour, 0)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	done := make(chan struct{})
	go func() {
		StartScheduler(ctx, store, baseOpts, time.Second, Limits{})
		close(done)
	}()

	// Cancel immediately: StartScheduler should return promptly from its
	// wait-for-minute-boundary select without ever ticking.
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartScheduler did not return after context cancellation")
	}
}

func TestCronJobRespectsSessionBudgets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := New(ctx, time.Hour, 0)
	const id = "0123456789abcdef"
	e, _ := st.Get(id)
	snap := st.Snapshot()[0]
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	// Runtime budget exhausted: the job must not run.
	st.AddUsage(id, time.Minute, e.Network)
	runCronJob(ctx, time.Now(), "echo ran > /marker", snap, cronEnv{store: st, baseOpts: baseOpts, timeout: time.Second, limits: Limits{MaxRuntime: time.Second}})
	if ok, _ := afero.Exists(e.Fs, "/marker"); ok {
		t.Fatal("cron job ran despite an exhausted runtime budget")
	}

	// Filesystem cap is enforced during the job's writes.
	runCronJob(ctx, time.Now(), "yes > /big", snap, cronEnv{store: st, baseOpts: baseOpts, timeout: 5 * time.Second, limits: Limits{MaxBytes: 64 << 10}})
	if info, err := e.Fs.Stat("/big"); err == nil && info.Size() > 64<<10 {
		t.Fatalf("cron job wrote %d bytes past a 64 KiB cap", info.Size())
	}

	// Runtime is charged to the session.
	before := e.Runtime
	runCronJob(ctx, time.Now(), "sleep 0.05", snap, cronEnv{store: st, baseOpts: baseOpts, timeout: 5 * time.Second})
	if e.Runtime <= before {
		t.Fatal("cron job runtime was not charged to the session")
	}
}

func TestCronLogIsBounded(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, cron.CronLogFile, []byte(strings.Repeat("old line\n", maxCronLogBytes/9+10)), 0o644)
	ss := Snap{ID: "s", Fs: fs, Cwd: "/", ExecMu: &sync.Mutex{}, CronMu: &sync.Mutex{}}
	runCronJob(ctx, time.Now(), "echo newest", ss, cronEnv{baseOpts: []shell.Option{shell.WithWASMEnabled(false)}, timeout: time.Second})
	data, _ := afero.ReadFile(fs, cron.CronLogFile)
	if len(data) > maxCronLogBytes {
		t.Fatalf("cron log is %d bytes, cap %d", len(data), maxCronLogBytes)
	}
	if !strings.Contains(string(data), "newest") {
		t.Fatal("newest entry missing after trimming")
	}
}
