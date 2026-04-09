// Package cron provides cron expression parsing, matching, and crontab file
// parsing for the memsh virtual shell scheduler, backed by robfig/cron/v3.
package cron

import (
	"fmt"
	"strings"
	"time"

	robfig "github.com/robfig/cron/v3"
)

// CrontabFile is the virtual-FS path where the user's crontab is stored.
const CrontabFile = "/.crontab"

// CronLogFile is the virtual-FS path where cron job output is appended.
const CronLogFile = "/.cron_log"

// CronExpr is a parsed cron schedule.
// It is a type alias for robfig.Schedule so callers that already hold a
// CronExpr can pass it directly to CronMatches without any conversion.
type CronExpr = robfig.Schedule

// CronJob is one entry parsed from a crontab file.
type CronJob struct {
	Expr    CronExpr
	Command string
	Raw     string
}

// parser is a shared, thread-safe expression parser.
// It supports the standard 5-field form and @special descriptors
// (@yearly, @annually, @monthly, @weekly, @daily, @midnight, @hourly).
var parser = robfig.NewParser(
	robfig.Minute | robfig.Hour | robfig.Dom | robfig.Month | robfig.Dow | robfig.Descriptor,
)

// ParseCronExpr parses a cron expression string into a CronExpr (Schedule).
// Both 5-field ("m h dom mon dow") and @special forms are accepted.
// The day-of-week value 7 (Sunday alias used by many cron implementations) is
// normalised to 0 before passing to the parser, since robfig/cron/v3 only
// accepts 0–6 in that field.
func ParseCronExpr(s string) (CronExpr, error) {
	return parser.Parse(normalizeDow(strings.TrimSpace(s)))
}

// normalizeDow replaces standalone "7" entries in the dow field with "0"
// (both mean Sunday; robfig only accepts 0–6).
func normalizeDow(expr string) string {
	if strings.HasPrefix(expr, "@") {
		return expr // @special — no field splitting needed
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return expr
	}
	// Replace each comma-separated token in the dow field.
	parts := strings.Split(fields[4], ",")
	for i, p := range parts {
		if p == "7" {
			parts[i] = "0"
		}
	}
	fields[4] = strings.Join(parts, ",")
	return strings.Join(fields, " ")
}

// CronMatches reports whether sched fires at the given time (minute granularity).
//
// robfig's Schedule.Next(t) returns the next fire time strictly after t, so we
// check whether the first fire time after (m − 1ns) is exactly m.
func CronMatches(sched CronExpr, t time.Time) bool {
	m := t.Truncate(time.Minute)
	return sched.Next(m.Add(-time.Nanosecond)).Equal(m)
}

// ParseCrontab parses the content of a crontab file into a slice of CronJobs.
// Blank lines and lines beginning with '#' are ignored.
// Each non-comment line must be either:
//
//	@special command...          (e.g. "@daily /backup.sh")
//	m h dom mon dow command...   (standard 5-field form)
func ParseCrontab(data string) ([]CronJob, error) {
	var jobs []CronJob
	for rawLine := range strings.SplitSeq(data, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		var exprStr, command string

		if strings.HasPrefix(fields[0], "@") {
			if len(fields) < 2 {
				return nil, fmt.Errorf("cron: %q: @special keyword requires a command", line)
			}
			exprStr = fields[0]
			command = strings.Join(fields[1:], " ")
		} else {
			if len(fields) < 6 {
				return nil, fmt.Errorf("cron: %q: need 5 cron fields + command, got %d fields", line, len(fields))
			}
			exprStr = strings.Join(fields[:5], " ")
			command = strings.Join(fields[5:], " ")
		}

		sched, err := ParseCronExpr(exprStr)
		if err != nil {
			return nil, fmt.Errorf("cron: %q: %w", line, err)
		}
		jobs = append(jobs, CronJob{Expr: sched, Command: command, Raw: line})
	}
	return jobs, nil
}
