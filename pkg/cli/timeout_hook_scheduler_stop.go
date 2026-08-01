package cli

import (
	"strings"
	"time"
)

// Outer CLI timeout alignment for "scheduler stop --wait".
//
// cmd/zqk/scheduler/scheduler_core.go stopScheduler extends the inner control timeout to
// maxWait + coordinator + tail slack when --wait is set. config/command_timeouts.yaml may
// still apply a shorter default for "scheduler stop"; these helpers lift the outer hook to
// cover the full inner poll so the CLI is not killed mid-wait.
//
// These helpers mirror stopScheduler's wall clock so the hook does not fire early.

const (
	schedulerStopCoordinatorTimeout = 5 * time.Second  // schedulerCoordinatorTimeout in scheduler_core.go
	schedulerStopTailSlack          = 20 * time.Second // stopWaitTailSlack in scheduler_core.go
	schedulerStopControlFloor       = 20 * time.Second // schedulerControlTimeout in scheduler_core.go
	// Must match pkg/scheduler.StopSchedulerShutdownWait and `max-wait` default in cmd/zqk/scheduler/scheduler_commands.go
	// (pkg/cli cannot import pkg/scheduler: import cycle).
	schedulerStopDefaultMaxWait = 3 * time.Minute
)

// parseSchedulerStopWaitArgs scans argv (os.Args[1:]) for "scheduler stop" and parses --wait / --max-wait.
func parseSchedulerStopWaitArgs(args []string) (maxWait time.Duration, wait bool) {
	maxWait = schedulerStopDefaultMaxWait
	stopIdx := -1
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "scheduler" && args[i+1] == "stop" {
			stopIdx = i + 2
			break
		}
	}
	if stopIdx < 0 {
		return 0, false
	}
	for i := stopIdx; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--wait" || a == "-w":
			wait = true
		case strings.HasPrefix(a, "--wait="):
			wait = true
		case a == "--max-wait" && i+1 < len(args):
			if d, err := time.ParseDuration(args[i+1]); err == nil {
				maxWait = d
			}
			i++
		case strings.HasPrefix(a, "--max-wait="):
			rest := strings.TrimPrefix(a, "--max-wait=")
			if d, err := time.ParseDuration(rest); err == nil {
				maxWait = d
			}
		}
	}
	return maxWait, wait
}

// adjustOuterTimeoutForSchedulerStopWait raises the hook timeout when --wait is used so it covers
// inner polling (see stopScheduler runSchedulerControlWithTimeoutDur wall).
func adjustOuterTimeoutForSchedulerStopWait(normalizedCmd string, args []string, timeout time.Duration) time.Duration {
	if !strings.Contains(normalizedCmd, "scheduler stop") {
		return timeout
	}
	maxWait, wait := parseSchedulerStopWaitArgs(args)
	if !wait {
		return timeout
	}
	if maxWait <= 0 {
		maxWait = schedulerStopDefaultMaxWait
	}
	wall := maxWait + schedulerStopCoordinatorTimeout + schedulerStopTailSlack
	if wall < schedulerStopControlFloor {
		wall = schedulerStopControlFloor
	}
	if timeout < wall {
		return wall
	}
	return timeout
}

// schedulerStopWaitExemptFromChildCap returns true when "scheduler stop --wait" should not be capped
// to 2m for non-zqk parents (IDE wrappers); inner --max-wait can exceed that cap.
func schedulerStopWaitExemptFromChildCap(normalizedCmd string, args []string) bool {
	if !strings.Contains(normalizedCmd, "scheduler stop") {
		return false
	}
	_, wait := parseSchedulerStopWaitArgs(args)
	return wait
}
