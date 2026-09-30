package validation

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ValidationMetrics tracks performance metrics for async validation
// Implements concurrency.LockMetrics for timeout mutex integration
type ValidationMetrics struct {
	mu sync.RWMutex

	// Timing metrics
	StartTime          time.Time `json:"start_time"`
	EndTime            time.Time `json:"end_time"`
	TotalDuration      string    `json:"total_duration"`
	EnqueueDuration    string    `json:"enqueue_duration"`
	ValidationDuration string    `json:"validation_duration"`
	CollectionDuration string    `json:"collection_duration"`

	// Count metrics
	TotalObjects     int `json:"total_objects"`
	ValidatedObjects int `json:"validated_objects"`
	FailedObjects    int `json:"failed_objects"`
	CacheHits        int `json:"cache_hits"`
	CacheMisses      int `json:"cache_misses"`
	Retries          int `json:"retries"`

	// Performance metrics
	ObjectsPerSecond      float64 `json:"objects_per_second"`
	AverageValidationTime string  `json:"average_validation_time"`

	// Worker metrics
	WorkerCount      int     `json:"worker_count"`
	MaxQueueSize     int     `json:"max_queue_size"`
	AverageQueueSize float64 `json:"average_queue_size"`

	// Breakdown by tier
	Tier1Count int `json:"tier1_count"`
	Tier2Count int `json:"tier2_count"`
	Tier3Count int `json:"tier3_count"`
	Tier4Count int `json:"tier4_count"`

	// Memory metrics (if available)
	PeakMemoryMB float64 `json:"peak_memory_mb,omitempty"`

	// Hash registry cache metrics (deadlock detection)
	HashRegistryCacheLockWaitDurationMs     []int64 `json:"hash_registry_cache_lock_wait_duration_ms"`     // Time waiting for cache lock
	HashRegistryCacheLockHoldDurationMs     []int64 `json:"hash_registry_cache_lock_hold_duration_ms"`     // Time holding cache lock
	HashRegistryLoadDurationMs              []int64 `json:"hash_registry_load_duration_ms"`                // Time to load registry from disk
	HashRegistryConcurrentAccessCount       int     `json:"hash_registry_concurrent_access_count"`         // Workers accessing same cache key
	HashRegistryOperationsPerValidation     []int   `json:"hash_registry_operations_per_validation"`       // GetHash/SetHash/Save calls per validation
	HashRegistryBucketedProcessingTimeMs    []int64 `json:"hash_registry_bucketed_processing_time_ms"`     // Processing time for bucketed objects
	HashRegistryNonBucketedProcessingTimeMs []int64 `json:"hash_registry_non_bucketed_processing_time_ms"` // Processing time for non-bucketed objects

	// Lock contention tracking (for LockMetrics interface)
	lockWaitCount       int   // Total number of lock wait events
	lockContentionCount int   // Number of contention events (wait > 50ms)
	lockTimeoutCount    int   // Number of timeout events
	maxLockWaitTime     int64 // Maximum lock wait time in milliseconds
}

// NewValidationMetrics creates a new metrics tracker
func NewValidationMetrics() *ValidationMetrics {
	return &ValidationMetrics{
		StartTime:                               time.Now(),
		HashRegistryCacheLockWaitDurationMs:     make([]int64, 0),
		HashRegistryCacheLockHoldDurationMs:     make([]int64, 0),
		HashRegistryLoadDurationMs:              make([]int64, 0),
		HashRegistryOperationsPerValidation:     make([]int, 0),
		HashRegistryBucketedProcessingTimeMs:    make([]int64, 0),
		HashRegistryNonBucketedProcessingTimeMs: make([]int64, 0),
	}
}

// GetObjectsPerSecond implements concurrency.LockMetrics
func (vm *ValidationMetrics) GetObjectsPerSecond() float64 {
	var result float64
	if err := concurrency.RunInRLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsGetObjectsPerSecond,
		lockLoggerSystem(),
		func() error {
			result = vm.ObjectsPerSecond
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockGetObjectsPerSec, err).Log()
	}
	return result
}

// RecordLockWait implements concurrency.LockMetrics
// Alias for RecordHashRegistryCacheLockWait for interface compatibility
func (vm *ValidationMetrics) RecordLockWait(duration time.Duration) {
	vm.RecordHashRegistryCacheLockWait(duration)
	// Track contention (wait > 50ms indicates contention)
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordLockWaitTracking,
		lockLoggerSystem(),
		func() error {
			vm.lockWaitCount++
			if duration > 50*time.Millisecond {
				vm.lockContentionCount++
			}
			waitMs := duration.Milliseconds()
			if waitMs > vm.maxLockWaitTime {
				vm.maxLockWaitTime = waitMs
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordLockHold implements concurrency.LockMetrics
			// Alias for RecordHashRegistryCacheLockHold for interface compatibility
			Error(ErrMsgLockRecordWait, err).Log()
	}
}

func (vm *ValidationMetrics) RecordLockHold(duration time.Duration) {
	vm.RecordHashRegistryCacheLockHold(duration)
}

// GetContentionRate implements concurrency.LockMetrics
// Returns contention rate based on hash registry cache lock wait times
func (vm *ValidationMetrics) GetContentionRate() float64 {
	var result float64
	if err := concurrency.RunInRLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsGetContentionRate,
		lockLoggerSystem(),
		func() error {
			if vm.lockWaitCount == 0 {
				result = 0.0
				return nil
			}
			result = float64(vm.lockContentionCount) / float64(vm.lockWaitCount)
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockGetContentionRate, err).Log()
	}
	return result
}

// GetTimeoutRate implements concurrency.LockMetrics
// Returns timeout rate based on recorded timeout events
func (vm *ValidationMetrics) GetTimeoutRate() float64 {
	var result float64
	if err := concurrency.RunInRLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsGetTimeoutRate,
		lockLoggerSystem(),
		func() error {
			if vm.lockWaitCount == 0 {
				result = 0.0
				return nil
			}
			result = float64(vm.lockTimeoutCount) / float64(vm.lockWaitCount)
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockGetTimeoutRate, err).Log()
	}
	return result
}

// GetMaxWaitTime implements concurrency.LockMetrics
// Returns maximum lock wait time observed
func (vm *ValidationMetrics) GetMaxWaitTime() time.Duration {
	var result time.Duration
	if err := concurrency.RunInRLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsGetMaxWaitTime,
		lockLoggerSystem(),
		func() error {
			result = time.Duration(vm.maxLockWaitTime) * time.Millisecond
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockGetMaxWaitTime, err).Log()
	}
	return result
}

// RecordLockTimeout records a lock timeout event (for metrics tracking)
func (vm *ValidationMetrics) RecordLockTimeout() {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordLockTimeout,
		lockLoggerSystem(),
		func() error {
			vm.lockTimeoutCount++
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordHashRegistryCacheLockWait records time waiting for cache lock
			Error(ErrMsgLockRecordTimeout, err).Log()
	}
}

func (vm *ValidationMetrics) RecordHashRegistryCacheLockWait(duration time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordLockWait,
		lockLoggerSystem(),
		func() error {
			vm.HashRegistryCacheLockWaitDurationMs = append(vm.HashRegistryCacheLockWaitDurationMs, duration.Milliseconds())
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordHashRegistryCacheLockHold records time holding cache lock
			Error(ErrMsgLockRecordHRCacheWait, err).Log()
	}
}

func (vm *ValidationMetrics) RecordHashRegistryCacheLockHold(duration time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordLockHold,
		lockLoggerSystem(),
		func() error {
			vm.HashRegistryCacheLockHoldDurationMs = append(vm.HashRegistryCacheLockHoldDurationMs, duration.Milliseconds())
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordHashRegistryLoad records time to load registry from disk
			Error(ErrMsgLockRecordHRCacheHold, err).Log()
	}
}

func (vm *ValidationMetrics) RecordHashRegistryLoad(duration time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordRegistryLoad,
		lockLoggerSystem(),
		func() error {
			vm.HashRegistryLoadDurationMs = append(vm.HashRegistryLoadDurationMs, duration.Milliseconds())
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// IncrementHashRegistryConcurrentAccess increments concurrent access counter
			Error(ErrMsgLockRecordHRLoad, err).Log()
	}
}

func (vm *ValidationMetrics) IncrementHashRegistryConcurrentAccess() {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsIncrementConcurrentAccess,
		lockLoggerSystem(),
		func() error {
			vm.HashRegistryConcurrentAccessCount++
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordHashRegistryOperations records number of registry operations per validation
			Error(ErrMsgLockIncHRAccess, err).Log()
	}
}

func (vm *ValidationMetrics) RecordHashRegistryOperations(count int) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordRegistryOperations,
		lockLoggerSystem(),
		func() error {
			vm.HashRegistryOperationsPerValidation = append(vm.HashRegistryOperationsPerValidation, count)
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordHashRegistryBucketedProcessing records processing time for bucketed objects
			Error(ErrMsgLockRecordHROps, err).Log()
	}
}

func (vm *ValidationMetrics) RecordHashRegistryBucketedProcessing(duration time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordBucketedProcessing,
		lockLoggerSystem(),
		func() error {
			vm.HashRegistryBucketedProcessingTimeMs = append(vm.HashRegistryBucketedProcessingTimeMs, duration.Milliseconds())
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordHashRegistryNonBucketedProcessing records processing time for non-bucketed objects
			Error(ErrMsgLockRecordHRBucket, err).Log()
	}
}

func (vm *ValidationMetrics) RecordHashRegistryNonBucketedProcessing(duration time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordNonBucketedProcessing,
		lockLoggerSystem(),
		func() error {
			vm.HashRegistryNonBucketedProcessingTimeMs = append(vm.HashRegistryNonBucketedProcessingTimeMs, duration.Milliseconds())
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordEnqueue records enqueue timing
			Error(ErrMsgLockRecordHRNonBucket, err).Log()
	}
}

func (vm *ValidationMetrics) RecordEnqueue(duration time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordEnqueue,
		lockLoggerSystem(),
		func() error {
			vm.EnqueueDuration = duration.String()
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordValidation records validation timing
			Error(ErrMsgLockRecordEnqueue, err).Log()
	}
}

func (vm *ValidationMetrics) RecordValidation(duration time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordValidation,
		lockLoggerSystem(),
		func() error {
			vm.ValidationDuration = duration.String()
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordCollection records result collection timing
			Error(ErrMsgLockRecordValidation, err).Log()
	}
}

func (vm *ValidationMetrics) RecordCollection(duration time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordCollection,
		lockLoggerSystem(),
		func() error {
			vm.CollectionDuration = duration.String()
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// IncrementValidated increments validated object count
			Error(ErrMsgLockRecordCollection, err).Log()
	}
}

func (vm *ValidationMetrics) IncrementValidated() {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsIncrementValidated,
		lockLoggerSystem(),
		func() error {
			vm.ValidatedObjects++
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// IncrementFailed increments failed object count
			Error(ErrMsgLockIncValidated, err).Log()
	}
}

func (vm *ValidationMetrics) IncrementFailed() {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsIncrementFailed,
		lockLoggerSystem(),
		func() error {
			vm.FailedObjects++
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// IncrementCacheHit increments cache hit count
			Error(ErrMsgLockIncFailed, err).Log()
	}
}

func (vm *ValidationMetrics) IncrementCacheHit() {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsIncrementCacheHit,
		lockLoggerSystem(),
		func() error {
			vm.CacheHits++
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// IncrementCacheMiss increments cache miss count
			Error(ErrMsgLockIncCacheHit, err).Log()
	}
}

func (vm *ValidationMetrics) IncrementCacheMiss() {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsIncrementCacheMiss,
		lockLoggerSystem(),
		func() error {
			vm.CacheMisses++
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// IncrementRetry increments retry count
			Error(ErrMsgLockIncCacheMiss, err).Log()
	}
}

func (vm *ValidationMetrics) IncrementRetry() {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsIncrementRetry,
		lockLoggerSystem(),
		func() error {
			vm.Retries++
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// SetTotalObjects sets the total object count
			ProfileSystem))).Error(ErrMsgLockIncRetry, err).Log()
	}
}

func (vm *ValidationMetrics) SetTotalObjects(count int) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsSetTotalObjects,
		lockLoggerSystem(),
		func() error {
			vm.TotalObjects = count
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// SetWorkerCount sets the worker count
			Error(ErrMsgLockSetTotalObjects, err).Log()
	}
}

func (vm *ValidationMetrics) SetWorkerCount(count int) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsSetWorkerCount,
		lockLoggerSystem(),
		func() error {
			vm.WorkerCount = count
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// UpdateQueueSize updates queue size metrics
			Error(ErrMsgLockSetWorkerCount, err).Log()
	}
}

func (vm *ValidationMetrics) UpdateQueueSize(size int) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsUpdateQueueSize,
		lockLoggerSystem(),
		func() error {
			if size > vm.MaxQueueSize {
				vm.MaxQueueSize = size
			}
			// Simple average calculation (could be improved with proper tracking)
			if vm.AverageQueueSize == 0 {
				vm.AverageQueueSize = float64(size)
			} else {
				vm.AverageQueueSize = (vm.AverageQueueSize + float64(size)) / 2
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// RecordTierIssue records an issue by tier
			Error(ErrMsgLockUpdateQueueSize, err).Log()
	}
}

func (vm *ValidationMetrics) RecordTierIssue(tier int) {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsRecordTierIssue,
		lockLoggerSystem(),
		func() error {
			switch tier {
			case 1:
				vm.Tier1Count++
			case 2:
				vm.Tier2Count++
			case 3:
				vm.Tier3Count++
			case 4:
				vm.Tier4Count++
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Finalize calculates final metrics
			Error(ErrMsgLockRecordTierIssue, err).Log()
	}
}

func (vm *ValidationMetrics) Finalize() {
	if err := concurrency.RunInLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsFinalize,
		lockLoggerSystem(),
		func() error {
			vm.EndTime = time.Now()
			totalDuration := vm.EndTime.Sub(vm.StartTime)
			vm.TotalDuration = totalDuration.String()

			// Calculate objects per second
			if totalDuration.Seconds() > 0 {
				vm.ObjectsPerSecond = float64(vm.ValidatedObjects) / totalDuration.Seconds()
			}

			// Calculate average validation time
			if vm.ValidatedObjects > 0 {
				validationDur, err := time.ParseDuration(vm.ValidationDuration)
				if err != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
				}
				if validationDur > 0 {
					avgTime := validationDur / time.Duration(vm.ValidatedObjects)
					vm.AverageValidationTime = avgTime.String()
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// JSONSnapshot returns the current metrics as JSON (read lock). Does not call Finalize;
			// callers should invoke Finalize before this when the run is complete.
			ProfileSystem))).Error(ErrMsgLockFinalize, err).Log()
	}
}

func (vm *ValidationMetrics) JSONSnapshot() ([]byte, error) {
	var data []byte
	err := concurrency.RunInRLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsJsonSnapshot,
		lockLoggerSystem(),
		func() error {
			var marshalErr error
			data, marshalErr = json.Marshal(vm)
			return marshalErr
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgMarshalMetrics).Wrap(err)
	}
	return data, nil
}

// Save saves metrics to a JSON file
func (vm *ValidationMetrics) Save(filePath string) error {
	// Finalize first (needs write lock)
	vm.Finalize()

	var data []byte
	var err error
	if err := concurrency.RunInRLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsToJson,
		lockLoggerSystem(),
		func() error {
			data, err = json.MarshalIndent(vm, "", "  ")
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockSave, err).Log()
	}
	if err != nil {
		return errfmt.Newf(ErrMsgMarshalMetrics).Wrap(err)
	}

	if err := fileutil.WriteFile(filePath, data, paths.FilePerm600); err != nil {
		return errfmt.Newf(ErrMsgWriteMetricsFile).Wrap(err)
	}

	return nil
}

// String returns a human-readable summary
func (vm *ValidationMetrics) String() string {
	var totalDuration string
	var validatedObjects, failedObjects, cacheHits, cacheMisses int
	var objectsPerSecond float64
	var workerCount, maxQueueSize int
	var averageQueueSize float64
	var tier1Count, tier2Count, tier3Count, tier4Count int
	var startTime, endTime time.Time
	if err := concurrency.RunInRLockWithLogger(
		&vm.mu,
		LockNameValidationMetricsString,
		lockLoggerSystem(),
		func() error {
			// Calculate total duration if not already set (Finalize() may not have been called)
			totalDuration = vm.TotalDuration
			startTime = vm.StartTime
			endTime = vm.EndTime
			if totalDuration == emptyValue {
				end := endTime
				if end.IsZero() {
					end = time.Now()
				}
				duration := end.Sub(startTime)
				totalDuration = duration.String()
			}
			validatedObjects = vm.ValidatedObjects
			failedObjects = vm.FailedObjects
			cacheHits = vm.CacheHits
			cacheMisses = vm.CacheMisses
			objectsPerSecond = vm.ObjectsPerSecond
			workerCount = vm.WorkerCount
			maxQueueSize = vm.MaxQueueSize
			averageQueueSize = vm.AverageQueueSize
			tier1Count = vm.Tier1Count
			tier2Count = vm.Tier2Count
			tier3Count = vm.Tier3Count
			tier4Count = vm.Tier4Count
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// Calculate cache hit rate
			ProfileSystem))).Error(ErrMsgLockString, err).Log()
	}

	if objectsPerSecond == 0 && validatedObjects > 0 {
		dur := time.Duration(0)
		if totalDuration != emptyValue {
			if parsed, err := time.ParseDuration(totalDuration); err == nil {
				dur = parsed
			}
		}
		if dur <= 0 {
			end := endTime
			if end.IsZero() {
				end = time.Now()
			}
			dur = end.Sub(startTime)
		}
		if dur.Seconds() > 0 {
			objectsPerSecond = float64(validatedObjects) / dur.Seconds()
		}
	}

	return fmt.Sprintf(`Validation Metrics:
  Total Duration: %s
  Objects: %d validated, %d failed%s
  Performance: %.2f objects/sec
  Workers: %d
  Queue: max %d, avg %.1f
  Issues: Tier1=%d, Tier2=%d, Tier3=%d, Tier4=%d`,
		totalDuration,
		validatedObjects,
		failedObjects,
		cacheHitRateSummaryLine(cacheHits, cacheMisses),
		objectsPerSecond,
		workerCount,
		maxQueueSize,
		averageQueueSize,
		tier1Count,
		tier2Count,
		tier3Count,
		tier4Count,
	)
}

// cacheHitRateSummaryLine is omitted when the check recorded no hits.
// A constant 0.0% line looks like a broken cache even though system check
// often counts every object as a miss (cold scan or starved warm path).
// print only when IncrementCacheHit ran.
func cacheHitRateSummaryLine(hits, misses int) string {
	if hits <= 0 {
		return ""
	}
	total := hits + misses
	rate := 0.0
	if total > 0 {
		rate = float64(hits) / float64(total) * 100
	}
	return fmt.Sprintf("\n  Cache: %d hits, %d misses (%.1f%% hit rate)", hits, misses, rate)
}

func (vm *ValidationMetrics) cacheHitRate() float64 {
	total := vm.CacheHits + vm.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(vm.CacheHits) / float64(total) * 100
}
