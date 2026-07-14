package cron_test

import (
	"strings"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/pkg/cron"
)

func TestParseCronExprMatchesExpectedMinute(t *testing.T) {
	sched, err := cron.ParseCronExpr("30 9 * * *")
	if err != nil {
		t.Fatalf("ParseCronExpr returned error: %v", err)
	}

	match := time.Date(2026, 7, 15, 9, 30, 42, 0, time.UTC)
	if !cron.CronMatches(sched, match) {
		t.Fatal("CronMatches returned false at scheduled minute")
	}
	nonMatch := time.Date(2026, 7, 15, 9, 31, 0, 0, time.UTC)
	if cron.CronMatches(sched, nonMatch) {
		t.Fatal("CronMatches returned true outside scheduled minute")
	}
}

func TestParseCronExprNormalizesSundaySeven(t *testing.T) {
	sched, err := cron.ParseCronExpr("0 12 * * 7")
	if err != nil {
		t.Fatalf("ParseCronExpr returned error for Sunday 7: %v", err)
	}
	sunday := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	if !cron.CronMatches(sched, sunday) {
		t.Fatal("CronMatches returned false for Sunday normalized from 7")
	}
}

func TestParseCronExprAcceptsDescriptors(t *testing.T) {
	sched, err := cron.ParseCronExpr("@hourly")
	if err != nil {
		t.Fatalf("ParseCronExpr returned error for descriptor: %v", err)
	}
	if !cron.CronMatches(sched, time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)) {
		t.Fatal("@hourly did not match the top of the hour")
	}
}

func TestParseCrontabParsesJobsAndSkipsComments(t *testing.T) {
	jobs, err := cron.ParseCrontab(`
# comment

*/5 * * * * echo tick
@daily /backup.sh --full
`)
	if err != nil {
		t.Fatalf("ParseCrontab returned error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	if jobs[0].Command != "echo tick" {
		t.Fatalf("first command = %q, want echo tick", jobs[0].Command)
	}
	if jobs[0].Raw != "*/5 * * * * echo tick" {
		t.Fatalf("first raw = %q", jobs[0].Raw)
	}
	if jobs[1].Command != "/backup.sh --full" {
		t.Fatalf("second command = %q, want backup command", jobs[1].Command)
	}
}

func TestParseCrontabReportsHelpfulErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "descriptor without command",
			data: "@daily",
			want: "@special keyword requires a command",
		},
		{
			name: "too few standard fields",
			data: "* * * echo",
			want: "need 5 cron fields + command",
		},
		{
			name: "invalid expression",
			data: "bad bad bad bad bad echo",
			want: "failed to parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cron.ParseCrontab(tt.data)
			if err == nil {
				t.Fatal("ParseCrontab returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}
