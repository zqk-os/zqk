package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ActivityCacheEntry represents a single job's activity statistics
type ActivityCacheEntry struct {
	JobID          string    `json:"job_id"`
	LastStarted    time.Time `json:"last_started,omitempty"`
	LastCompleted  time.Time `json:"last_completed,omitempty"`
	LastFailed     time.Time `json:"last_failed,omitempty"`
	TotalStarted   int       `json:"total_started"`
	TotalCompleted int       `json:"total_completed"`
	TotalFailed    int       `json:"total_failed"`
	LastDuration   float64   `json:"last_duration_seconds,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	LastEventTime  time.Time `json:"last_event_time"`
	LastEventType  string    `json:"last_event_type"`
}

// ActivityCacheMetadata stores cache metadata
type ActivityCacheMetadata struct {
	Version     string    `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
	ProjectRoot string    `json:"project_root"`
}

// ActivityCache is a thread-safe cache for scheduler job activity
type ActivityCache struct {
	mu       sync.RWMutex
	entries  map[string]*ActivityCacheEntry // jobID -> entry
	metadata *ActivityCacheMetadata
	cacheDir string

	// Single-writer pattern: save requests go through a channel to avoid blocking workers on file I/O
	saveCh       chan saveRequest
	writerOnce   sync.Once
	writerCtx    context.Context
	writerCancel context.CancelFunc
}

type saveRequest struct {
	projectRoot string
}

const (
	activityCacheVersion = "1.0"
	activityCacheFile    = "scheduler_activity_cache.json"
	// maxSchedulerJobIDLenForCache matches storage limit; legacy long-id jobs are pruned from cache.
	maxSchedulerJobIDLenForCache = 200
	// maxActivityCacheEntries caps in-memory (and persisted) entries so the cache cannot grow unbounded.
	// When at cap, oldest entry by LastEventTime is evicted before adding a new one (SCHEDULER_MEMORY_LEAK_INVESTIGATION).
	maxActivityCacheEntries = 20_000
)

var (
	globalActivityCache ActivityCacheInterface
	activityCacheOnce   sync.Once
)

// activityCacheableJobID reports whether a job ID should be stored in the activity cache.
// Legacy long-id jobs (len > maxSchedulerJobIDLenForCache) are excluded so they are not retained or re-saved.
func activityCacheableJobID(jobID string) bool {
	return len(jobID) <= maxSchedulerJobIDLenForCache
}

// GetGlobalActivityCache returns the global activity cache instance
func GetGlobalActivityCache() ActivityCacheInterface {
	activityCacheOnce.Do(func() {
		globalActivityCache = NewActivityCache()
	})
	return globalActivityCache
}

// GetMetadata returns the cache metadata
func (c *ActivityCache) GetMetadata() *ActivityCacheMetadata {
	return c.metadata
}

// Stop stops the activity cache writer goroutine (cleanup on scheduler shutdown).
// Idempotent: safe to call multiple times.
func (c *ActivityCache) Stop() {
	if c.writerCancel != nil {
		c.writerCancel()
	}
}

// NewActivityCache creates a new activity cache
// NewActivityCache creates a new activity cache
func NewActivityCache() ActivityCacheInterface {
	ctx, cancel := context.WithCancel(context.Background()) // Background: request-or-shutdown derived
	return &ActivityCache{
		entries:      make(map[string]*ActivityCacheEntry),
		metadata:     nil,
		cacheDir:     "",
		saveCh:       make(chan saveRequest, 10), // Small buffer to avoid blocking
		writerCtx:    ctx,
		writerCancel: cancel,
	}
}

// getCacheFilePath returns the path to the cache file
func (c *ActivityCache) getCacheFilePath(projectRoot string) string {
	if c.cacheDir != emptyValue {
		return filepath.Join(c.cacheDir, activityCacheFile)
	}
	cacheDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir)
	return filepath.Join(cacheDir, activityCacheFile)
}

// LoadCache loads the cache from disk
func (c *ActivityCache) LoadCache(projectRoot string) error {
	cachePath := c.getCacheFilePath(projectRoot)
	cacheDir := filepath.Dir(cachePath)

	// Ensure cache directory exists
	if err := fileutil.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create cache directory").Wrap(err)
	}

	// Check if cache file exists
	if _, err := fileutil.Stat(cachePath); fileutil.IsNotExist(err) {
		// Cache doesn't exist yet, start with empty cache
		if err := concurrency.RunInLockWithLogger(
			&c.mu, LockNameActivityCacheInitEmpty, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				c.metadata = &ActivityCacheMetadata{
					Version:     activityCacheVersion,
					UpdatedAt:   time.Now(),
					ProjectRoot: projectRoot,
				}
				return nil
			},
		); err != nil {
			return errfmt.Newf("failed to initialize empty cache").Wrap(err)
		}
		return nil
	}

	// Read cache file (NO LOCK HELD)
	data, err := fileutil.ReadFile(cachePath)
	if err != nil {
		return errfmt.Newf("failed to read cache file").Wrap(err)
	}

	var cacheData struct {
		Metadata *ActivityCacheMetadata         `json:"metadata"`
		Entries  map[string]*ActivityCacheEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &cacheData); err != nil {
		// Invalid cache file, start fresh
		if err := concurrency.RunInLockWithLogger(
			&c.mu, LockNameActivityCacheInitInvalid, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				c.metadata = &ActivityCacheMetadata{
					Version:     activityCacheVersion,
					UpdatedAt:   time.Now(),
					ProjectRoot: projectRoot,
				}
				return nil
			},
		); err != nil {
			return errfmt.Newf("failed to initialize cache after unmarshal failure").Wrap(err)
		}
		return nil
	}

	// Validate version
	if cacheData.Metadata == nil || cacheData.Metadata.Version != activityCacheVersion {
		// Version mismatch, start fresh
		if err := concurrency.RunInLockWithLogger(
			&c.mu, LockNameActivityCacheInitVersionMismatch, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				c.metadata = &ActivityCacheMetadata{
					Version:     activityCacheVersion,
					UpdatedAt:   time.Now(),
					ProjectRoot: projectRoot,
				}
				c.entries = make(map[string]*ActivityCacheEntry)
				return nil
			},
		); err != nil {
			return errfmt.Newf("failed to initialize cache after version mismatch").Wrap(err)
		}
		return nil
	}

	// Load entries (re-acquire lock after I/O). Prune legacy long-id job entries so they are not retained or re-saved.
	if err := concurrency.RunInLockWithLogger(
		&c.mu, LockNameActivityCacheLoadEntries, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.metadata = cacheData.Metadata
			c.entries = make(map[string]*ActivityCacheEntry)
			if cacheData.Entries != nil {
				for jobID, entry := range cacheData.Entries {
					if activityCacheableJobID(jobID) && entry != nil {
						c.entries[jobID] = entry
					}
				}
			}
			return nil
		},
	); err != nil {
		return errfmt.Newf("failed to load cache entries").Wrap(err)
	}

	return nil
}

// SaveCache saves the cache to disk asynchronously via a single writer goroutine.
// Non-blocking: sends save request to channel; writer does I/O sequentially to avoid thread explosion.
func (c *ActivityCache) SaveCache(projectRoot string) error {
	// Start writer goroutine on first save (once)
	c.writerOnce.Do(func() {
		b := goroutinelabels.DefaultBudget()
		writerBuilder := goroutinelabels.NewGoroutine("activity_cache_writer", "writing activity cache to disk")
		if b != nil {
			writerBuilder = writerBuilder.WithBudget(b)
		}
		writerBuilder.StartWithContext(c.writerCtx, func(ctx context.Context) error {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case req := <-c.saveCh:
					if err := c.doSaveCache(req.projectRoot); err != nil {
						SLog(logger).Error("ActivityCache: background save failed", err).Log()
					}
				}
			}
		})
	})

	// Send save request (non-blocking if buffer available)
	select {
	case c.saveCh <- saveRequest{projectRoot: projectRoot}:
		return nil
	default:
		// Buffer full - skip save rather than block worker
		return nil
	}
}

// doSaveCache performs the actual file I/O (called by writer goroutine)
func (c *ActivityCache) doSaveCache(projectRoot string) error {
	cachePath := c.getCacheFilePath(projectRoot)
	cacheDir := filepath.Dir(cachePath)

	// Ensure cache directory exists
	if err := fileutil.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create cache directory").Wrap(err)
	}

	// Update metadata and prepare cache data
	var cacheData struct {
		Metadata *ActivityCacheMetadata         `json:"metadata"`
		Entries  map[string]*ActivityCacheEntry `json:"entries"`
	}
	if err := concurrency.RunInLockWithLogger(
		&c.mu, LockNameActivityCacheSavePrepare, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.metadata = &ActivityCacheMetadata{
				Version:     activityCacheVersion,
				UpdatedAt:   time.Now(),
				ProjectRoot: projectRoot,
			}
			cacheData = struct {
				Metadata *ActivityCacheMetadata         `json:"metadata"`
				Entries  map[string]*ActivityCacheEntry `json:"entries"`
			}{
				Metadata: c.metadata,
				Entries:  make(map[string]*ActivityCacheEntry),
			}
			for jobID, entry := range c.entries {
				if !activityCacheableJobID(jobID) {
					continue
				}
				entryCopy := *entry
				cacheData.Entries[jobID] = &entryCopy
			}
			return nil
		},
	); err != nil {
		return errfmt.Newf("failed to acquire lock for cache save preparation").Wrap(err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal cache").Wrap(err)
	}

	// Add trailing newline
	data = append(data, '\n')

	// Write to file atomically (write to temp file, then rename)
	tempPath := cachePath + ".tmp"
	if err := fileutil.WriteFile(tempPath, data, paths.FilePerm644); err != nil { //nolint:gosec // Cache files - 0600 is acceptable
		return errfmt.Newf("failed to write cache file").Wrap(err)
	}
	if err := fileutil.Rename(tempPath, cachePath); err != nil {
		return errfmt.Newf("failed to rename cache file").Wrap(err)
	}

	return nil
}

// UpdateEvent updates the cache with a new job execution event
func (c *ActivityCache) UpdateEvent(jobID, eventType string, duration time.Duration, err error) {
	if jobID == emptyValue || eventType == emptyValue {
		return
	}
	if !activityCacheableJobID(jobID) {
		return
	}

	now := time.Now()
	durationSeconds := duration.Seconds()
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	if err := concurrency.RunInLockWithLogger(
		&c.mu, LockNameActivityCacheUpdateEvent, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			entry, exists := c.entries[jobID]
			if !exists {
				// Evict oldest by LastEventTime when at cap so cache stays bounded (no unbounded growth).
				if len(c.entries) >= maxActivityCacheEntries {
					var oldestID string
					var oldestTime time.Time
					first := true
					for id, e := range c.entries {
						t := e.LastEventTime
						if t.IsZero() {
							t = e.LastStarted
						}
						if first || t.Before(oldestTime) {
							oldestID = id
							oldestTime = t
							first = false
						}
					}
					if oldestID != emptyValue {
						delete(c.entries, oldestID)
					}
				}
				entry = &ActivityCacheEntry{
					JobID: jobID,
				}
				c.entries[jobID] = entry
			}

			// Update based on event type
			switch eventType {
			case "scheduler_job_started":
				entry.LastStarted = now
				entry.TotalStarted++
				entry.LastEventTime = now
				entry.LastEventType = eventType
			case "scheduler_job_completed":
				entry.LastCompleted = now
				entry.TotalCompleted++
				entry.LastDuration = durationSeconds
				entry.LastEventTime = now
				entry.LastEventType = eventType
				entry.LastError = "" // Clear error on success
			case "scheduler_job_failed":
				entry.LastFailed = now
				entry.TotalFailed++
				entry.LastDuration = durationSeconds
				entry.LastError = errorMsg
				entry.LastEventTime = now
				entry.LastEventType = eventType
			}
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ActivityCache: UpdateEvent lock failed", err).Log()
	}
}

// GetEntry retrieves a cache entry for a job
func (c *ActivityCache) GetEntry(jobID string) (*ActivityCacheEntry, bool) {
	var entryCopy *ActivityCacheEntry
	var exists bool
	if err := concurrency.RunInRLockWithLogger(
		&c.mu, LockNameActivityCacheGetEntry, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			entry, found := c.entries[jobID]
			if !found {
				return nil
			}
			// Return a copy to avoid race conditions
			copy := *entry
			entryCopy = &copy
			exists = true
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ActivityCache: GetEntry lock failed", err).Log()
	}
	return entryCopy, exists
}

// GetAllEntries retrieves all cache entries
func (c *ActivityCache) GetAllEntries() map[string]*ActivityCacheEntry {
	var result map[string]*ActivityCacheEntry
	if err := concurrency.RunInRLockWithLogger(
		&c.mu, LockNameActivityCacheGetAllEntries, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Return a copy to avoid race conditions
			result = make(map[string]*ActivityCacheEntry, len(c.entries))
			for jobID, entry := range c.entries {
				entryCopy := *entry
				result[jobID] = &entryCopy
			}
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ActivityCache: GetAllEntries lock failed", err).Log()
	}
	return result
}

// GetEntriesForJobs retrieves entries for specific job IDs
func (c *ActivityCache) GetEntriesForJobs(jobIDs []string) map[string]*ActivityCacheEntry {
	var result map[string]*ActivityCacheEntry
	if err := concurrency.RunInRLockWithLogger(
		&c.mu, LockNameActivityCacheGetEntriesForJobs, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			result = make(map[string]*ActivityCacheEntry)
			for _, jobID := range jobIDs {
				if entry, exists := c.entries[jobID]; exists {
					entryCopy := *entry
					result[jobID] = &entryCopy
				}
			}
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("ActivityCache: GetEntriesForJobs lock failed", err).Log()
	}
	return result
}
