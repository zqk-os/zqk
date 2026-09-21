package concurrency

import (
	"runtime"
	"sync"
)

// ConcurrencyConfig defines global concurrency defaults for the system.
// These values provide "smart defaults" that can be overridden by callers
// (CLI, tests, or higher-level components) when needed.
type ConcurrencyConfig struct {
	// ValidatorMaxWorkers controls the default maximum number of async
	// validation workers when a caller does not specify an explicit value.
	ValidatorMaxWorkers int

	// AsyncRouterMaxWorkers controls the default maximum number of async
	// router workers (scheduler transceiver) when a caller does not specify
	// an explicit value.
	AsyncRouterMaxWorkers int

	// SchedulerMaxConcurrentTestJobs bounds how many run_wrapper (go test) jobs the scheduler
	// dispatches at once, across all packages.
	//
	// Until this existed the scheduler had no global cap: ConflictManager blocks only a repeated
	// job ID and, for non-run_wrapper types, one job per type, so test concurrency scaled with the
	// number of distinct packages in flight. Nothing in that product referred to the host, and each
	// job spawns a `go test` that itself fans out to GOMAXPROCS for t.Parallel tests. A scan across
	// many packages therefore drove a 10-core machine to a load average of 15.
	SchedulerMaxConcurrentTestJobs int

	// SchedulerTestJobParallelism bounds each test job's own fan-out (go test -parallel), so the
	// worst case is SchedulerMaxConcurrentTestJobs * SchedulerTestJobParallelism threads and is
	// predictable. Left unset, go test defaults -parallel to GOMAXPROCS, which makes the total
	// depend on a value neither the scheduler nor the bundle knows.
	SchedulerTestJobParallelism int
}

var (
	globalConfig     *ConcurrencyConfig
	globalConfigOnce sync.Once
	globalConfigMu   sync.RWMutex
)

// initDefaultConfig computes smart defaults based on the current runtime.
func initDefaultConfig() *ConcurrencyConfig {
	// Default validator workers: NumCPU*2, clamped to [4, 16].
	// The previous [4, 8] cap left 10–12 core laptops at ~23 objects/s (~6 min for 8k).
	// Nested validate still holds the semaphore until done (no timeout thread leak).
	// Hostload refuses extra slots under foreign CPU pressure, not self-heat.
	// TRACK: follow-up in kernel backlog
	const maxValidatorWorkers = 16
	validatorWorkers := runtime.NumCPU() * 2
	if validatorWorkers < 4 {
		validatorWorkers = 4
	}
	if validatorWorkers > maxValidatorWorkers {
		validatorWorkers = maxValidatorWorkers
	}

	// Default async router workers: NumCPU, clamped to [2, 16]
	routerWorkers := runtime.NumCPU()
	if routerWorkers < 2 {
		routerWorkers = 2
	}
	if routerWorkers > 16 {
		routerWorkers = 16
	}

	return &ConcurrencyConfig{
		ValidatorMaxWorkers:            validatorWorkers,
		AsyncRouterMaxWorkers:          routerWorkers,
		SchedulerMaxConcurrentTestJobs: defaultSchedulerMaxConcurrentTestJobs(),
		SchedulerTestJobParallelism:    defaultSchedulerTestJobParallelism,
	}
}

// Each test job may use this many parallel test goroutines. Two rather than one because this repo's
// tests are heavily I/O bound (CAS writes, file locks), so a second goroutine overlaps waiting rather
// than stacking CPU work; the pair still keeps the worst case bounded and predictable.
const defaultSchedulerTestJobParallelism = 2

// defaultSchedulerMaxConcurrentTestJobs gives tests half the logical CPUs, less one slot reserved for
// the daemon's own bookkeeping and whatever the operator is running in the foreground. On a 10-core
// host that is 4, leaving the other half free rather than saturating the machine during a scan.
//
// That static budget is the ceiling. Live host CPU pressure (AV, other tenants) further scales it
// via pkg/hostload so ZQK does not pile on when the machine is already starved.
// TRACK: TDE-CEF-HOST-CPU-BACKPRESSURE-001
//
// Clamped to [1, 8]: a single-core host must still make progress, and beyond 8 the limit stops being
// the binding constraint — disk and the CAS index lock are, as a --all scan showed by timing out
// waiting 30s for that lock.
func defaultSchedulerMaxConcurrentTestJobs() int {
	const (
		minJobs = 1
		maxJobs = 8
	)
	n := runtime.NumCPU()/2 - 1
	if n < minJobs {
		return minJobs
	}
	if n > maxJobs {
		return maxJobs
	}
	return n
}

// GetGlobalConcurrencyConfig returns the global concurrency configuration.
// Callers should treat the returned struct as read-only.
func GetGlobalConcurrencyConfig() *ConcurrencyConfig {
	globalConfigOnce.Do(func() {
		globalConfig = initDefaultConfig()
	})

	globalConfigMu.RLock()
	defer globalConfigMu.RUnlock()
	cp := *globalConfig
	return &cp
}

// SetGlobalConcurrencyConfig overrides the global concurrency configuration.
// This is primarily intended for CLI wiring and tests. Callers may provide a
// partial config (zero values will preserve existing defaults).
func SetGlobalConcurrencyConfig(cfg *ConcurrencyConfig) {
	if cfg == nil {
		return
	}

	globalConfigOnce.Do(func() {
		globalConfig = initDefaultConfig()
	})

	globalConfigMu.Lock()
	defer globalConfigMu.Unlock()

	newCfg := *globalConfig
	// Preserve existing values when caller leaves a field as zero.
	if cfg.ValidatorMaxWorkers > 0 {
		newCfg.ValidatorMaxWorkers = cfg.ValidatorMaxWorkers
	}
	if cfg.AsyncRouterMaxWorkers > 0 {
		newCfg.AsyncRouterMaxWorkers = cfg.AsyncRouterMaxWorkers
	}
	globalConfig = &newCfg
}
