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
}

var (
	globalConfig     *ConcurrencyConfig
	globalConfigOnce sync.Once
	globalConfigMu   sync.RWMutex
)

// initDefaultConfig computes smart defaults based on the current runtime.
func initDefaultConfig() *ConcurrencyConfig {
	// Default validator workers: clamped to [2, 2] for minimal memory footprint (<100MB RSS)
	validatorWorkers := 2

	// Default async router workers: NumCPU, clamped to [2, 16]
	routerWorkers := runtime.NumCPU()
	if routerWorkers < 2 {
		routerWorkers = 2
	}
	if routerWorkers > 16 {
		routerWorkers = 16
	}

	return &ConcurrencyConfig{
		ValidatorMaxWorkers:   validatorWorkers,
		AsyncRouterMaxWorkers: routerWorkers,
	}
}

// GetGlobalConcurrencyConfig returns the global concurrency configuration.
// Callers should treat the returned struct as read-only.
func GetGlobalConcurrencyConfig() *ConcurrencyConfig {
	globalConfigOnce.Do(func() {
		globalConfig = initDefaultConfig()
	})

	globalConfigMu.RLock()
	defer globalConfigMu.RUnlock()
	return globalConfig
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

	// Preserve existing values when caller leaves a field as zero.
	if cfg.ValidatorMaxWorkers > 0 {
		globalConfig.ValidatorMaxWorkers = cfg.ValidatorMaxWorkers
	}
	if cfg.AsyncRouterMaxWorkers > 0 {
		globalConfig.AsyncRouterMaxWorkers = cfg.AsyncRouterMaxWorkers
	}
}
