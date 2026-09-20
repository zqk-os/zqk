package storage

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/concurrency"
)

// AsyncCacheValidationStrategy implements asynchronous file existence validation using A/B caching.
//
// This strategy maintains two caches of existing file hashes:
//   - Active cache: Used for validation lookups (read-only during validation)
//   - Background cache: Being built/updated by the scanner goroutine
//
// When a scan completes, the caches are swapped atomically. This minimizes lock contention
// since readers only need a read lock on the active cache, while the scanner only needs
// a write lock during the brief swap operation.
//
// The scanner monitors the directory mtime to detect changes and triggers rescans as needed.
//
// Fallback behavior: If the cache is empty (initial startup or after errors), the strategy
// falls back to synchronous validation to ensure correctness.
//
// See [REDACTED-ID] for the design rationale.
type AsyncCacheValidationStrategy struct {
	// Configuration
	scanInterval time.Duration // How often to check for changes
	kindDir      string        // Directory being monitored

	// A/B cache - uses atomic pointer for lock-free reads in hot path
	activeCache atomic.Pointer[fileExistenceCache]
	buildCache  *fileExistenceCache // Only accessed by scanner goroutine

	// Scanner state
	scannerMu      sync.Mutex // Protects scanner lifecycle
	scannerRunning atomic.Bool
	stopCh         chan struct{}
	doneCh         chan struct{}

	// Directory monitoring
	lastMtime atomic.Int64 // Unix nano timestamp of last observed mtime

	// Metrics
	metrics          *ValidationMetrics
	cacheHits        atomic.Int64
	cacheMisses      atomic.Int64
	fallbackToSync   atomic.Int64
	scanCount        atomic.Int64
	swapCount        atomic.Int64
	lastScanDuration atomic.Int64 // nanoseconds
}

// fileExistenceCache holds a set of existing file hashes for a directory.
// The cache maps hash -> true for files that exist.
type fileExistenceCache struct {
	// hashes contains all hash values (without .yaml extension) that exist in the directory
	hashes map[string]struct{}
	// bucketHashes maps bucketKey -> hash -> struct{} for bucketed files
	bucketHashes map[string]map[string]struct{}
	// buildTime is when this cache was built
	buildTime time.Time
	// fileCount is the total number of files in this cache
	fileCount int
}

// newFileExistenceCache creates a new empty cache.
func newFileExistenceCache() *fileExistenceCache {
	return &fileExistenceCache{
		hashes:       make(map[string]struct{}),
		bucketHashes: make(map[string]map[string]struct{}),
		buildTime:    time.Now(),
	}
}

// AsyncCacheConfig configures the async cache strategy.
type AsyncCacheConfig struct {
	// ScanInterval is how often to check for directory changes. Default: 1 second.
	ScanInterval time.Duration
}

// DefaultAsyncCacheConfig returns sensible defaults.
func DefaultAsyncCacheConfig() *AsyncCacheConfig {
	return &AsyncCacheConfig{
		ScanInterval: time.Second,
	}
}

// NewAsyncCacheValidationStrategy creates a new async cache validation strategy.
func NewAsyncCacheValidationStrategy(config *AsyncCacheConfig) *AsyncCacheValidationStrategy {
	if config == nil {
		config = DefaultAsyncCacheConfig()
	}

	s := &AsyncCacheValidationStrategy{
		scanInterval: config.ScanInterval,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
		metrics:      &ValidationMetrics{},
	}

	// Initialize with empty cache
	emptyCache := newFileExistenceCache()
	s.activeCache.Store(emptyCache)
	s.buildCache = newFileExistenceCache()

	return s
}

// ValidateMappings validates mappings using the cached file existence data.
// If the cache is empty (not yet built), falls back to synchronous validation.
func (s *AsyncCacheValidationStrategy) ValidateMappings(kindDir string, mappings map[string]string, bucketKeys map[string]string) (validMappings map[string]string, validBucketKeys map[string]string, staleCount int) {
	if len(mappings) == 0 {
		return mappings, bucketKeys, 0
	}

	// Get current active cache (atomic load, no lock needed)
	cache := s.activeCache.Load()

	// Fallback to sync if cache is empty (initial startup or error recovery)
	if cache == nil || cache.fileCount == 0 {
		s.fallbackToSync.Add(1)
		return s.validateSync(kindDir, mappings, bucketKeys)
	}

	// Pre-allocate with same capacity
	validMappings = make(map[string]string, len(mappings))
	if bucketKeys != nil {
		validBucketKeys = make(map[string]string, len(bucketKeys))
	}

	for objectID, hash := range mappings {
		var bucketKey string
		if bucketKeys != nil {
			bucketKey = bucketKeys[objectID]
		}

		exists := s.hashExistsInCache(cache, hash, bucketKey)
		if exists {
			s.cacheHits.Add(1)
		} else {
			// Cache lag after concurrent create is common (scan interval 500ms). A miss is
			// not proof the hash file is gone — Stat before dropping or we strip brand-new
			// mappings, save an index without them, and Create returns success with a ghost.
			// TRACK: REQ-CEF-FRIC-001 — async validate must not drop fresh CAS files.
			s.cacheMisses.Add(1)
			hashFile := buildHashFilePath(kindDir, hash, bucketKey)
			if _, err := fileutil.Stat(hashFile); err == nil {
				exists = true
			}
		}
		if exists {
			validMappings[objectID] = hash
			if bucketKeys != nil && bucketKey != emptyValue {
				validBucketKeys[objectID] = bucketKey
			}
		} else {
			staleCount++
		}
	}

	return validMappings, validBucketKeys, staleCount
}

// hashExistsInCache checks if a hash exists in the cache.
func (s *AsyncCacheValidationStrategy) hashExistsInCache(cache *fileExistenceCache, hash, bucketKey string) bool {
	if bucketKey != emptyValue {
		// Check bucketed hashes
		if bucketMap, ok := cache.bucketHashes[bucketKey]; ok {
			_, exists := bucketMap[hash]
			return exists
		}
		return false
	}
	// Check non-bucketed hashes
	_, exists := cache.hashes[hash]
	return exists
}

// validateSync performs synchronous validation as a fallback.
func (s *AsyncCacheValidationStrategy) validateSync(kindDir string, mappings map[string]string, bucketKeys map[string]string) (validMappings map[string]string, validBucketKeys map[string]string, staleCount int) {
	validMappings = make(map[string]string, len(mappings))
	if bucketKeys != nil {
		validBucketKeys = make(map[string]string, len(bucketKeys))
	}

	for objectID, hash := range mappings {
		var bucketKey string
		if bucketKeys != nil {
			bucketKey = bucketKeys[objectID]
		}

		hashFile := buildHashFilePath(kindDir, hash, bucketKey)
		if _, err := fileutil.Stat(hashFile); err == nil {
			validMappings[objectID] = hash
			if bucketKeys != nil && bucketKey != emptyValue {
				validBucketKeys[objectID] = bucketKey
			}
		} else {
			staleCount++
		}
	}

	return validMappings, validBucketKeys, staleCount
}

// Start begins the background scanner goroutine.
func (s *AsyncCacheValidationStrategy) Start(kindDir string) error {
	var err error
	if err_swallow := concurrency.RunInLock(&s.scannerMu, func() error {
		if s.scannerRunning.Load() {
			return nil // Already running
		}

		s.kindDir = kindDir
		s.stopCh = make(chan struct{})
		s.doneCh = make(chan struct{})
		s.scannerRunning.Store(true)

		// Do initial scan synchronously to populate cache before returning
		if scanErr := s.performScan(); scanErr != nil {
			// Log but don't fail - will fallback to sync validation
			err = scanErr
		}

		// Start background scanner
		goroutinelabels.NewGoroutine(ConstMiscValidationStrategyScanner, ConstMiscAsyncValidationScannerLoop).
			StartWithContext(context.Background(), func(ctx context.Context) error { // Background: request-or-shutdown derived
				s.scannerLoop()
				return nil
			})
		return nil
	}); err_swallow != nil {
		logging.LogSwallowedError(err_swallow)
	}
	return err
}

func (s *AsyncCacheValidationStrategy) Stop() error {
	if err_swallow := concurrency.RunInLock(&s.scannerMu, func() error {
		if !s.scannerRunning.Load() {
			return nil // Not running
		}

		close(s.stopCh)
		s.scannerRunning.Store(false)
		return nil
	}); err_swallow != nil {
		logging.LogSwallowedError(err_swallow)
	}

	select {
	case <-s.doneCh:
	case <-time.After(5 * time.Second):
		// Timeout waiting for scanner to stop
	}

	return nil
}

// Name returns the strategy name.
func (s *AsyncCacheValidationStrategy) Name() string {
	return "async-cache"
}

// GetMetrics returns the metrics for this strategy instance.
func (s *AsyncCacheValidationStrategy) GetMetrics() *ValidationMetrics {
	return s.metrics
}

// GetCacheStats returns cache performance statistics.
func (s *AsyncCacheValidationStrategy) GetCacheStats() (hits, misses, fallbacks, scans, swaps int64, lastScanDurationNs int64) {
	return s.cacheHits.Load(),
		s.cacheMisses.Load(),
		s.fallbackToSync.Load(),
		s.scanCount.Load(),
		s.swapCount.Load(),
		s.lastScanDuration.Load()
}

// scannerLoop runs the background scanning loop.
func (s *AsyncCacheValidationStrategy) scannerLoop() {
	defer close(s.doneCh)

	ticker := time.NewTicker(s.scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			// Check if directory has changed
			if s.hasDirectoryChanged() {
				var _err_84125051 = s.performScan()
				if _err_84125051 !=

					// hasDirectoryChanged checks if the directory mtime has changed since last scan.
					nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84125051).Log()
				}
			}
		}
	}
}

func (s *AsyncCacheValidationStrategy) hasDirectoryChanged() bool {
	info, err := fileutil.Stat(s.kindDir)
	if err != nil {
		return false
	}

	currentMtime := info.ModTime().UnixNano()
	lastMtime := s.lastMtime.Load()

	if currentMtime != lastMtime {
		s.lastMtime.Store(currentMtime)
		return true
	}

	return false
}

// performScan scans the directory and builds a new cache, then swaps it with the active cache.
func (s *AsyncCacheValidationStrategy) performScan() error {
	startTime := time.Now()
	s.scanCount.Add(1)

	// Build new cache
	newCache := newFileExistenceCache()

	// Scan the kind directory
	err := s.scanDirectory(s.kindDir, "", newCache)
	if err != nil {
		return err
	}

	newCache.buildTime = time.Now()

	// Atomic swap: store new cache as active
	s.activeCache.Store(newCache)
	s.swapCount.Add(1)

	duration := time.Since(startTime)
	s.lastScanDuration.Store(int64(duration))

	return nil
}

// scanDirectory recursively scans a directory for .yaml files.
func (s *AsyncCacheValidationStrategy) scanDirectory(dir, bucketKey string, cache *fileExistenceCache) error {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil // Directory doesn't exist yet - that's OK
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// This is a bucket subdirectory
			subBucketKey := entry.Name()
			subDir := filepath.Join(dir, subBucketKey)
			if err := s.scanDirectory(subDir, subBucketKey, cache); err != nil {
				// Log but continue scanning other directories
				continue
			}
		} else if filepath.Ext(entry.Name()) == ".yaml" {
			// This is a hash file
			hash := entry.Name()[:len(entry.Name())-5] // Remove .yaml extension

			if bucketKey != emptyValue {
				// Bucketed file
				if cache.bucketHashes[bucketKey] == nil {
					cache.bucketHashes[bucketKey] = make(map[string]struct{})
				}
				cache.bucketHashes[bucketKey][hash] = struct{}{}
			} else {
				// Non-bucketed file
				cache.hashes[hash] = struct{}{}
			}
			cache.fileCount++
		}
	}

	return nil
}
