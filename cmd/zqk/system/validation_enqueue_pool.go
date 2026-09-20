package system

import (
	stdcontext "context"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Incremental validation after CUD is syscall-heavy (os.ReadFile per compose lookup).
// One StartSimple per object exhausted the CLI 512-thread cap during object promote
// (2026-09-13 dump: thousands of enqueue_validation_for_object Gs in syscall.Open).
const (
	incrementalValidationMaxWorkers = 8
	incrementalValidationQueue      = 128
)

var (
	incrementalValidationPoolOnce sync.Once
	incrementalValidationPoolInst *goroutinelabels.Pool
)

func incrementalValidationWorkerCount() int {
	n := incrementalValidationMaxWorkers
	if cfg := concurrency.GetGlobalConcurrencyConfig(); cfg != nil && cfg.ValidatorMaxWorkers > 0 && cfg.ValidatorMaxWorkers < n {
		n = cfg.ValidatorMaxWorkers
	}
	if n < 1 {
		return 1
	}
	return n
}

func incrementalValidationPool() *goroutinelabels.Pool {
	incrementalValidationPoolOnce.Do(func() {
		n := incrementalValidationWorkerCount()
		p := goroutinelabels.NewPool(
			goroutinelabels.DefaultBudget(),
			"incremental_validation",
			"incremental validation cache repopulation",
			n,
			incrementalValidationQueue,
		)
		p.Start(stdcontext.Background())
		incrementalValidationPoolInst = p
	})
	return incrementalValidationPoolInst
}
