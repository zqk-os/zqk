package app

import (
	"runtime/debug"
	"strconv"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Fail-closed OS thread cap. Go mints an M per G blocked in a syscall; those Ms
// park in pthread_cond_wait and never shrink. Sample 2026-09-02: ~2041 threads.
// TRACK: BLI-CEF-STORAGE-INDEX-CACHE-001
const (
	defaultMaxOSThreads = 512
	minMaxOSThreads     = 64
	maxMaxOSThreads     = 1024
	// CLI process-wide labeled-goroutine cap. Restored to 512 (matching defaultMaxOSThreads
	// and defaultSchedulerGoroutineCap) so baseline daemons (~35) and 99-kind discovery
	// have generous headroom and never starve control-plane or data-plane pipelines.
	defaultCLIGoroutineBudget = 512
)

func maxOSThreads() int {
	n := 0
	if raw := zqkenv.MaxOSThreads().Get(); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			n = v
		}
	}
	if n == 0 {
		n = config.SystemMaxOSThreads().OrDefault(defaultMaxOSThreads)
	}
	if n <= 0 {
		n = defaultMaxOSThreads
	}
	if n < minMaxOSThreads {
		n = minMaxOSThreads
	}
	if n > maxMaxOSThreads {
		n = maxMaxOSThreads
	}
	return n
}

func applyMaxOSThreads() {
	debug.SetMaxThreads(maxOSThreads())
	applyDefaultCLIGoroutineBudget()
}

// applyDefaultCLIGoroutineBudget installs a process DefaultBudget when none is set.
// Scheduler daemons already set one; CLI object promote did not, so EnqueueValidation
// and other optional-budget StartSimple paths were unbounded.
// TRACK: BLI-CEF-STORAGE-INDEX-CACHE-001
func applyDefaultCLIGoroutineBudget() {
	if goroutinelabels.DefaultBudget() != nil {
		return
	}
	n := defaultCLIGoroutineBudget
	if threads := maxOSThreads(); threads > n {
		n = threads
	}
	goroutinelabels.SetDefaultBudget(goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: n}))
}
