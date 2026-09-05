package storage

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DeferredHashManager manages deferred hash updates for objects
// Hashes are only computed and updated after all pending operations on an object are complete
type DeferredHashManager struct {
	storage         ObjectStorageProvider
	pendingOps      map[string]*PendingObjectOps // objectID -> pending operations
	mu              sync.RWMutex
	logger          *logging.EventLogger
	updateInterval  time.Duration // How often to check for objects ready for hash update
	cleanupInterval time.Duration // How often to cleanup old completed operations

	registeredTotal atomic.Int64
	processedTotal  atomic.Int64
}

// PendingObjectOps tracks pending operations for an object
type PendingObjectOps struct {
	ObjectID   string
	ObjectKind string
	FilePath   string
	Operations []string // Operation IDs that affect this object
	LastUpdate time.Time
	mu         sync.RWMutex
}

// NewDeferredHashManager creates a new deferred hash manager
func NewDeferredHashManager(storage ObjectStorageProvider) *DeferredHashManager {
	return &DeferredHashManager{
		storage:         storage,
		pendingOps:      make(map[string]*PendingObjectOps),
		logger:          logging.NewEventLogger(pkgctx.NewSystemContext()),
		updateInterval:  5 * time.Second,  // Check every 5 seconds
		cleanupInterval: 30 * time.Second, // Cleanup every 30 seconds
	}
}

// GetDeferredHashStats returns lifetime counters for registered and processed deferred operations.
func (dhm *DeferredHashManager) GetDeferredHashStats() (registered, processed int64) {
	return dhm.registeredTotal.Load(), dhm.processedTotal.Load()
}

// RegisterOperation registers a pending operation that affects an object
// This should be called before any operation that modifies an object
func (dhm *DeferredHashManager) RegisterOperation(objectID, objectKind, filePath, operationID string) {
	var pending *PendingObjectOps
	_ = concurrency.RunInLockOrLog(
		&dhm.mu, locknames.LockNameDeferredHashRegisterGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var exists bool
			pending, exists = dhm.pendingOps[objectID]
			if !exists {
				pending = &PendingObjectOps{
					ObjectID:   objectID,
					ObjectKind: objectKind,
					FilePath:   filePath,
					Operations: make([]string, 0),
					LastUpdate: time.Now(),
				}
				dhm.pendingOps[objectID] = pending
			}
			return nil
		},
	)

	// Update pending operations (nested lock)
	_ = concurrency.RunInLockOrLog(
		&pending.mu, locknames.LockNameDeferredHashRegisterUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Add operation if not already present
			found := false
			for _, opID := range pending.Operations {
				if opID == operationID {
					found = true
					break
				}
			}
			if !found {
				pending.Operations = append(pending.Operations, operationID)
				pending.LastUpdate = time.Now()
				dhm.registeredTotal.Add(1)
			}
			return nil
		},
	)
}

// CompleteOperation marks an operation as complete
// If this was the last pending operation for an object, triggers hash update
func (dhm *DeferredHashManager) CompleteOperation(objectID, operationID string) error {
	var pending *PendingObjectOps
	var exists bool
	var objectKind, filePath string
	var shouldUpdate bool
	_ = concurrency.RunInLockOrLog(
		&dhm.mu, locknames.LockNameDeferredHashCompleteGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pending, exists = dhm.pendingOps[objectID]
			if !exists {
				return nil
			}
			return nil
		},
	)

	if !exists {
		// No pending operations - object is ready for hash update
		return dhm.updateHashAndCache(pkgctx.NewSystemContext(), objectID, "", "")
	}

	// Update pending operations (nested lock)
	_ = concurrency.RunInLockOrLog(
		&pending.mu, locknames.LockNameDeferredHashCompleteUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Remove operation from pending list
			newOps := make([]string, 0, len(pending.Operations))
			for _, opID := range pending.Operations {
				if opID != operationID {
					newOps = append(newOps, opID)
				}
			}
			pending.Operations = newOps

			// If no more pending operations, update hash and cache
			if len(pending.Operations) == 0 {
				shouldUpdate = true
				objectKind = pending.ObjectKind
				filePath = pending.FilePath
			}
			return nil
		},
	)

	if shouldUpdate {
		// Remove from map and update hash (outside locks)
		_ = concurrency.RunInLockOrLog(
			&dhm.mu, locknames.LockNameDeferredHashCompleteDelete, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				delete(dhm.pendingOps, objectID)
				return nil
			},
		)
		return dhm.updateHashAndCache(pkgctx.NewSystemContext(), objectID, objectKind, filePath)
	}

	return nil
}

// updateHashAndCache computes and updates the integrity hash and ID cache for an object
// This is the final step after all operations are complete
func (dhm *DeferredHashManager) updateHashAndCache(ctx context.Context, objectID, objectKind, filePath string) error {
	// If file path not provided, infer it
	if filePath == emptyValue {
		// Try to get file path from storage
		// For file storage, we can infer from object ID and kind
		if fileStorage, ok := dhm.storage.(*FileObjectStorage); ok {
			var err error
			filePath, err = fileStorage.getObjectFilePath(objectID, objectKind)
			if err != nil {
				return errfmt.Errorf(ConstMiscFailedToGetFilePathForObjectSW, objectID, err)
			}
		} else {
			return errfmt.Errorf(ConstMiscCannotInferFilePathForNonFileStorage)
		}
	}

	// Read file content
	fileData, err := fileutil.ReadFile(filePath)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToReadFileForHashCalculation).Wrap(err)
	}

	// Calculate hash
	hash := dhm.calculateHash(fileData)

	// Update hash registry
	if fileStorage, ok := dhm.storage.(*FileObjectStorage); ok {
		kind := objectKind
		if kind == emptyValue {
			// Infer kind from file path using storage's ID validator
			if err := fileStorage.idValidator.LoadPatterns(); err == nil {
				kind = fileStorage.idValidator.InferKindFromID(objectID)
			}
			if kind == emptyValue {
				// Fallback: try to infer from directory structure
				dir := filepath.Dir(filePath)
				// Extract kind from directory (e.g., docs/process/backlog -> backlog_item)
				// This is a simplified inference
				baseDir := filepath.Base(dir)
				kind = baseDir // Simplified - may need mapping
			}
		}

		hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), kind, filepath.Dir(filePath))
		if err := hashRegistry.Load(); err != nil {
			// Hash registry doesn't exist yet - will be created on save
		}

		filename := filepath.Base(filePath)
		hashRegistry.SetHash(filename, hash)

		// Save hash registry with retry (critical for integrity - must succeed)
		if err := fileStorage.saveHashRegistryWithRetry(hashRegistry, objectID, filename, hash); err != nil {
			return errfmt.Newf(ConstMiscFailedToPersistHashRegistry).Wrap(err)
		}

		// Update ID cache
		if err := dhm.updateIDCache(ctx, objectID, objectKind, filePath); err != nil {
			// Log warning but don't fail - ID cache is best effort
			StorageLog(dhm.logger.Logger()).Warn(LogEventStorageDeferredHashIDCacheUpdateFailedWarn).
				ObjectID(objectID).
				WithError(err).
				Log()
		}

		StorageLog(dhm.logger.Logger()).Info(LogEventStorageDeferredHashUpdatedIntegrityInfo).
			ObjectID(objectID).
			Kind(kind).
			String("hash", hash).
			Log()

		dhm.processedTotal.Add(1)
		return nil
	}

	return errfmt.Errorf(ConstMiscDeferredHashUpdateOnlySupportedForFileSt)
}

// updateIDCache updates the object ID cache for an object
func (dhm *DeferredHashManager) updateIDCache(ctx context.Context, objectID, objectKind, filePath string) error {
	// Use existing cache operation handler if available
	if cacheOperationHandler != nil {
		cacheCtx := pkgctx.WithCacheUpdate(ctx, objectID, objectKind, filePath)
		// Extract CacheContext from context
		if cacheCtxValue := pkgctx.GetCacheContext(cacheCtx); cacheCtxValue != nil {
			return cacheOperationHandler(cacheCtxValue)
		}
	}
	return nil
}

// calculateHash calculates SHA256 hash of content
func (dhm *DeferredHashManager) calculateHash(content []byte) string {
	if fileStorage, ok := dhm.storage.(*FileObjectStorage); ok {
		return fileStorage.calculateHash(content)
	}
	// Fallback implementation - use shared hash calculation function
	// This ensures consistency even when storage is not FileObjectStorage
	return CalculateSHA256Hash(content)
}

// GetPendingOperations returns the number of pending operations for an object
func (dhm *DeferredHashManager) GetPendingOperations(objectID string) int {
	var pending *PendingObjectOps
	var exists bool
	_ = concurrency.RunInRLockOrLog(
		&dhm.mu, locknames.LockNameDeferredHashGetPendingManager, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			pending, ok = dhm.pendingOps[objectID]
			exists = ok
			return nil
		},
	)

	if !exists {
		return 0
	}

	var count int
	_ = concurrency.RunInRLockOrLog(
		&pending.mu, locknames.LockNameDeferredHashGetPendingOps, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = len(pending.Operations)
			return nil
		},
	)
	return count
}

// IsObjectReady returns true if an object has no pending operations and is ready for hash update
func (dhm *DeferredHashManager) IsObjectReady(objectID string) bool {
	return dhm.GetPendingOperations(objectID) == 0
}

// StartBackgroundProcessor starts a background goroutine that periodically
// checks for objects ready for hash update and processes them
func (dhm *DeferredHashManager) StartBackgroundProcessor(ctx context.Context) {
	goroutinelabels.NewGoroutine(ConstMiscDeferredHashManagerProcessor, ConstMiscProcessingDeferredHashUpdatesAndCleanup).
		StartWithContext(ctx, func(ctx context.Context) error {
			ticker := time.NewTicker(dhm.updateInterval)
			defer ticker.Stop()

			cleanupTicker := time.NewTicker(dhm.cleanupInterval)
			defer cleanupTicker.Stop()

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					dhm.processReadyObjects(ctx)
				case <-cleanupTicker.C:
					dhm.cleanupOldOperations()
				}
			}
		})
}

// processReadyObjects processes objects that are ready for hash update
func (dhm *DeferredHashManager) processReadyObjects(ctx context.Context) {
	var readyObjects []*PendingObjectOps
	_ = concurrency.RunInRLockOrLog(
		&dhm.mu, locknames.LockNameDeferredHashProcessReadyCollect, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			readyObjects = make([]*PendingObjectOps, 0)
			for _, pending := range dhm.pendingOps {
				_ = concurrency.RunInRLockOrLog(
					&pending.mu, locknames.LockNameDeferredHashProcessReadyCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						if len(pending.Operations) == 0 {
							readyObjects = append(readyObjects, pending)
						}
						return nil
					},
				)
			}
			return nil
		},
	)

	// Process ready objects
	for _, pending := range readyObjects {
		_ = concurrency.RunInLockOrLog(
			&dhm.mu, locknames.LockNameDeferredHashProcessReadyDelete, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				delete(dhm.pendingOps, pending.ObjectID)
				return nil
			},
		)

		// Update hash and cache
		if err := dhm.updateHashAndCache(ctx, pending.ObjectID, pending.ObjectKind, pending.FilePath); err != nil {
			StorageLog(dhm.logger.Logger()).Error(LogEventStorageDeferredHashUpdateReadyFailedErr, err).
				ObjectID(pending.ObjectID).
				Log()
		}
	}
}

// cleanupOldOperations removes old completed operations from tracking
func (dhm *DeferredHashManager) cleanupOldOperations() {
	cutoff := time.Now().Add(-1 * time.Hour) // Remove operations older than 1 hour
	var staleObjects []struct {
		objectID   string
		objectKind string
		filePath   string
	}
	_ = concurrency.RunInLockOrLog(
		&dhm.mu, locknames.LockNameDeferredHashCleanupCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			staleObjects = make([]struct {
				objectID   string
				objectKind string
				filePath   string
			}, 0)
			for objectID, pending := range dhm.pendingOps {
				var lastUpdate time.Time
				_ = concurrency.RunInRLockOrLog(
					&pending.mu, locknames.LockNameDeferredHashCleanupCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						lastUpdate = pending.LastUpdate
						return nil
					},
				)

				if lastUpdate.Before(cutoff) {
					staleObjects = append(staleObjects, struct {
						objectID   string
						objectKind string
						filePath   string
					}{
						objectID:   objectID,
						objectKind: pending.ObjectKind,
						filePath:   pending.FilePath,
					})
				}
			}
			return nil
		},
	)

	// Process stale objects (outside lock)
	for _, stale := range staleObjects {
		// Force hash update
		if err := dhm.updateHashAndCache(pkgctx.NewSystemContext(), stale.objectID, stale.objectKind, stale.filePath); err != nil {
			StorageLog(dhm.logger.Logger()).Warn(LogEventStorageDeferredHashForceStaleFailedWarn).
				ObjectID(stale.objectID).
				WithError(err).
				Log()
		}

		// Remove from map
		_ = concurrency.RunInLockOrLog(
			&dhm.mu, locknames.LockNameDeferredHashCleanupRemove, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				delete(dhm.pendingOps, stale.objectID)
				return nil
			},
		)
	}
}

// Global deferred hash manager instance
var globalDeferredHashManager *DeferredHashManager
var globalDeferredHashManagerOnce sync.Once

// GetDeferredHashManager returns the global deferred hash manager instance
func GetDeferredHashManager(storage ObjectStorageProvider) *DeferredHashManager {
	globalDeferredHashManagerOnce.Do(func() {
		globalDeferredHashManager = NewDeferredHashManager(storage)
	})
	return globalDeferredHashManager
}
