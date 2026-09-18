package scheduler

import (
	"testing"
)

func TestDetachedDaemonStartArgs_monolithicIncludesSchedulerSubcommand(t *testing.T) {
	t.Parallel()
	got := detachedDaemonStartArgs("/usr/local/bin/zqk")
	want := []string{schedulerArgScheduler, schedulerArgStart, schedulerArgForeground}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d]=%q want %q (full %#v)", i, got[i], want[i], got)
		}
	}
}

func TestDetachedDaemonStartArgs_standaloneOmitsSchedulerSubcommand(t *testing.T) {
	t.Parallel()
	got := detachedDaemonStartArgs("/usr/local/bin/zqk-scheduler")
	want := []string{schedulerArgStart, schedulerArgForeground}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d]=%q want %q (full %#v)", i, got[i], want[i], got)
		}
	}
}
