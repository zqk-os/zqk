package storage

import (
	"context"
	"runtime"
	"strconv"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// listCountSlotLimiter limits how many file-heavy List or Count operations can run concurrently.
// Each operation uses getListReadWorkers() syscalls (open/read). 16 slots × 64 workers
// was 1024 concurrent opens — Darwin Ms parked after those bursts (2026-08-20 dump;
// 2026-09-02 zqk-stable sample ~2041 parked Ms). Defaults are 4 slots × ≤8 workers.
// TRACK: BLI-CAS-HAND-DUP-CHECK-001
// Override via ZQK_LIST_COUNT_MAX_CONCURRENT / ZQK_LIST_READ_WORKERS.
const defaultListCountMaxConcurrent = 4

var (
	listCountOnce sync.Once
	listCountMax  int
	listCountSem  chan struct{}
)

func getListCountMaxConcurrent() int {
	listCountOnce.Do(func() {
		max := defaultListCountMaxConcurrent
		if v := zqkenv.ListCountMaxConcurrent().Get(); v != emptyValue {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				max = n
			}
		}
		// Clamp to a sane range so misconfiguration cannot explode concurrency.
		if max < 4 {
			max = 4
		}
		if max > 16 {
			max = 16
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

func ReleaseListCountSlot(parentCtx context.Context) {
	if pkgctx.HasListCountSlotHeld(parentCtx) {
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
// Pass the parent context from before Acquire (not the slot-held ctx): if the parent
// already held a slot, Acquire was a no-op and Release must not pop the semaphore.

func defaultListReadWorkers() int {
	n := runtime.GOMAXPROCS(0)
	if n < 4 {
		return 4
	}
	if n > 8 {
		return 8
	}
	return n
}

// getListReadWorkers returns the configured worker count for list/count operations that
// use a bounded worker pool (per-operation parallelism). Default is GOMAXPROCS clamped
// to [4,16]; override with ZQK_LIST_READ_WORKERS. Applies a safety clamp.
func getListReadWorkers() int {
	const (
		minWorkers = 4
		maxWorkers = 16
	)
	def := defaultListReadWorkers()
	v := zqkenv.ListReadWorkers().Get()
	if v == emptyValue {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	if n < minWorkers {
		return minWorkers
	}
	if n > maxWorkers {
		return maxWorkers
	}
	return n
}
