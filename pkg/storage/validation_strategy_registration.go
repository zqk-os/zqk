package storage

import (
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"

	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/objects"
	// HighVolumeKinds lists the kinds that should use async validation due to high write frequency.
	// These are registered with AsyncCacheValidationStrategy when InitializeAsyncValidationStrategies is called.
	// Align with high_volume_kinds.yaml and streamStorageEnabledKinds where applicable.
)

var HighVolumeKinds = []string{
	objects.KindAuditEvent,   // Created on every CLI command - highest frequency
	objects.KindMcpSession,   // Created per MCP server session - high during active use
	objects.KindZqkSession,   // Created per CLI or interactive session - high during active use
	objects.KindSchedulerJob, // Created by triggers and test_case runs; high at scale
	objects.KindDocEntry,     // Batch-created during docman-sync - spiky but large batches
}

// asyncStrategiesState tracks initialization state for async strategies.
type asyncStrategiesState struct {
	initialized bool
	projectRoot string
	strategies  map[string]*AsyncCacheValidationStrategy
}

var (
	asyncState   asyncStrategiesState
	asyncStateMu sync.RWMutex
)

// InitializeAsyncValidationStrategies sets up async validation strategies for high-volume kinds.
// This should be called once during storage initialization with the project root.
// Thread-safe and idempotent - subsequent calls with the same project root are no-ops.
//
// The function:
// 1. Creates AsyncCacheValidationStrategy instances for each high-volume kind
// 2. Registers them with the global ValidationStrategyRegistry
// 3. Starts the background scanners
// 4. Registers a shutdown handler with the QueueShutdownCoordinator
//
// Returns the number of strategies initialized (0 if already initialized).
func InitializeAsyncValidationStrategies(projectRoot string) int {
	var count int
	var needsShutdownRegistration bool
	_err_84062965 := concurrency.RunInLock(&asyncStateMu, func() error {
		// Skip if already initialized for this project root
		if asyncState.initialized && asyncState.projectRoot == projectRoot {
			return nil
		}

		// If initialized for a different project root, stop old strategies first
		if asyncState.initialized && asyncState.projectRoot != projectRoot {
			for _, strategy := range asyncState.strategies {
				_err_84063418 := strategy.Stop()
				if _err_84063418 !=

					// Track if this is first initialization (need to register shutdown handler)
					nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84063418).Log()
				}
			}
		}

		needsShutdownRegistration = !asyncState.initialized

		// Initialize new state
		asyncState = asyncStrategiesState{
			initialized: true,
			projectRoot: projectRoot,
			strategies:  make(map[string]*AsyncCacheValidationStrategy),
		}

		registry := GetGlobalValidationStrategyRegistry()

		for _, kind := range HighVolumeKinds {
			kindDir := datacell.CellCASPrimaryDir(projectRoot, kindDirName(kind))

			// Create and configure the strategy
			strategy := NewAsyncCacheValidationStrategy(&AsyncCacheConfig{
				ScanInterval: 3 * time.Second, // Relaxed scan interval for high-volume to prevent scan storms and GC thrash
			})

			// Register with the global registry
			registry.RegisterStrategy(kind, strategy)

			// Start the background scanner
			if err := strategy.Start(kindDir); err != nil {
				// Log but continue - sync fallback will be used
				continue
			}

			asyncState.strategies[kind] = strategy
			count++
		}

		return nil
	})
	if _err_84062965 !=

		// Register shutdown handler outside of lock to avoid potential deadlock
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84062965).Log()
	}

	if needsShutdownRegistration {
		registerAsyncValidationShutdownHandler()
	}

	return count
}

// ShutdownAsyncValidationStrategies stops all async validation strategies.
// Should be called during graceful shutdown.
func ShutdownAsyncValidationStrategies() {
	_err_84064785 := concurrency.RunInLock(&asyncStateMu, func() error {
		if !asyncState.initialized {
			return nil
		}

		for _, strategy := range asyncState.strategies {
			_err_84064943 := strategy.Stop()
			if _err_84064943 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// GetAsyncStrategyStats returns statistics for all active async strategies.
					// Returns a map of kind -> stats.
					Error(ErrMsgSwallowedError, _err_84064943).Log()

			}
		}

		asyncState = asyncStrategiesState{}
		return nil
	})
	if _err_84064785 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84064785).Log()
	}
}

func GetAsyncStrategyStats() map[string]AsyncStrategyStats {
	stats := make(map[string]AsyncStrategyStats)
	_err_84065202 := concurrency.RunInRLock(&asyncStateMu, func() error {
		if !asyncState.initialized {
			return nil
		}

		for kind, strategy := range asyncState.strategies {
			hits, misses, fallbacks, scans, swaps, lastScanDuration := strategy.GetCacheStats()
			stats[kind] = AsyncStrategyStats{
				Kind:               kind,
				CacheHits:          hits,
				CacheMisses:        misses,
				FallbacksToSync:    fallbacks,
				ScanCount:          scans,
				SwapCount:          swaps,
				LastScanDurationNs: lastScanDuration,
			}
		}
		return nil
	})
	if _err_84065202 != nil {
		logging.

			// AsyncStrategyStats holds statistics for an async validation strategy.
			Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84065202).Log()
	}

	return stats
}

type AsyncStrategyStats struct {
	Kind               string `json:"kind"`
	CacheHits          int64  `json:"cache_hits"`
	CacheMisses        int64  `json:"cache_misses"`
	FallbacksToSync    int64  `json:"fallbacks_to_sync"`
	ScanCount          int64  `json:"scan_count"`
	SwapCount          int64  `json:"swap_count"`
	LastScanDurationNs int64  `json:"last_scan_duration_ns"`
}

// kindDirName returns the directory name for a kind.
// Most kinds use underscores, but some may have different conventions.
func kindDirName(kind string) string {
	// Must match FileObjectStorage / CAS kind directories (e.g. doc_entry → doc_entries).
	// Using the raw kind string made async validation scan an empty/wrong tree, so the
	// existence cache never saw new hash files and treated them as stale on save.
	if dir := objects.GetDirectoryFromKind(kind); dir != emptyValue {
		return dir
	}
	switch kind {
	case objects.KindAuditEvent:
		return "audit"
	default:
		return kind
	}
}

// IsAsyncValidationEnabled returns true if async validation is initialized.
func IsAsyncValidationEnabled() bool {
	var enabled bool
	_err_84066795 := concurrency.RunInRLock(&asyncStateMu, func() error {
		enabled = asyncState.initialized
		return nil
	})
	if _err_84066795 != nil {
		logging.

			// GetAsyncValidationKinds returns the list of kinds using async validation.
			Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84066795).Log()
	}
	return enabled
}

func GetAsyncValidationKinds() []string {
	var kinds []string
	_err_84067061 := concurrency.RunInRLock(&asyncStateMu, func() error {
		if !asyncState.initialized {
			return nil
		}
		kinds = make([]string, 0, len(asyncState.strategies))
		for kind := range asyncState.strategies {
			kinds = append(kinds, kind)
		}
		return nil
	})
	if _err_84067061 != nil {
		logging.

			// asyncValidationShutdownHandler implements QueueShutdownHandler for async validation strategies.
			Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84067061).Log()
	}
	return kinds
}

type asyncValidationShutdownHandler struct{}

var (
	asyncShutdownHandler     *asyncValidationShutdownHandler
	asyncShutdownHandlerOnce sync.Once
)

// getAsyncValidationShutdownHandler returns the singleton shutdown handler.
func getAsyncValidationShutdownHandler() *asyncValidationShutdownHandler {
	asyncShutdownHandlerOnce.Do(func() {
		asyncShutdownHandler = &asyncValidationShutdownHandler{}
	})
	return asyncShutdownHandler
}

// GetName returns the handler name for logging.
func (h *asyncValidationShutdownHandler) GetName() string {
	return ConstMiscAsyncValidationStrategies
}

// InitiateShutdown stops accepting new validation requests.
func (h *asyncValidationShutdownHandler) InitiateShutdown() error {
	ShutdownAsyncValidationStrategies()
	return nil
}

// Drain is a no-op for validation strategies (no pending work to drain).
func (h *asyncValidationShutdownHandler) Drain(_ context.Context) error {
	return nil
}

// IsDrained returns true (validation strategies don't have pending work).
func (h *asyncValidationShutdownHandler) IsDrained() bool {
	return true
}

// GetPendingCount returns 0 (validation strategies don't queue work).
func (h *asyncValidationShutdownHandler) GetPendingCount() int64 {
	return 0
}

// IsCritical returns false (validation strategies are not critical).
func (h *asyncValidationShutdownHandler) IsCritical() bool {
	return false
}

// registerAsyncValidationShutdownHandler registers the handler with the shutdown coordinator.
// Called automatically during initialization.
func registerAsyncValidationShutdownHandler() {
	coordinator := GetGlobalShutdownCoordinator()
	if coordinator != nil {
		coordinator.RegisterQueue(getAsyncValidationShutdownHandler())
	}
}
