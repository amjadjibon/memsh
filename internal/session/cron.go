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
	"github.com/amjadjibon/memsh/pkg/shell"
)

// StartScheduler fires cron jobs for every active session once per minute.
// It aligns to the next minute boundary before starting the ticker so that
// CronMatches is evaluated at a consistent wall-clock minute. ctx should be
// cancelled when the server shuts down.
func StartScheduler(ctx context.Context, store *Store, baseOpts []shell.Option, timeout time.Duration) {
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
				go runSessionCronJobs(ctx, t, ss, baseOpts, timeout)
			}
		}
	}
}

// runSessionCronJobs reads /.crontab from ss.fs, parses it, and runs any jobs
// whose cron expression matches the given time t.
func runSessionCronJobs(ctx context.Context, t time.Time, ss Snap, baseOpts []shell.Option, timeout time.Duration) {
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
			runCronJob(ctx, t, job.Command, ss, baseOpts, timeout)
		}
	}
}

// runCronJob runs a single cron job command inside the session's virtual FS and
// appends a timestamped log entry (including output) to /.cron_log.
func runCronJob(ctx context.Context, t time.Time, command string, ss Snap, baseOpts []shell.Option, timeout time.Duration) {
	var out strings.Builder

	opts := make([]shell.Option, len(baseOpts)+3)
	copy(opts, baseOpts)
	opts[len(baseOpts)] = shell.WithFS(ss.Fs)
	opts[len(baseOpts)+1] = shell.WithCwd(ss.Cwd)
	opts[len(baseOpts)+2] = shell.WithStdIO(strings.NewReader(""), &out, &out)

	sh, err := shell.New(opts...)
	if err != nil {
		log.Printf("cron: session %s: shell init: %v", ss.ID, err)
		return
	}
	defer sh.Close()

	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	runErr := sh.Run(jobCtx, command)

	// Build the log entry regardless of error so users can see when jobs ran.
	stamp := t.Format("2006-01-02 15:04")
	entry := fmt.Sprintf("[%s] %s\n%s\n", stamp, command, out.String())
	if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
		entry += fmt.Sprintf("# error: %v\n", runErr)
	}

	// Serialise log writes for this session.
	ss.CronMu.Lock()
	defer ss.CronMu.Unlock()

	existing, _ := afero.ReadFile(ss.Fs, cron.CronLogFile)
	updated := append(existing, []byte(entry)...)
	if writeErr := afero.WriteFile(ss.Fs, cron.CronLogFile, updated, 0644); writeErr != nil {
		log.Printf("cron: session %s: write log: %v", ss.ID, writeErr)
	}
}
