package storage

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// casHashFilenameRe matches content-addressed filenames (64 hex chars + .yaml)
var casHashFilenameRe = regexp.MustCompile(`^[a-f0-9]{64}\.yaml$`)

// usesContentAddressableStorage checks if a kind uses content-addressable (hash-named) storage.
// All kinds are CAS-based; there is no ID-based storage. Objects are always stored with hash filenames.
//
//nolint:unparam // Always returns true - all kinds use CAS
func (f *FileObjectStorage) usesContentAddressableStorage(kind string) bool {
	return true
}

// ClearCASCaches clears the CAS caches. Used in tests where the same FileObjectStorage
// instance checks files created by a different process.
func (f *FileObjectStorage) ClearCASCaches() {
	if f.casCache != nil {
		f.casCache.Clear()
	}
}

// InvalidateCASCacheForKind clears the CAS cache for a specific kind. Used to keep the CAS index cache fresh
// when objects are created, updated, or deleted, preventing stale index reads.
func (f *FileObjectStorage) InvalidateCASCacheForKind(kind string) {
	if f.casCache != nil {
		f.casCache.Delete(kind)
	}
}

// GetContentAddressableStorage gets or creates a ContentAddressableStorage instance for a kind (exported for auto-fix)
func (f *FileObjectStorage) GetContentAddressableStorage(kind string) (*ContentAddressableStorage, error) {
	return f.getContentAddressableStorage(kind)
}

// getContentAddressableStorage gets or creates a ContentAddressableStorage instance for a kind
func (f *FileObjectStorage) getContentAddressableStorage(kind string) (*ContentAddressableStorage, error) {
	// Check if kind uses content-addressable storage
	if !f.usesContentAddressableStorage(kind) {
		return nil, errfmt.Errorf(ConstStreamKindStrDoesNotUseContentAddressableStorage, kind)
	}

	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return nil, errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}

	// Get write queue for this project root (from factory or global singleton)
	writeQueue := GetListingIndexWriteQueueForProjectRoot(f.projectRoot)
	if writeQueue != GetGlobalListingIndexWriteQueue() {
		// Dedicated queue (e.g. from per-project-root factory); set its context
		writeQueue.SetProjectRoot(f.projectRoot)
		writeQueue.SetStorage(f)
	} else {
		// Global queue: only set if not already set for a different project root
		existingWriteProjectRoot := writeQueue.GetProjectRoot()
		existingWriteStorage := writeQueue.GetStorage()
		if existingWriteStorage == nil || existingWriteProjectRoot == f.projectRoot {
			writeQueue.SetProjectRoot(f.projectRoot)
			writeQueue.SetStorage(f)
		} else {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			msg := "Skipping SetProjectRoot/SetStorage on global CAS index write queue - different project root already set"
			if IsTestOrTempProjectRoot(f.projectRoot) {
				StorageLog(logger).Debug(msg).
					String(ConstStreamExistingProjectRoot, existingWriteProjectRoot).
					String(ConstStreamNewProjectRoot, f.projectRoot).
					Log()
			} else {
				StorageLog(logger).Warn(msg).
					String(ConstStreamExistingProjectRoot, existingWriteProjectRoot).
					String(ConstStreamNewProjectRoot, f.projectRoot).
					Log()
			}
		}
	}

	// Get or create CAS instance using ResourceCache abstraction (thread-safe)
	// Use system context for CAS cache operations
	ctx := pkgctx.NewSystemContext()
	cas, err := f.casCache.GetOrCreate(ctx, kind, func(ctx context.Context, key string) (*ContentAddressableStorage, error) {
		return NewContentAddressableStorage(kindDir, key, writeQueue), nil
	})
	if err != nil {
		return nil, errfmt.Errorf(ConstStreamFailedToCreateCasInstanceForKindStrErr, kind, err)
	}

	// Set up default orphan cleanup callback to queue old hash files for cleanup
	// NOTE: This is file-backend specific - CAS queues are only used for file backend
	// This prevents orphaned files when objects are updated with new content (new hash)
	// Cleanup is queued (not immediate) to avoid blocking CAS updates and provide flexibility
	var cleanupQueue *CASOrphanCleanupQueue
	if f.orphanCleanupQueue != nil {
		cleanupQueue = f.orphanCleanupQueue
		// Injected queue (test isolation): already configured by factory
	} else {
		cleanupQueue = GetGlobalOrphanCleanupQueue()
		// Configure global queue with project root and storage for audit events
		// CRITICAL: Only set if queue doesn't already have storage with a different project root
		existingCleanupProjectRoot := cleanupQueue.GetProjectRoot()
		existingCleanupStorage := cleanupQueue.GetStorage()
		if existingCleanupStorage == nil || existingCleanupProjectRoot == f.projectRoot {
			cleanupQueue.SetProjectRoot(f.projectRoot)
			cleanupQueue.SetStorage(f)
		} else {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			msg := "Skipping SetProjectRoot/SetStorage on global CAS orphan cleanup queue - different project root already set"
			if IsTestOrTempProjectRoot(f.projectRoot) {
				StorageLog(logger).Debug(msg).
					String(ConstStreamExistingProjectRoot, existingCleanupProjectRoot).
					String(ConstStreamNewProjectRoot, f.projectRoot).
					Log()
			} else {
				StorageLog(logger).Warn(msg).
					String(ConstStreamExistingProjectRoot, existingCleanupProjectRoot).
					String(ConstStreamNewProjectRoot, f.projectRoot).
					Log()
			}
		}
	}

	cas.SetOrphanCleanupCallback(func(oldHashFilePath string) error {
		// Queue the cleanup operation instead of executing immediately
		// This provides:
		// - Non-blocking updates (doesn't slow down CAS operations)
		// - Batching (multiple cleanups processed together)
		// - Retry capability (failed cleanups can be retried)
		// - Flexibility (can defer cleanup under load)
		// - Worker lifecycle management (shuts down when idle, wakes on work)
		if err := cleanupQueue.EnqueueCleanup(oldHashFilePath); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectCASEnqueueOrphanCleanupFailedWarn).
				String("file_path", oldHashFilePath).
				WithError(err).
				Log()
			// Synchronous fallback: same rename-then-delete as the queue worker so we never
			// rely solely on the channel when it is full or shutdown is in progress.
			if rmErr := removeOrphanCASHashFileSync(oldHashFilePath); rmErr != nil {
				StorageLog(logger).Warn(LogEventStorageObjectCASOrphanCleanupSyncFailedWarn).
					String("file_path", oldHashFilePath).
					WithError(rmErr).
					Log()
			}
		}
		return nil // Always return nil - cleanup is best-effort and queued
	})

	// Set post-sync callback to register object ID and filename in cache after sync succeeds
	// This keeps the object ID and file path coupled together, ensuring cache has correct hash-based filename
	cas.SetPostSyncCallback(func(objectID, casKind, hash, filePath string) error {
		// Get cache operation handler if registered
		if cacheOperationHandler == nil {
			return nil // No handler registered - cache operations are optional
		}

		// Register object ID and filename together in cache (no synchronization needed)
		cacheCtx := &pkgctx.CacheContext{
			Operation: pkgctx.CacheOperationUpdate,
			NewID:     objectID,
			Kind:      casKind,
			FilePath:  filePath,
		}

		// Call handler to register in cache (object ID and filename stay coupled)
		if err := cacheOperationHandler(cacheCtx); err != nil {
			// Log warning but don't fail - cache updates are best effort
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectCASRegisterCachePostSyncFailedWarn).
				ObjectID(objectID).
				Kind(casKind).
				String("file_path", filePath).
				WithError(err).
				Log()
		}

		return nil // Always return nil - cache registration is best effort
	})

	// CAS instance already cached in ResourceCache during GetOrCreate() above.
	// Write queue project root and storage were set above when we obtained the queue.

	return cas, nil
}

// casHashFilePeekContainsObjectID returns true if path is a readable hash-named YAML whose content
// includes an embedded id field matching objectID. Used when the CAS index misses an entry
// (discovery scan). Search the full file: long YAML (e.g. convergence_session with a large
// activity_log) can place id: deep in the file, which previously broke discovery when limited to 16KB.
func casHashFilePeekContainsObjectID(path, objectID string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer size to handle very long lines (e.g. 10MB test fields)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	target1 := []byte("id: " + objectID)
	target2 := []byte("id: \"" + objectID + "\"")
	target3 := []byte("id: '" + objectID + "'")

	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.Contains(line, target1) || bytes.Contains(line, target2) || bytes.Contains(line, target3) {
			return true
		}
	}
	return false
}

// removeOrphanCASFilesForObjectID deletes every hash-named YAML under kindDir and one level of bucket
// subdirs whose embedded id matches objectID. After an Update, an older content hash file can remain
// on disk while the index points at the new hash; Delete removes only the indexed file. Without this
// pass, Read's CAS discovery would re-index the orphan and resurrect the object (stale content).
func removeOrphanCASFilesForObjectID(objectID, kindDir string) {
	if objectID == emptyValue || kindDir == emptyValue {
		return
	}
	removeMatchesInDir := func(dir string) {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !casHashFilenameRe.MatchString(name) {
				continue
			}
			path := filepath.Join(dir, name)
			if casHashFilePeekContainsObjectID(path, objectID) {
				var _err_83295028 = os.Remove(path)
				if _err_83295028 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83295028).Log()
				}
			}
		}
	}
	removeMatchesInDir(kindDir)
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		removeMatchesInDir(filepath.Join(kindDir, e.Name()))
	}
}

// discoverCASFilePathByScanning scans the kind directory for hash-named files whose "id" field
// matches the given objectID. Used when the CAS index is missing an entry (e.g. object
// created outside this process or index not yet written). Returns path and hash (basename
// without .yaml) if found, so the caller can update the index.
func discoverCASFilePathByScanning(objectID, kindDir string) (filePath, hash string, err error) {
	scanDir := func(dir string) (string, string, bool) {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			return "", "", false
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !casHashFilenameRe.MatchString(name) {
				continue
			}
			path := filepath.Join(dir, name)
			if casHashFilePeekContainsObjectID(path, objectID) {
				hash := strings.TrimSuffix(name, filepath.Ext(name))
				return path, hash, true
			}
		}
		return "", "", false
	}
	if path, hash, ok := scanDir(kindDir); ok {
		return path, hash, nil
	}
	// Bucketed: scan subdirs (e.g. YYYY-MM)
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return "", "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(kindDir, e.Name())
		if path, hash, ok := scanDir(subDir); ok {
			return path, hash, nil
		}
	}
	return "", "", errfmt.Errorf(ConstStreamIdStrNotFoundInCasDirectoryStr, objectID, kindDir)
}

// findCASFilePathByScanning delegates to discoverCASFilePathByScanning (FileObjectStorage receiver for call sites).
func (f *FileObjectStorage) findCASFilePathByScanning(objectID, kindDir string) (filePath, hash string, err error) {
	return discoverCASFilePathByScanning(objectID, kindDir)
}

// WriteObjectRaw writes a raw object using the correct CAS/Stream settings, resolving bucket key/streams automatically.
func (f *FileObjectStorage) WriteObjectRaw(ctx context.Context, kind, id string, data []byte) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	if StreamStorageEnabledForKind(kind) {
		return f.writeObjectToStream(ctx, id, kind, data)
	}

	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return errfmt.Errorf("unknown object kind: %s", kind)
	}
	filePath := filepath.Join(kindDir, id+".yaml")
	return f.writeObjectToCAS(ctx, id, kind, filePath, data, secCtx)
}
