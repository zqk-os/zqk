package storage

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// (same class of miss as reverse-reference-index before SaveCache-on-mutate).
const highVolumeEventPersistDebounce = 250 * time.Millisecond

var (
	hvePersistMu    sync.Mutex
	hvePersistTimer *time.Timer
)

func scheduleHighVolumeEventCachePersist(projectRoot string) {
	if projectRoot == emptyValue {
		projectRoot = reverseReferenceBoundProjectRoot()
	}
	if projectRoot == emptyValue {
		return
	}
	root := projectRoot
	hvePersistMu.Lock()
	defer hvePersistMu.Unlock()
	if hvePersistTimer != nil {
		hvePersistTimer.Stop()
	}
	hvePersistTimer = time.AfterFunc(highVolumeEventPersistDebounce, func() {
		hvePersistMu.Lock()
		if hvePersistTimer == nil {
			hvePersistMu.Unlock()
			return
		}
		hvePersistTimer = nil
		hvePersistMu.Unlock()
		SaveHighVolumeEventCache(root)
	})
}

// StopHighVolumeEventCachePersistForTest cancels any pending debounce timer for tests.
func StopHighVolumeEventCachePersistForTest(projectRoot string) {
	hvePersistMu.Lock()
	defer hvePersistMu.Unlock()
	if hvePersistTimer != nil {
		hvePersistTimer.Stop()
		hvePersistTimer = nil
	}
}

// updateHighVolumeEventCacheOnCreate updates the high-volume event cache when an audit event is created
func updateHighVolumeEventCacheOnCreate(_ context.Context, storageProvider ObjectStorageProvider, eventID string, instance map[string]any, _ *AuditEventOptions) {
	cache := GetGlobalHighVolumeEventCache()

	var createdAt time.Time
	if s := objects.GetString(instance, objects.FieldKeyCreatedAt); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			createdAt = t
		}
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	entry := &HighVolumeEventCacheEntry{
		ID:        eventID,
		Kind:      objects.KindAuditEvent,
		CreatedAt: createdAt,
		EventType: objects.GetString(instance, objects.FieldKeyEventType),
		Status:    objects.GetString(instance, objects.FieldKeyStatus),
		Exists:    true,
	}

	projectRoot := emptyValue
	if fileStorage, ok := storageProvider.(*FileObjectStorage); ok {
		projectRoot = fileStorage.projectRoot
		if path, err := fileStorage.GetFilePathForObject(eventID, objects.KindAuditEvent); err == nil {
			entry.FilePath = path
		}
	}
	cache.Set(entry)
	scheduleHighVolumeEventCachePersist(projectRoot)
}

// updateHighVolumeEventCacheOnBulkDelete invalidates cache entries for deleted events
func updateHighVolumeEventCacheOnBulkDelete(eventIDs []string) {
	cache := GetGlobalHighVolumeEventCache()
	for _, id := range eventIDs {
		cache.Invalidate(id)
	}
	scheduleHighVolumeEventCachePersist(emptyValue)
}

// updateHighVolumeEventCacheOnDelete invalidates a single cache entry
func updateHighVolumeEventCacheOnDelete(eventID string) {
	cache := GetGlobalHighVolumeEventCache()
	cache.Invalidate(eventID)
	scheduleHighVolumeEventCachePersist(emptyValue)
}

// EnsureHighVolumeEventCacheReady ensures the cache is built and ready for use
func EnsureHighVolumeEventCacheReady(ctx context.Context, projectRoot string, storageProvider ObjectStorageProvider, forceRebuild bool) error {
	cache := GetGlobalHighVolumeEventCache()
	if !forceRebuild && cache.IsPopulatedForProject(projectRoot) {
		return nil
	}

	// Try to load from disk first
	if !forceRebuild {
		loaded, err := cache.LoadCache(projectRoot)
		if err == nil && loaded {
			return nil
		}
	}

	// Rebuild if load failed or forced
	return cache.BuildCache(ctx, projectRoot, storageProvider)
}

// SaveHighVolumeEventCache saves the global high-volume event cache to disk for the given project.
func SaveHighVolumeEventCache(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	cache := GetGlobalHighVolumeEventCache()
	// Always persist when scheduled: SaveCache creates metadata if missing so incremental
	// Set/Invalidate after CUD is durable across CLI processes (not only after BuildCache).
	if err := cache.SaveCache(projectRoot); err != nil {
		logger := logging.GetLoggerFromProfile("system")
		StorageLog(logger).Warn("Failed to save high-volume event cache").
			ProjectRoot(projectRoot).
			WithError(err).
			Log()
	}
}
