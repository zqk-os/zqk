package system

import (
	stdcontext "context"
	"runtime"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// fileStorageDiscovery is the interface used for strategy-based discovery (avoids importing storage in tests).
type fileStorageDiscovery interface {
	ListPathsForDiscovery(ctx stdcontext.Context, kind string) ([]storage.PathWithID, error)
}

// calculateMaxConcurrentWorkers calculates the maximum number of concurrent workers
func calculateMaxConcurrentWorkers(numKinds int) int {
	maxConcurrent := runtime.NumCPU() * 2
	if maxConcurrent > numKinds {
		maxConcurrent = numKinds
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return maxConcurrent
}

// processKindForDiscovery processes a single kind for object discovery
// Note: Goroutine label should be set by the caller using NewGoroutine()
// Note: Semaphore is managed by the caller via WithPreCleanup()
func processKindForDiscovery(projectRoot, kind string, targetIDs []string, logger logging.Logger, filesChan chan<- []scannedFile, sem chan struct{}, ctx stdcontext.Context, storageProvider storage.ObjectStorageProvider, profile string) error {
	// Check context cancellation before starting
	select {
	case <-ctx.Done():
		logging.Fluent(logger).Debug("Discovery cancelled before starting").
			Kind(kind).
			WithError(ctx.Err()).
			Log()
		// Emit cancellation event via coordinator
		emitDiscoveryCancellationEventViaCoordinator(
			ctx, projectRoot, storageProvider, kind,
			"before_start", 0, ctx.Err(), profile)
		return ctx.Err()
	default:
	}

	kindDir := getKindDirectory(projectRoot, kind)
	if kindDir == emptyValue {
		return nil // Not an error, just skip this kind
	}

	kindStartTime := time.Now()
	var files []scannedFile
	var err error
	// Prefer storage's strategy-based discovery when using file backend (uses spec bucketing).
	if fileStorage, ok := storageProvider.(fileStorageDiscovery); ok {
		paths, listErr := fileStorage.ListPathsForDiscovery(ctx, kind)
		if listErr == nil {
			files = make([]scannedFile, 0, len(paths))
			for _, p := range paths {
				if storage.IsObjectDraftPlanePath(projectRoot, p.Path) {
					continue
				}
				files = append(files, scannedFile{ObjectID: p.ID, Kind: kind, Path: p.Path})
			}
		} else {
			logging.Fluent(logger).Debug("Storage discovery failed, falling back to directory scan").
				Kind(kind).
				WithError(listErr).
				Log()
		}
	}
	if files == nil {
		files, err = scanObjectFilesWithContext(ctx, kindDir, kind, logger, storageProvider)
	}
	kindDuration := time.Since(kindStartTime)
	if err != nil {
		logging.Fluent(logger).Debug("Error scanning object files").
			Kind(kind).
			WithError(err).
			String("duration", kindDuration.String()).
			Log()
		return nil // Log but don't fail - continue with other kinds
	}

	// Log slow kind discovery for observability
	if kindDuration > 10*time.Second {
		logging.Fluent(logger).Info("Kind discovery took longer than expected").
			Kind(kind).
			Int("files_found", len(files)).
			String("duration", kindDuration.String()).
			Log()
	}

	// Filter by IDs if specified
	if len(targetIDs) > 0 {
		files = filterFilesByIDs(files, targetIDs)
		// Fallback for CAS kinds: index may be empty so ListPathsForDiscovery returns 0 paths.
		// Resolve each target ID via GetFilePathForObject (same path as "object get" — index + findCASFilePathByScanning).
		if len(files) == 0 {
			if fileStorage := extractFileStorage(storageProvider); fileStorage != nil {
				for _, id := range targetIDs {
					path, err := fileStorage.GetFilePathForObject(id, kind)
					if err == nil && path != emptyValue {
						if storage.IsObjectDraftPlanePath(projectRoot, path) {
							continue
						}
						files = append(files, scannedFile{ObjectID: id, Kind: kind, Path: path})
					}
				}
			}
		}
	}

	if len(files) > 0 {
		select {
		case filesChan <- files:
		case <-ctx.Done():
			logging.Fluent(logger).Debug("Discovery cancelled before sending results").
				Kind(kind).
				Int("files_count", len(files)).
				WithError(ctx.Err()).
				Log()
			// Emit cancellation event via coordinator
			emitDiscoveryCancellationEventViaCoordinator(
				ctx, projectRoot, storageProvider, kind,
				"before_send_results", len(files), ctx.Err(), profile)
			return ctx.Err()
		}
	}

	return nil
}

// filterFilesByIDs filters files by target IDs
func filterFilesByIDs(files []scannedFile, targetIDs []string) []scannedFile {
	filtered := make([]scannedFile, 0, len(files))
	for _, file := range files {
		for _, id := range targetIDs {
			if file.ObjectID == id {
				filtered = append(filtered, file)
				break
			}
		}
	}
	return filtered
}

// collectDiscoveryResults collects results from the files channel
func collectDiscoveryResults(filesChan <-chan []scannedFile) []scannedFile {
	var allFiles []scannedFile
	for files := range filesChan {
		allFiles = append(allFiles, files...)
	}
	return allFiles
}

// deduplicateFilesByObjectID removes duplicate files that have the same object ID
// When multiple files have the same object ID, we keep the first one encountered
// This prevents processing the same object multiple times, which was causing
// massive duplicate work (e.g., AUD-25803 appearing 682 times)
func deduplicateFilesByObjectID(files []scannedFile, logger logging.Logger) []scannedFile {
	if len(files) == 0 {
		return files
	}

	// Map to track which object IDs we've seen
	seen := make(map[string]bool)
	deduplicated := make([]scannedFile, 0, len(files))
	duplicateCount := 0

	for _, file := range files {
		if seen[file.ObjectID] {
			duplicateCount++
			// Skip duplicate - no need to log each one individually
			// Summary is logged below
			continue
		}
		seen[file.ObjectID] = true
		deduplicated = append(deduplicated, file)
	}

	if duplicateCount > 0 {
		logging.Fluent(logger).Info("Deduplicated files by object ID").
			Int("original_count", len(files)).
			Int("deduplicated_count", len(deduplicated)).
			Int("duplicates_removed", duplicateCount).
			Log()
	}

	return deduplicated
}
