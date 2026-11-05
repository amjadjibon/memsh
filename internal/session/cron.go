package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/cron"
	"github.com/amjadjibon/memsh/pkg/network"
	"github.com/amjadjibon/memsh/pkg/shell"
)

// maxCronLogBytes bounds /.cron_log; older entries are dropped first.
const maxCronLogBytes = 1 << 20

// maxCronJobOutput caps the output captured from one cron job.
const maxCronJobOutput = 64 << 10

// cronEnv is what a cron run needs from the server.
type cronEnv struct {
	store    *Store // optional; when set, jobs charge the session's budgets
	baseOpts []shell.Option
	timeout  time.Duration
	limits   Limits
}

// StartScheduler fires cron jobs for every active session once per minute.
// It aligns to the next minute boundary before starting the ticker so that
// CronMatches is evaluated at a consistent wall-clock minute. ctx should be
// cancelled when the server shuts down. Jobs are subject to the same
// session limits (runtime, filesystem, network budgets) as /run requests.
func StartScheduler(ctx context.Context, store *Store, baseOpts []shell.Option, timeout time.Duration, limits Limits) {
	env := cronEnv{store: store, baseOpts: baseOpts, timeout: timeout, limits: limits}

	// Align to the next minute boundary.
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Until(next)):
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			for _, ss := range store.Snapshot() {
				go runSessionCronJobs(ctx, t, ss, env)
			}
		}
	}
}

// runSessionCronJobs reads /.crontab from ss.fs, parses it, and runs any jobs
// whose cron expression matches the given time t.
func runSessionCronJobs(ctx context.Context, t time.Time, ss Snap, env cronEnv) {
	data, err := afero.ReadFile(ss.Fs, cron.CrontabFile)
	if err != nil {
		// No crontab installed for this session — nothing to do.
		return
	}
	jobs, err := cron.ParseCrontab(string(data))
	if err != nil {
		log.Printf("cron: session %s: parse error: %v", ss.ID, err)
		return
	}
	for _, job := range jobs {
		if cron.CronMatches(job.Expr, t) {
			runCronJob(ctx, t, job.Command, ss, env)
		}
	}
}

// runCronJob runs a single cron job command inside the session's virtual FS and
// appends a timestamped log entry (including output) to /.cron_log.
func runCronJob(ctx context.Context, t time.Time, command string, ss Snap, env cronEnv) {
	// Hold ExecMu for the full job so cron cannot race concurrent /run handlers.
	if ss.ExecMu != nil {
		ss.ExecMu.Lock()
		defer ss.ExecMu.Unlock()
	}

	// Re-read the entry under ExecMu: the FS may have been replaced
	// (snapshot import) and the budgets advanced since the tick.
	baseFS, cwd := ss.Fs, ss.Cwd
	var runtime time.Duration
	var netUsage network.Usage
	if env.store != nil {
		e, ok := env.store.GetExisting(ss.ID)
		if !ok {
			return // session deleted
		}
		baseFS, cwd, runtime, netUsage = e.Fs, e.Cwd, e.Runtime, e.Network
	}

	if err := env.limits.ValidateRuntime(runtime); err != nil {
		log.Printf("cron: session %s: skipped: %v", ss.ID, err)
		return
	}
	timeout, err := env.limits.EffectiveTimeout(env.timeout, runtime, time.Second)
	if err != nil {
		log.Printf("cron: session %s: skipped: %v", ss.ID, err)
		return
	}
	fs, err := env.limits.WrapFS(baseFS)
	if err != nil {
		log.Printf("cron: session %s: measure fs: %v", ss.ID, err)
		return
	}

	out := NewLimitedBuffer(maxCronJobOutput)
	opts := make([]shell.Option, len(env.baseOpts), len(env.baseOpts)+4)
	copy(opts, env.baseOpts)
	opts = append(opts,
		shell.WithFS(fs),
		shell.WithCwd(cwd),
		shell.WithStdIO(strings.NewReader(""), out, out),
		shell.WithNetworkUsage(netUsage),
	)

	sh, err := shell.New(opts...)
	if err != nil {
		log.Printf("cron: session %s: shell init: %v", ss.ID, err)
		return
	}
	defer sh.Close()

	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startedAt := time.Now()
	runErr := sh.Run(jobCtx, command)
	if env.store != nil {
		env.store.AddUsage(ss.ID, time.Since(startedAt), sh.NetworkUsage())
	}

	// Build the log entry regardless of error so users can see when jobs ran.
	stamp := t.Format("2006-01-02 15:04")
	entry := fmt.Sprintf("[%s] %s\n%s\n", stamp, command, out.String())
	if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
		entry += fmt.Sprintf("# error: %v\n", runErr)
	}

	// Serialise log writes for this session.
	ss.CronMu.Lock()
	defer ss.CronMu.Unlock()

	existing, _ := afero.ReadFile(fs, cron.CronLogFile)
	updated := append(existing, []byte(entry)...)
	if len(updated) > maxCronLogBytes {
		updated = updated[len(updated)-maxCronLogBytes:]
		if i := strings.IndexByte(string(updated), '\n'); i >= 0 {
			updated = updated[i+1:] // start at a line boundary
		}
	}
	if writeErr := afero.WriteFile(fs, cron.CronLogFile, updated, 0o644); writeErr != nil {
		log.Printf("cron: session %s: write log: %v", ss.ID, writeErr)
	}
}
