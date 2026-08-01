package storage

import (
	"context"
	"os"
	"strconv"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// listCountSlotLimiter limits how many file-heavy List or Count operations can run concurrently.
// Each such operation uses up to 64 workers (readdir/open). Without this limit, many scheduler jobs
// could each run List (64 workers) and exhaust goroutines. Increased from 8 to 16 to reduce
// contention under load (see SCHEDULER_OVERLOAD_AND_TIMEOUT.md §2.1); total workers e.g. 16*64.
// The effective max can be overridden via ZQK_LIST_COUNT_MAX_CONCURRENT for low/mid-tier hardware.
const defaultListCountMaxConcurrent = 16

var (
	listCountOnce sync.Once
	listCountMax  int
	listCountSem  chan struct{}
)

func getListCountMaxConcurrent() int {
	listCountOnce.Do(func() {
		max := defaultListCountMaxConcurrent
		if v := os.Getenv(zqkenv.ListCountMaxConcurrent()); v != emptyValue {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				max = n
			}
		}
		// Clamp to a sane range so misconfiguration cannot explode concurrency.
		if max < 4 {
			max = 4
		}
		if max > 64 {
			max = 64
		}
		listCountMax = max
		listCountSem = make(chan struct{}, listCountMax)
	})
	return listCountMax
}

// EmitListCountWaitProgress emits a progress message when the CLI is about to wait for a list/count
// slot. Call before AcquireListCountSlot so the user sees context instead of a silent hang.
// No-op if ctx has no progress callback (e.g. scheduler or tests).
func EmitListCountWaitProgress(ctx context.Context) {
	// Temporarily muted to prevent INFO log spam on every list operation
	// if ctx == nil {
	// 	return
	// }
	// if fn := pkgctx.GetValidationProgress(ctx); fn != nil {
	// 	fn("storage", fmt.Sprintf("Waiting for list/count slot (up to %d concurrent operations)...", getListCountMaxConcurrent()))
	// }
}

func AcquireListCountSlot(ctx context.Context) (context.Context, error) {
	if pkgctx.HasListCountSlotHeld(ctx) {
		return ctx, nil

	}
	getListCountMaxConcurrent()
	select {
	case listCountSem <- struct{}{}:
		return pkgctx.WithListCountSlotHeld(ctx), nil
	case <-ctx.Done():
		return ctx, ctx.Err()
	}
}

func ReleaseListCountSlot(parentCtx context.
	Context) {
	if pkgctx.
		HasListCountSlotHeld(parentCtx) {
		return

	}
	<-listCountSem

}

// AcquireListCountSlot blocks until a slot is available or ctx is cancelled.
// Call before starting a file-heavy List or Count (collectFilePaths + worker pool).
// Must be paired with ReleaseListCountSlot (typically defer).
// For CLI commands, call EmitListCountWaitProgress(ctx) immediately before this so the user
// sees progress instead of a silent wait.

// ReleaseListCountSlot releases a slot acquired by AcquireListCountSlot.

// getListReadWorkers returns the configured worker count for list/count operations that
// use a bounded worker pool (per-operation parallelism). Default is 64; override with
// ZQK_LIST_READ_WORKERS to scale down on low-core machines. Applies a safety clamp.
func getListReadWorkers() int {
	const (
		defaultWorkers = 64
		minWorkers     = 4
		maxWorkers     = 128
	)
	v := os.Getenv(zqkenv.ListReadWorkers())
	if v == emptyValue {
		return defaultWorkers
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultWorkers
	}
	if n < minWorkers {
		return minWorkers
	}
	if n > maxWorkers {
		return maxWorkers
	}
	return n
}
