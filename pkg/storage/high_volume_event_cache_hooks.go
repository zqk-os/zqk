package storage

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// updateHighVolumeEventCacheOnCreate updates the high-volume event cache when an audit event is created
func updateHighVolumeEventCacheOnCreate(ctx context.Context, storageProvider ObjectStorageProvider, eventID string, instance map[string]any, options *AuditEventOptions) {
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

	if fileStorage, ok := storageProvider.(*FileObjectStorage); ok {
		if path, err := fileStorage.GetFilePathForObject(eventID, objects.KindAuditEvent); err == nil {
			entry.FilePath = path
		}
	}
	cache.Set(entry)
}

// updateHighVolumeEventCacheOnBulkDelete invalidates cache entries for deleted events
func updateHighVolumeEventCacheOnBulkDelete(eventIDs []string) {
	cache := GetGlobalHighVolumeEventCache()
	for _, id := range eventIDs {
		cache.Invalidate(id)
	}
}

// updateHighVolumeEventCacheOnDelete invalidates a single cache entry
func updateHighVolumeEventCacheOnDelete(eventID string) {
	cache := GetGlobalHighVolumeEventCache()
	cache.Invalidate(eventID)
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
	cache := GetGlobalHighVolumeEventCache()
	if cache.IsPopulatedForProject(projectRoot) {
		if err := cache.SaveCache(projectRoot); err != nil {
			logger := logging.GetLoggerFromProfile("system")
			StorageLog(logger).Warn("Failed to save high-volume event cache").
				ProjectRoot(projectRoot).
				WithError(err).
				Log()
		}
	}
}
