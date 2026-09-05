package validation

import "time"

// DefaultValidationStateCacheMaxAge is how long a persisted validation result may be
// reused across CLI processes before Get treats it as expired. System check warm path
// depends on this spanning typical operator gaps; mtime/checksum still invalidate sooner.
const DefaultValidationStateCacheMaxAge = 24 * time.Hour

// AsyncValidatorConfig holds configurable timeouts and buffer size for the async validator.
// Used by NewAsyncValidator; defaults are documented so operators can tune for large repos.
// See CODEBASE_EVALUATION_AND_REMEDIAL_PLAN.md §5.2 and BLI configurable validation timeouts.
type AsyncValidatorConfig struct {
	// WorkerStopTimeout is how long to wait for workers to drain and stop (default: 60s).
	// Must be sufficient under load so shutdown completes reliably; increased from 30s to reduce "Worker stop timeout" on large runs.
	WorkerStopTimeout time.Duration
	// CacheSaveTimeout is base time for state cache flush on stop (default: 20s); lifecycle scales with state count.
	// Increased from 10s to reduce "Cache save timeout" when many objects are validated.
	CacheSaveTimeout time.Duration
	// ProgressChannelSize is the buffer size for the progress channel (default: 10000).
	// Must be large enough to absorb bursts when enqueueing many cached objects; 10000
	// handles 4k+ objects with headroom. Reduce for small repos to save memory.
	ProgressChannelSize int
}

// DefaultAsyncValidatorConfig returns the default async validator config.
// Prioritizes system reliability: worker stop and cache save must complete so validation state is consistent.
func DefaultAsyncValidatorConfig() *AsyncValidatorConfig {
	return &AsyncValidatorConfig{
		WorkerStopTimeout:   60 * time.Second, // Allow workers to drain under load (validation, I/O, locks)
		CacheSaveTimeout:    20 * time.Second, // Base; lifecycle scales with state count (see async_validator_lifecycle)
		ProgressChannelSize: 10000,
	}
}

// applyDefaults fills zero values with defaults so callers can pass partial config.
func (c *AsyncValidatorConfig) applyDefaults() {
	def := DefaultAsyncValidatorConfig()
	if c.WorkerStopTimeout <= 0 {
		c.WorkerStopTimeout = def.WorkerStopTimeout
	}
	if c.CacheSaveTimeout <= 0 {
		c.CacheSaveTimeout = def.CacheSaveTimeout
	}
	if c.ProgressChannelSize <= 0 {
		c.ProgressChannelSize = def.ProgressChannelSize
	}
}
