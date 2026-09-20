package cli

import (
	"testing"
	"time"
)

func TestParseSchedulerStopWaitArgs(t *testing.T) {
	t.Parallel()
	maxWait, wait := parseSchedulerStopWaitArgs([]string{"scheduler", "stop", "--wait", "--max-wait", "180s"})
	if !wait {
		t.Fatal("expected wait true")
	}
	if maxWait != 180*time.Second {
		t.Fatalf("maxWait: got %v want 180s", maxWait)
	}

	maxWait, wait = parseSchedulerStopWaitArgs([]string{"scheduler", "stop"})
	if wait {
		t.Fatal("expected wait false without --wait")
	}
	if maxWait != schedulerStopDefaultMaxWait {
		t.Fatalf("default maxWait: got %v", maxWait)
	}

	maxWait, wait = parseSchedulerStopWaitArgs([]string{"--profile", "x", "scheduler", "stop", "--wait", "--max-wait=2m"})
	if !wait || maxWait != 2*time.Minute {
		t.Fatalf("global flags: wait=%v maxWait=%v", wait, maxWait)
	}
}

func TestAdjustOuterTimeoutForSchedulerStopWait(t *testing.T) {
	t.Parallel()
	nc := "zqk-stable scheduler stop --wait --max-wait 180s"
	args := []string{"scheduler", "stop", "--wait", "--max-wait", "180s"}
	// wall = 180s + 5s + 20s = 205s; floor 20s does not apply
	want := 180*time.Second + schedulerStopCoordinatorTimeout + schedulerStopTailSlack
	got := adjustOuterTimeoutForSchedulerStopWait(nc, args, 30*time.Second)
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}

	// --wait without explicit --max-wait: argv parser uses schedulerStopDefaultMaxWait (matches cobra default)
	argsDefaultWait := []string{"scheduler", "stop", "--wait"}
	wantDefault := schedulerStopDefaultMaxWait + schedulerStopCoordinatorTimeout + schedulerStopTailSlack
	got = adjustOuterTimeoutForSchedulerStopWait("zqk scheduler stop --wait", argsDefaultWait, 30*time.Second)
	if got != wantDefault {
		t.Fatalf("default max-wait wall: got %v want %v", got, wantDefault)
	}

	// Non-wait: leave baseline
	got = adjustOuterTimeoutForSchedulerStopWait("zqk scheduler stop", []string{"scheduler", "stop"}, 30*time.Second)
	if got != 30*time.Second {
		t.Fatalf("non-wait: got %v want 30s", got)
	}

	// Already above wall: unchanged
	long := 10 * time.Minute
	got = adjustOuterTimeoutForSchedulerStopWait(nc, args, long)
	if got != long {
		t.Fatalf("preserve larger timeout: got %v", got)
	}
}

func TestSchedulerStopWaitExemptFromChildCap(t *testing.T) {
	t.Parallel()
	if !schedulerStopWaitExemptFromChildCap("bin scheduler stop --wait", []string{"scheduler", "stop", "--wait"}) {
		t.Fatal("expected exempt when --wait")
	}
	if schedulerStopWaitExemptFromChildCap("bin scheduler stop", []string{"scheduler", "stop"}) {
		t.Fatal("expected not exempt without --wait")
	}
}
