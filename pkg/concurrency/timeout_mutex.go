package concurrency

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const (
	lockOperationDefault             = "default"
	lockLogFieldOperation            = "operation"
	lockLogFieldWaitTime             = "wait_time"
	lockLogFieldHoldTime             = "hold_time"
	lockLogFieldTimeout              = "timeout"
	lockLogFieldError                = "error"
	lockWaitLogThreshold             = 10 * time.Millisecond
	lockHoldLogThreshold             = 100 * time.Millisecond
	lockTimeoutContentionHigh        = 0.3
	lockTimeoutContentionModerate    = 0.1
	lockTimeoutContentionLowModerate = 0.05
	lockTimeoutRateVeryHigh          = 0.2
	lockTimeoutRateHigh              = 0.1
	lockTimeoutRateModerate          = 0.05
	lockTimeoutMarginNumerator       = 110
	lockTimeoutMarginDenominator     = 100
	lockOverhead5ms                  = 5 * time.Millisecond
	lockOverhead10ms                 = 10 * time.Millisecond
	lockOverhead20ms                 = 20 * time.Millisecond
	lockOverhead50ms                 = 50 * time.Millisecond
	lockOverhead100ms                = 100 * time.Millisecond
	lockOverhead200ms                = 200 * time.Millisecond
	lockOverhead300ms                = 300 * time.Millisecond
	lockMinTimeout                   = 10 * time.Millisecond
	lockMaxTimeoutDefault            = 30 * time.Second
	lockMaxTimeoutSpecBase           = 60 * time.Second
	lockMaxTimeoutSpecHighContention = 90 * time.Second
	lockMaxTimeoutSpecExtreme        = 120 * time.Second
)

// LockMetrics provides optional metrics for lock operations
// If nil, default timeouts are used
// ValidationMetrics implements this interface for validation-specific metrics
type LockMetrics interface {
	// GetObjectsPerSecond returns the current processing rate
	GetObjectsPerSecond() float64
	// RecordLockWait records lock wait time
	RecordLockWait(duration time.Duration)
	// RecordLockHold records lock hold time
	RecordLockHold(duration time.Duration)
	// GetContentionRate returns the current contention rate (0.0 to 1.0)
	// Returns 0.0 if not available or not implemented
	GetContentionRate() float64
	// GetTimeoutRate returns the current timeout rate (0.0 to 1.0)
	// Returns 0.0 if not available or not implemented
	GetTimeoutRate() float64
	// GetMaxWaitTime returns the maximum wait time observed
	// Returns 0 if not available or not implemented
	GetMaxWaitTime() time.Duration
}

// LockLogger provides optional logging for lock operations
// If nil, no logging is performed
// This interface is defined here to avoid import cycle with pkg/logging
type LockLogger interface {
	Debug(msg string, fields ...LockField)
	Warn(msg string, fields ...LockField)
}

// LockField represents a structured log field (minimal interface to avoid import cycle)
type LockField struct {
	Key   string
	Value any
}

// calculateLockTimeout calculates timeout based on work size and baseline rate from metrics
// Now uses contention and timeout rates to dynamically adjust timeouts
// If metrics is nil, uses conservative defaults
func calculateLockTimeout(metrics LockMetrics, workSize int, operation string) time.Duration {
	// Get baseline rate from metrics
	var objectsPerSecond float64
	var contentionRate float64
	var timeoutRate float64
	var maxWaitTime time.Duration

	if metrics != nil {
		objectsPerSecond = metrics.GetObjectsPerSecond()
		contentionRate = metrics.GetContentionRate()
		timeoutRate = metrics.GetTimeoutRate()
		maxWaitTime = metrics.GetMaxWaitTime()
	}

	// If no baseline yet, use conservative estimate
	if objectsPerSecond == 0 {
		objectsPerSecond = 100.0 // Conservative: 100 objects/sec
	}

	// Calculate expected time for work
	// For lock operations, workSize is typically 1 (single operation)
	// But we account for potential contention
	expectedTime := time.Duration(float64(workSize) / objectsPerSecond * float64(time.Second))

	// Add 10% margin + operation-specific overhead
	overhead := map[string]time.Duration{
		"cache_get":     lockOverhead10ms,
		"cache_set":     lockOverhead20ms,
		"cache_save":    lockOverhead100ms,
		"cache_getall":  lockOverhead50ms,
		"hash_registry": lockOverhead50ms,
		// Hash registry cache get/set: under load many goroutines contend; need headroom so we don't fail and create new registries
		"hash_registry_cache_get": lockOverhead300ms,
		"hash_registry_cache_set": lockOverhead300ms,
		"progress_update":         lockOverhead5ms,
		"validator_start":         lockOverhead100ms,
		"validator_stop":          lockOverhead200ms,
		"validator_read":          lockOverhead10ms,
		"queue_dequeue":           lockOverhead5ms,
		// Spec loader operations (higher overhead due to file I/O and inheritance resolution)
		"spec_loader_check_cache":                  lockOverhead50ms,
		"spec_loader_cache_after_load":             lockOverhead100ms,
		"spec_loader_get_ontology_check_cache":     lockOverhead50ms,
		"spec_loader_get_ontology_cache":           lockOverhead100ms,
		"spec_loader_invalidate":                   lockOverhead50ms,
		"spec_loader_invalidate_parent":            lockOverhead50ms,
		"spec_loader_resolve_invalidate":           lockOverhead50ms,
		"spec_loader_resolve_invalidate_auditable": lockOverhead50ms,
		"spec_loader_clear_cache_shard":            lockOverhead200ms,
		"spec_loader_set_builder_registry":         lockOverhead100ms,
		"spec_loader_get_registry":                 lockOverhead100ms,
		"spec_loader_resolve_get_registry":         lockOverhead100ms,
		"callback_processor_initialize":            lockOverhead100ms,
		"callback_processor_shutdown":              lockOverhead300ms,
		"callback_queue_stop_processing":           lockOverhead300ms,
		"callback_processor_enqueue":               lockOverhead50ms,
		"callback_processor_process_direct":        2 * time.Second,
		"concurrent_test":                          500 * time.Millisecond,
		lockOperationDefault:                       lockOverhead50ms,
	}

	opOverhead, ok := overhead[operation]
	if !ok {
		opOverhead = overhead[lockOperationDefault]
	}

	timeout := expectedTime*lockTimeoutMarginNumerator/lockTimeoutMarginDenominator + opOverhead

	// DYNAMIC ADJUSTMENT: Increase timeout based on contention and timeout rates
	// If contention is high, we need longer timeouts to avoid premature failures
	if contentionRate > lockTimeoutContentionHigh {
		// High contention (>30%) - increase timeout by 3x
		timeout *= 3
	} else if contentionRate > lockTimeoutContentionModerate {
		// Moderate contention (>10%) - increase timeout by 2x
		timeout *= 2
	} else if contentionRate > lockTimeoutContentionLowModerate {
		// Low-moderate contention (>5%) - increase timeout by 1.5x
		timeout = timeout * 3 / 2
	}

	// If timeout rate is high, timeouts are too short - increase them
	if timeoutRate > lockTimeoutRateVeryHigh {
		// Very high timeout rate (>20%) - increase timeout by 4x
		timeout *= 4
	} else if timeoutRate > lockTimeoutRateHigh {
		// High timeout rate (>10%) - increase timeout by 2x
		timeout *= 2
	} else if timeoutRate > lockTimeoutRateModerate {
		// Moderate timeout rate (>5%) - increase timeout by 1.5x
		timeout = timeout * 3 / 2
	}

	// Use max wait time as a baseline if it's significantly higher than calculated timeout
	// This ensures we don't set timeouts shorter than what we've observed
	if maxWaitTime > timeout*2 {
		// Max wait time is much higher - use it as baseline with 50% margin
		timeout = maxWaitTime * 3 / 2
	}

	// Clamp to reasonable bounds
	minTimeout := lockMinTimeout
	// Increase max timeout for spec loading operations (file I/O and inheritance resolution can be slow)
	// Under heavy contention (e.g., 88k+ files, multiple goroutines loading specs), 60s may be insufficient
	// Use dynamic max based on contention
	maxTimeout := lockMaxTimeoutDefault
	if strings.HasPrefix(operation, "spec_loader") {
		maxTimeout = lockMaxTimeoutSpecBase // Base max for spec loading operations
		// Increase max timeout further if contention is high
		if contentionRate > lockTimeoutContentionHigh {
			maxTimeout = lockMaxTimeoutSpecExtreme // 2 minutes for extreme contention
		} else if contentionRate > lockTimeoutContentionModerate {
			maxTimeout = lockMaxTimeoutSpecHighContention // 90 seconds for high contention
		}
	}

	if timeout < minTimeout {
		timeout = minTimeout
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	return timeout
}

type tryLocker interface {
	TryLock() bool
}

type tryRLocker interface {
	TryRLock() bool
}

// acquireLockWithContext attempts to acquire mu within the lifetime of ctx.
// If mu supports TryLock (e.g. *sync.Mutex or *sync.RWMutex), it polls with an adaptive backoff
// without ever blocking indefinitely on mu.Lock().
func acquireLockWithContext(ctx context.Context, mu sync.Locker) error {
	if tl, ok := mu.(tryLocker); ok {
		if tl.TryLock() {
			return nil
		}
		backoff := 10 * time.Microsecond
		maxBackoff := 250 * time.Microsecond
		timer := time.NewTimer(backoff)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				if tl.TryLock() {
					return nil
				}
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				timer.Reset(backoff)
			}
		}
	}

	// Custom Locker without TryLock cannot honor ctx without a helper
	// goroutine that can leak mu.Lock() after cancel. Refuse.
	// TRACK: BLI-CEF-R2-CON-LOCK-TIMEOUT — require TryLock for timeout waits
	return errfmt.Errorf("locker %T does not implement TryLock; refusing helper goroutine that can leak Lock after cancel", mu)
}

// acquireRLockWithContext attempts to acquire mu's read lock within the lifetime of ctx.
func acquireRLockWithContext(ctx context.Context, mu *sync.RWMutex) error {
	if mu.TryRLock() {
		return nil
	}
	backoff := 10 * time.Microsecond
	maxBackoff := 250 * time.Microsecond
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			if mu.TryRLock() {
				return nil
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			timer.Reset(backoff)
		}
	}
}

// WithLockTimeout executes a function with a lock and timeout monitoring
// This is the recommended pattern: wrap lock-protected operations with timeout
// MUST be used for all mutex operations in production code
//
// CRITICAL: Lock must be acquired and released in the same goroutine.
// This function acquires the lock in the current goroutine to ensure proper unlock.
//
// Uses GoroutineBuilder pattern for timeout detection (approved pattern)
// Tracks lock wait and hold times via optional metrics
// Provides deterministic timeout to prevent deadlocks
func WithLockTimeout(
	mu sync.Locker,
	ctx context.Context,
	metrics LockMetrics,
	logger LockLogger,
	operation string,
	fn func() error,
) error {
	timeout := calculateLockTimeout(metrics, 1, operation)

	// Create context with timeout
	lockCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Track lock acquisition time
	acquireStart := time.Now()

	// Acquire lock with context cancellation / timeout support
	if err := acquireLockWithContext(lockCtx, mu); err != nil {
		waitTime := time.Since(acquireStart)
		if metrics != nil {
			metrics.RecordLockWait(waitTime)
		}
		opErr := errfmt.Errorf("operation %s exceeded timeout %v while waiting for lock: %w", operation, timeout, err)
		if logger != nil {
			logger.Warn("Operation timeout while waiting for lock",
				LockField{Key: lockLogFieldOperation, Value: operation},
				LockField{Key: lockLogFieldTimeout, Value: timeout.String()},
				LockField{Key: lockLogFieldError, Value: opErr.Error()})
		}
		return opErr
	}

	// Record wait time (time to acquire lock)
	waitTime := time.Since(acquireStart)
	if waitTime > lockWaitLogThreshold {
		if logger != nil {
			logger.Debug("Lock acquired after wait",
				LockField{Key: lockLogFieldOperation, Value: operation},
				LockField{Key: lockLogFieldWaitTime, Value: waitTime.String()})
		}
		if metrics != nil {
			metrics.RecordLockWait(waitTime)
		}
	}

	// Execute function with lock held, monitor hold time
	holdStart := time.Now()
	defer func() {
		holdTime := time.Since(holdStart)
		if holdTime > lockHoldLogThreshold {
			if logger != nil {
				logger.Debug("Lock held for extended period",
					LockField{Key: lockLogFieldOperation, Value: operation},
					LockField{Key: lockLogFieldHoldTime, Value: holdTime.String()})
			}
			if metrics != nil {
				metrics.RecordLockHold(holdTime)
			}
		}

		// If the operation took longer than the timeout, log a warning
		if holdTime > timeout {
			if logger != nil {
				logger.Warn("Operation exceeded timeout while holding lock (synchronous execution)",
					LockField{Key: lockLogFieldOperation, Value: operation},
					LockField{Key: lockLogFieldTimeout, Value: timeout.String()},
					LockField{Key: lockLogFieldHoldTime, Value: holdTime.String()})
			}
		}

		mu.Unlock()
	}()

	// Execute synchronously to maintain mutual exclusion
	var opErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				opErr = errfmt.Errorf("panic in lock operation %s: %v", operation, r)
			}
		}()
		opErr = fn()
	}()

	return opErr
}

// WithRLockTimeout executes a function with a read lock and timeout monitoring
// MUST be used for all RWMutex read lock operations in production code
//
// Uses GoroutineBuilder pattern for all goroutines (approved pattern)
// Tracks lock wait and hold times via optional metrics
// Provides deterministic timeout to prevent deadlocks
func WithRLockTimeout(
	mu *sync.RWMutex,
	ctx context.Context,
	metrics LockMetrics,
	logger LockLogger,
	operation string,
	fn func() error,
) error {
	timeout := calculateLockTimeout(metrics, 1, operation)

	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	acquireStart := time.Now()

	// Acquire read lock with context cancellation / timeout support
	if err := acquireRLockWithContext(opCtx, mu); err != nil {
		waitTime := time.Since(acquireStart)
		if metrics != nil {
			metrics.RecordLockWait(waitTime)
		}
		opErr := errfmt.Errorf("operation %s exceeded timeout %v while waiting for rlock: %w", operation, timeout, err)
		if logger != nil {
			logger.Warn("Operation timeout while waiting for rlock",
				LockField{Key: lockLogFieldOperation, Value: operation},
				LockField{Key: lockLogFieldTimeout, Value: timeout.String()},
				LockField{Key: lockLogFieldError, Value: opErr.Error()})
		}
		return opErr
	}

	waitTime := time.Since(acquireStart)
	if waitTime > lockWaitLogThreshold {
		if logger != nil {
			logger.Debug("RLock acquired after wait",
				LockField{Key: lockLogFieldOperation, Value: operation},
				LockField{Key: lockLogFieldWaitTime, Value: waitTime.String()})
		}
		if metrics != nil {
			metrics.RecordLockWait(waitTime)
		}
	}

	holdStart := time.Now()
	defer func() {
		holdTime := time.Since(holdStart)
		if holdTime > lockHoldLogThreshold {
			if logger != nil {
				logger.Debug("RLock held for extended period",
					LockField{Key: lockLogFieldOperation, Value: operation},
					LockField{Key: lockLogFieldHoldTime, Value: holdTime.String()})
			}
			if metrics != nil {
				metrics.RecordLockHold(holdTime)
			}
		}

		if holdTime > timeout {
			if logger != nil {
				logger.Warn("Operation exceeded timeout while holding rlock (synchronous execution)",
					LockField{Key: lockLogFieldOperation, Value: operation},
					LockField{Key: lockLogFieldTimeout, Value: timeout.String()},
					LockField{Key: lockLogFieldHoldTime, Value: holdTime.String()})
			}
		}

		mu.RUnlock()
	}()

	var opErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				opErr = errfmt.Errorf("panic in rlock operation %s: %v", operation, r)
			}
		}()
		opErr = fn()
	}()

	return opErr
}

// DefaultLockTimeout returns default timeout for lock operations
// Used when metrics are not available
func DefaultLockTimeout() time.Duration {
	return 30 * time.Second
}

// CalculateLockTimeout calculates timeout based on operation size and metrics
// This is a convenience function for callers that want to calculate timeout separately
func CalculateLockTimeout(operationSize int, objectsPerSecond float64, baseTimeout time.Duration) time.Duration {
	if objectsPerSecond <= 0 {
		return baseTimeout
	}

	estimatedDuration := time.Duration(float64(operationSize)/objectsPerSecond) * time.Second
	if estimatedDuration > baseTimeout {
		return estimatedDuration * 2 // Add 2x buffer
	}
	return baseTimeout
}

// WithLock executes a function with a lock and timeout monitoring (convenience overload)
// Uses context.Background() and no metrics/logger - for simple cases where defaults are sufficient
func WithLock(mu sync.Locker, operation string, fn func() error) error {
	return WithLockTimeout(mu, context.Background(), nil, nil, operation, fn)
}

// WithRLock executes a function with a read lock and timeout monitoring (convenience overload)
// Uses context.Background() and no metrics/logger - for simple cases where defaults are sufficient
func WithRLock(mu *sync.RWMutex, operation string, fn func() error) error {
	return WithRLockTimeout(mu, context.Background(), nil, nil, operation, fn)
}

// WithLockCtx executes a function with a lock and timeout monitoring (convenience overload with context)
// Uses provided context and no metrics/logger - for cases where you need a specific context
func WithLockCtx(mu sync.Locker, ctx context.Context, operation string, fn func() error) error {
	return WithLockTimeout(mu, ctx, nil, nil, operation, fn)
}

// WithRLockCtx executes a function with a read lock and timeout monitoring (convenience overload with context)
// Uses provided context and no metrics/logger - for cases where you need a specific context
func WithRLockCtx(mu *sync.RWMutex, ctx context.Context, operation string, fn func() error) error {
	return WithRLockTimeout(mu, ctx, nil, nil, operation, fn)
}

// WithLockLogger executes a function with a lock and timeout monitoring (convenience overload with logger)
// Uses context.Background() and no metrics - for cases where you need logging but not metrics
func WithLockLogger(mu sync.Locker, operation string, logger LockLogger, fn func() error) error {
	return WithLockTimeout(mu, context.Background(), nil, logger, operation, fn)
}

// WithRLockLogger executes a function with a read lock and timeout monitoring (convenience overload with logger)
// Uses context.Background() and no metrics - for cases where you need logging but not metrics
func WithRLockLogger(mu *sync.RWMutex, operation string, logger LockLogger, fn func() error) error {
	return WithRLockTimeout(mu, context.Background(), nil, logger, operation, fn)
}

// WithLockCtxLogger executes a function with a lock and timeout monitoring (convenience overload with context and logger)
// Uses no metrics - for cases where you need context and logging but not metrics
func WithLockCtxLogger(mu sync.Locker, ctx context.Context, operation string, logger LockLogger, fn func() error) error {
	return WithLockTimeout(mu, ctx, nil, logger, operation, fn)
}

// WithRLockCtxLogger executes a function with a read lock and timeout monitoring (convenience overload with context and logger)
// Uses no metrics - for cases where you need context and logging but not metrics
func WithRLockCtxLogger(mu *sync.RWMutex, ctx context.Context, operation string, logger LockLogger, fn func() error) error {
	return WithRLockTimeout(mu, ctx, nil, logger, operation, fn)
}
