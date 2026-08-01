package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/appledouble"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func (f *FileObjectStorage) extractTimeRangeFromFilters(filters map[string]any) *timeRange {
	createdAtFilter, ok := filters[objects.FieldKeyCreatedAt].(map[string]any)
	if !ok {
		return nil
	}

	var start, end time.Time
	var hasRange bool

	if gte := objects.GetString(createdAtFilter, "$gte"); gte != "" {
		if t, err := time.Parse(time.RFC3339, gte); err == nil {
			start = t
			hasRange = true
		}
	}
	if lte := objects.GetString(createdAtFilter, "$lte"); lte != "" {
		if t, err := time.Parse(time.RFC3339, lte); err == nil {
			end = t
			hasRange = true
		}
	}

	if !hasRange {
		return nil
	}

	return &timeRange{start: start, end: end}
}

// walkBucketedStorageByDateRange walks only date subdirectories within the time range
// This is much faster than walking all subdirectories
func (f *FileObjectStorage) walkBucketedStorageByDateRange(ctx context.Context, kindDir string, start, end time.Time) []string {
	eventLogger := logging.NewEventLogger(ctx)
	var filePaths []string

	// Generate list of date subdirectories to check (YYYY-MM format)
	// Start from the month of start time, end at the month of end time
	current := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	endMonth := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location())

	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionWalkingBucketedDateRangeDebug).
		String("start", start.Format("2006-01")).
		String("end", end.Format("2006-01")).
		Log()

	for !current.After(endMonth) {
		// Check context cancellation
		select {
		case <-ctx.Done():
			StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionContextCancelledWalkDebug).Log()
			return filePaths
		default:
		}

		// Format as YYYY-MM
		dateDir := current.Format("2006-01")
		subDir := filepath.Join(kindDir, dateDir)

		// Check if subdirectory exists
		if entries, err := os.ReadDir(subDir); err == nil {
			fileCount := 0
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				config := GetStorageConfig()
				if strings.HasSuffix(entry.Name(), config.YAMLExtension) || strings.HasSuffix(entry.Name(), config.YAMLAltExtension) {
					filePaths = append(filePaths, filepath.Join(subDir, entry.Name()))
					fileCount++
				}
			}
			if fileCount > 0 {
				StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionFoundFilesDateSubdirDebug).
					String("date_dir", dateDir).
					Int("file_count", fileCount).
					Log()
			}
		}

		// Move to next month
		current = current.AddDate(0, 1, 0)
	}

	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionCompletedDateRangeWalkDebug).
		Int("total_files", len(filePaths)).
		Log()
	return filePaths
}

// collectFilePathsWithStrategy collects all YAML file paths from a kind directory
// Uses the bucketing strategy to understand where files are stored
// If timeRange is provided, only walks date subdirectories within that range
func (f *FileObjectStorage) collectFilePathsWithStrategy(ctx context.Context, kind string, kindDir string, bucketStrategy BucketStrategy, timeRange *timeRange, eventLogger *logging.EventLogger) ([]string, error) {
	// Stream-backed kinds: source of truth is the stream registry and segments, not individual YAML files.
	// Optimization: Skip directory walking entirely for high-volume stream kinds to avoid CLI timeouts.
	if StreamStorageEnabledForKind(kind) {
		return nil, nil
	}

	var filePaths []string

	// Create event logger if not provided
	if eventLogger == nil {
		eventLogger = logging.NewEventLogger(ctx)
	}

	// If we have a bucketing strategy, use it to understand storage structure
	if bucketStrategy != nil {
		// Strategy defines how files are organized - walk directories based on strategy
		return f.collectFilePathsUsingStrategy(ctx, kind, kindDir, bucketStrategy, timeRange, eventLogger)
	}

	// Fallback: Check if directory has date-based subdirectories (legacy detection)
	hasDateDirs := f.hasDateSubdirectories(kindDir)

	// Fallback to legacy detection if no strategy
	if hasDateDirs {
		// Bucketed storage - use optimized traversal
		if timeRange != nil {
			// Optimized: only walk date subdirectories within time range
			StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionOptimizedDateRangeWalkDebug).
				String("start", zqktime.FormatRFC3339UTC(timeRange.start)).
				String("end", zqktime.FormatRFC3339UTC(timeRange.end)).
				Log()
			filePaths = f.walkBucketedStorageByDateRange(ctx, kindDir, timeRange.start, timeRange.end)
		} else {
			// Walk all subdirectories recursively
			StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionWalkingAllSubdirsDebug).
				String("kind_dir", kindDir).
				Log()
			err := filepath.Walk(kindDir, func(path string, info os.FileInfo, err error) error {
				// Check context cancellation
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				if err != nil {
					if eventLogger != nil {
						StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionWalkErrorSkippingDebug).
							String("path", path).
							WithError(err).
							Log()
					}
					return nil // Skip errors, continue walking
				}
				if info == nil || info.IsDir() {
					return nil
				}
				if appledouble.SkipPathInTreeWalk(path) {
					return nil
				}
				config := GetStorageConfig()
				if strings.HasSuffix(info.Name(), config.YAMLExtension) || strings.HasSuffix(info.Name(), config.YAMLAltExtension) {
					filePaths = append(filePaths, path)
					if eventLogger != nil && len(filePaths) <= 5 {
						StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionFoundYAMLFileDebug).
							String("path", path).
							Log()
					}
				}
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				return nil, errfmt.Newf(ConstStreamFailedToWalkBucketedDirectory).Wrap(err)
			}
		}
	} else {
		// Flat storage - read files directly from kind directory
		entries, err := os.ReadDir(kindDir)
		if err != nil {
			if os.IsNotExist(err) {
				return []string{}, nil
			}
			return nil, errfmt.Newf(ConstStreamFailedToReadDirectory).Wrap(err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if appledouble.SkipNameInReadDir(entry.Name()) {
				continue
			}
			config := GetStorageConfig()
			if !strings.HasSuffix(entry.Name(), config.YAMLExtension) && !strings.HasSuffix(entry.Name(), config.YAMLAltExtension) {
				continue
			}
			filePaths = append(filePaths, filepath.Join(kindDir, entry.Name()))
		}
	}

	return filePaths, nil
}

// EnsureBucketStrategyRegistryReady initializes the bucket strategy registry if not yet done.
// Call this after storage is fully created (e.g. in OnStorageCreated) so the first Create does not pay.
// Must not be called from inside NewFileObjectStorage (would cause recursion via LoadAllStrategies -> NewStorageFactory -> NewFileObjectStorage).
func (f *FileObjectStorage) EnsureBucketStrategyRegistryReady(ctx context.Context) {
	registry := f.getBucketStrategyRegistry(ctx)
	_ = registry
}

// getBucketStrategyRegistry gets or initializes the bucket strategy registry
// This is lazy-loaded to avoid circular dependencies during FileObjectStorage initialization
func (f *FileObjectStorage) getBucketStrategyRegistry(ctx context.Context) *DefaultBucketStrategyRegistry {
	// Skip initialization if we're already in the process of initializing
	// This prevents circular dependencies when listing bucketing_strategy objects
	if IsBucketStrategyRegistryInitializing(ctx) {
		return nil
	}

	// Initialize once (thread-safe, no mutex needed).
	// Use the current storage as provider to avoid recursion: NewDefaultBucketStrategyRegistry(projectRoot)
	// would call NewBucketStrategyLoader -> NewBucketStrategyStorageFactory -> NewStorageFactory ->
	// NewFileObjectStorage -> write-behind worker -> apply (re-entering getBucketStrategyRegistry).
	initStart := time.Now()
	f.bucketStrategyOnce.Do(func() {
		provider := NewExistingStorageBucketStrategyProvider(f)
		registry, err := NewDefaultBucketStrategyRegistryWithProvider(ctx, provider)
		if err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageBucketingInitRegistryFailedWarn).WithError(err).Log()
			return
		}
		f.bucketStrategyRegistry = registry
		initDuration := time.Since(initStart)
		if initDuration > 100*time.Millisecond {
			var _err_83536192 = f.trackPersistenceStep(ctx, pkgctx.NewSystemSecurityContext(), OpGetBucketStrategyRegistry, PersistenceStepInitialize, nil, map[string]any{"duration_ms": float64(initDuration.Nanoseconds()) / 1e6}, initDuration, nil)
			if _err_83536192 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

					// collectFilePathsUsingStrategy collects file paths using the bucketing strategy
					// The strategy defines where files are stored, so we use it to discover files
					ProfileSystem))).Error(ErrMsgSwallowedError,

					_err_83536192).Log()
			}
		}
	})

	return f.bucketStrategyRegistry
}

func (f *FileObjectStorage) collectFilePathsUsingStrategy(ctx context.Context, _, kindDir string, bucketStrategy BucketStrategy, timeRange *timeRange, eventLogger *logging.EventLogger) ([]string, error) {
	var filePaths []string

	// Walk the directory structure - the strategy tells us where files might be
	// For path-based strategies, files are in subdirectories
	// For chrono strategies, files are in date-based subdirectories
	err := filepath.Walk(kindDir, func(path string, info os.FileInfo, err error) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			if eventLogger != nil {
				StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionWalkErrorSkippingDebug).
					String("path", path).
					WithError(err).
					Log()
			}
			return nil // Skip errors, continue walking
		}

		if info == nil || info.IsDir() {
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		// Only collect YAML files
		config := GetStorageConfig()
		if strings.HasSuffix(info.Name(), config.YAMLExtension) || strings.HasSuffix(info.Name(), config.YAMLAltExtension) {
			// For time-based strategies with timeRange, filter by bucket
			if timeRange != nil {
				// Extract bucket key from path to check if it's in range
				// The path structure is: kindDir/bucketKey/file.yaml
				relPath, err := filepath.Rel(kindDir, filepath.Dir(path))
				if err == nil && relPath != "." {
					// Check if this bucket is in the time range
					// For chrono strategies, bucket key is a date string
					if !f.isBucketInTimeRange(relPath, timeRange) {
						return nil // Skip files outside time range
					}
				}
			}

			filePaths = append(filePaths, path)
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, errfmt.Newf(ConstStreamFailedToWalkDirectoryUsingStrategy).Wrap(err)
	}

	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListCollectionCollectedPathsStrategyDebug).
		String("strategy", bucketStrategy.Name()).
		Int("file_count", len(filePaths)).
		Log()

	return filePaths, nil
}

// isBucketInTimeRange checks if a bucket key (directory name) is within the time range
// For chrono strategies, bucket keys are date strings like "2026-01"
func (f *FileObjectStorage) isBucketInTimeRange(bucketKey string, timeRange *timeRange) bool {
	// Try to parse bucket key as date (YYYY-MM format)
	parsedTime, err := time.Parse("2006-01", bucketKey)
	if err != nil {
		// Not a date-based bucket, include it
		return true
	}

	// Check if parsed time is within range
	bucketStart := time.Date(parsedTime.Year(), parsedTime.Month(), 1, 0, 0, 0, 0, parsedTime.Location())
	bucketEnd := bucketStart.AddDate(0, 1, 0).Add(-time.Nanosecond) // End of month

	return !bucketStart.After(timeRange.end) && !bucketEnd.Before(timeRange.start)
}

// collectFilePaths collects all YAML file paths from a kind directory (backward compatibility wrapper)
// This function is kept for backward compatibility - it infers the kind from the directory
func (f *FileObjectStorage) collectFilePaths(ctx context.Context, kindDir string, timeRange *timeRange, eventLogger *logging.EventLogger) ([]string, error) {
	// Infer kind from directory name
	dirName := filepath.Base(kindDir)
	kind := objects.GetKindFromDirectory(dirName)
	if kind == emptyValue {
		kind = dirName // Fallback to directory name
	}

	// Try to get bucketing strategy
	var bucketStrategy BucketStrategy
	registry := f.getBucketStrategyRegistry(ctx)
	if registry != nil {
		strategy, err := registry.GetStrategyForKind(ctx, kind)
		if err == nil {
			bucketStrategy = strategy
		}
	}

	return f.collectFilePathsWithStrategy(ctx, kind, kindDir, bucketStrategy, timeRange, eventLogger)
}

// hasDateSubdirectories checks if a directory has date-based subdirectories (YYYY-MM pattern)
func (f *FileObjectStorage) hasDateSubdirectories(kindDir string) bool {
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return false
	}

	datePattern := regexp.MustCompile(`^\d{4}-\d{2}(-\d{2})?$`)
	for _, entry := range entries {
		if entry.IsDir() && datePattern.MatchString(entry.Name()) {
			return true
		}
	}
	return false
}

type bucketedCacheEntry struct {
	bucketed bool
	mtime    int64
}

// usesBucketedStorage checks if a kind uses bucketed directory structure
// Bucketed kinds store objects in subdirectories (e.g., audit/2025-12/, backlog/active/, etc.)
// This is backend-agnostic and works with any bucket strategy (chronological, categorical, state-based, etc.)
// Result is cached per kindDir (validated by kindDir mtime) to avoid repeated ReadDir during List/getObjectFilePath.
func (f *FileObjectStorage) usesBucketedStorage(kind, kindDir string) bool {
	mtime := int64(0)
	if info, err := os.Stat(kindDir); err == nil {
		mtime = info.ModTime().UnixNano()
	}
	if val, ok := f.bucketedCache.Load(kindDir); ok {
		if entry, ok := val.(bucketedCacheEntry); ok && entry.mtime == mtime {
			return entry.bucketed
		}
		// Directory changed since last cache: invalidate dependent caches.
		f.bucketListCache.Delete(kindDir)
		f.bucketedCache.Delete(kindDir)
	}
	// Check if directory exists and has ANY subdirectories
	// This is robust - works with any bucket strategy, not just date-based patterns
	// The actual bucket directory selection is done by the bucket strategy when creating objects
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		f.bucketedCache.Store(kindDir, bucketedCacheEntry{bucketed: false, mtime: mtime})
		return false
	}

	// Look for any subdirectories (not just date patterns) - supports any bucket strategy
	for _, entry := range entries {
		if entry.IsDir() {
			f.bucketedCache.Store(kindDir, bucketedCacheEntry{bucketed: true, mtime: mtime})
			return true
		}
	}
	f.bucketedCache.Store(kindDir, bucketedCacheEntry{bucketed: false, mtime: mtime})
	return false
}

// scanIDBasedFiles scans a directory for legacy ID-named YAML files (not hash-based).
// All objects are CAS (hash-named); this finds legacy files that may not be in the CAS index yet (migration).
// Hash-based filenames are 64-char hex; we filter those out so we only return ID-named entries.
func (f *FileObjectStorage) scanIDBasedFiles(kindDir, kind string) []string {
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return nil
	}

	var ids []string
	config := GetStorageConfig()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), config.YAMLExtension) && !strings.HasSuffix(entry.Name(), config.YAMLAltExtension) {
			continue
		}

		// Skip hash-based files (64-char hex) - uses package-level compiled regex
		if casHashFilenameRe.MatchString(entry.Name()) {
			continue
		}

		// Skip index files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Extract ID from filename
		filenameID := strings.TrimSuffix(entry.Name(), config.YAMLExtension)
		filenameID = strings.TrimSuffix(filenameID, config.YAMLAltExtension)

		// For accounts, handle account-username format
		accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
		if accountDir != emptyValue && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(filenameID, "account-") {
			username := strings.TrimPrefix(filenameID, "account-")
			filenameID = fmt.Sprintf("account:%s", username)
		}

		// Validate that this looks like an ID (not a hash)
		// IDs typically have a prefix and numbers (e.g., ITEM-001, TAM-100)
		if filenameID != emptyValue {
			ids = append(ids, filenameID)
		}
	}

	return ids
}
