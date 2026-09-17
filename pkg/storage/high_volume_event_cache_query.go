// Extracted from high_volume_event_cache.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"sort"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

func (c *HighVolumeEventCache) QueryByTimeWindow(startTime, endTime time.Time, limit int) []string {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var result []string
	var err_swallow_23 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheQueryTimeWindow,
		func() error {
			if len(c.byTime) == 0 {
				return nil
			}

			startIdx := sort.Search(len(c.byTime), func(i int) bool {
				return !c.byTime[i].CreatedAt.Before(startTime)
			})

			endIdx := sort.Search(len(c.byTime), func(i int) bool {
				return c.byTime[i].CreatedAt.After(endTime)
			})

			result = make([]string, 0, endIdx-startIdx)
			for i := startIdx; i < endIdx && (limit <= 0 || len(result) < limit); i++ {
				if c.byTime[i].Exists {
					result = append(result, c.byTime[i].ID)
				}
			}
			return nil
		},
	)
	if err_swallow_23 != nil {

		// QueryOlderThan returns event IDs older than cutoff time
		// Note: If cache is partial (e.g., only 10k of 17k events cached), this will only return
		// IDs from cached events. The aggregation service falls back to storage queries if cache
		// returns empty or insufficient results.
		logging.LogSwallowedError(err_swallow_23)
	}
	return result
}

func (c *HighVolumeEventCache) QueryOlderThan(cutoffTime time.Time, limit int) []string {
	return c.QueryByTimeWindow(time.Time{}, cutoffTime, limit)
}

// QueryOldestByKind returns the oldest event IDs for a given kind (by created_at), up to limit.
// Used by retention max_count enforcement to delete oldest audit_events without List() over 260k+ files.
func (c *HighVolumeEventCache) QueryOldestByKind(kind string, limit int) []string {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var result []string
	var err_swallow_24 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheQueryOldestByKind,
		func() error {
			for _, entry := range c.byTime {
				if limit > 0 && len(result) >= limit {
					break
				}
				if entry != nil && entry.Exists && entry.Kind == kind {
					result = append(result, entry.ID)
				}
			}
			return nil
		},
	)
	if err_swallow_24 != nil {

		// QueryOldestByKindExcludingStatus returns oldest event IDs for kind (by created_at) whose status is not in excludeStatuses, up to limit.
		// Use when cache was built from index/stream (HasStatus true); avoids full List for retention max_count with protect_statuses. See RETENTION_MAX_COUNT_PERFORMANCE.md.
		logging.LogSwallowedError(err_swallow_24)
	}
	return result
}

func (c *HighVolumeEventCache) QueryOldestByKindExcludingStatus(kind string, limit int, excludeStatuses []string) []string {
	excludeSet := make(map[string]bool)
	for _, s := range excludeStatuses {
		excludeSet[s] = true
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var result []string
	var err_swallow_25 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheQueryOldestExcludingStatus,
		func() error {
			for _, entry := range c.byTime {
				if limit > 0 && len(result) >= limit {
					break
				}
				if entry == nil || !entry.Exists || entry.Kind != kind {
					continue
				}
				if entry.Status == emptyValue || entry.Status == objects.ObjectStatusUnspecified {
					continue
				}
				if excludeSet[entry.Status] {
					continue
				}
				result = append(result, entry.ID)
			}
			return nil
		},
	)
	if err_swallow_25 != nil {

		// HasStatus returns true when cache entries have Status populated (built from index/stream). False when loaded from disk.
		logging.LogSwallowedError(err_swallow_25)
	}
	return result
}

func (c *HighVolumeEventCache) HasStatus() bool {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var has bool
	var err_swallow_26 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheHasStatus,
		func() error {
			has = c.metadata != nil && c.metadata.HasStatus
			return nil
		},
	)
	if err_swallow_26 !=

		// Count returns total count of events in cache
		nil {
		logging.LogSwallowedError(err_swallow_26)
	}
	return has
}

func (c *HighVolumeEventCache) Count() int {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var count int
	var err_swallow_27 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheCount,
		func() error {
			count = len(c.cache)
			return nil
		},
	)
	if err_swallow_27 != nil {

		// CountByKind returns count of events for a specific kind
		logging.LogSwallowedError(err_swallow_27)
	}
	return count
}

func (c *HighVolumeEventCache) CountByKind(kind string) int {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var count int
	var err_swallow_28 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheCountByKind,
		func() error {
			for _, entry := range c.cache {
				if entry != nil && entry.Exists && entry.Kind == kind {
					count++
				}
			}
			return nil
		},
	)
	if err_swallow_28 != nil {

		// CountByTimeWindow returns count of events within a time window
		logging.LogSwallowedError(err_swallow_28)
	}
	return count
}

func (c *HighVolumeEventCache) CountByTimeWindow(startTime, endTime time.Time) int {
	ids := c.QueryByTimeWindow(startTime, endTime, 0)
	return len(ids)
}

// IsPopulatedForProject returns true if cache has been built for the given projectRoot
func (c *HighVolumeEventCache) IsPopulatedForProject(projectRoot string) bool {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var populated bool
	var err_swallow_29 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheIsPopulated,
		func() error {
			if c.metadata == nil {
				populated = false
				return nil
			}
			populated = c.metadata.ProjectRoot == projectRoot && len(c.cache) > 0
			return nil
		},
	)
	if err_swallow_29 != nil {
		logging.

			// BuildCache builds the cache by scanning high-volume event directories.
			// Heavy work (List, building map/slice) runs in a background goroutine; the only lock hold is the final swap.
			// Caller blocks until the background build and swap complete (so cache is ready when BuildCache returns).
			LogSwallowedError(err_swallow_29)
	}
	return populated
}

// Only block (hold lock) during the final swap.

// built from index/stream so entries have Status for retention

// Respect ctx deadline so aggregation handler 10s timeout works (scheduler diagnostics: BuildCache was blocking 78+ min).

// buildCacheData builds the cache. Prefers index-based path (CAS ListIDs + parallel read) to avoid
// List()'s 500s legacy scan and 47k full reads; falls back to List when not FileObjectStorage.

// Fast path: build from CAS index and stream registry for all high-volume kinds. Merge by ID so stream-backed
// objects (scheduler_job, zqk_session, etc.) are included after rebuild/restart. Enables efficient Count/OldestIDs (HIGH_VOLUME_EVENT_INDEXES.md).

//nolint:gosec // Safe: totalEntries.Load() is unlikely to exceed int range for log counts.

// Fallback: List (slow for 47k: legacy scan + full reads; may timeout)

// buildOneKindForHighVolumeCache aggregates CAS index entries and stream overlay for one kind.
// Index/list errors are non-fatal (logged, empty/partial merge). Returns (nil, err) only for context cancellation.

// stream overwrites CAS when both exist

// buildCacheFromIndex builds cache entries from CAS index (ListIDs) + parallel minimal read. No legacy scan.

// Cap work when ctx has deadline so aggregation handler timeout can stop build (scheduler diagnostics: 78+ min blocks).
