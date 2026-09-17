package storage

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// streamLocationKey returns the sync.Map key for stream-backed object location (kind\x00id).
func streamLocationKey(id, kind string) string {
	return kind + "\x00" + id
}

// setStreamLocation registers the segment path and offset for a stream-backed object (used by writeObjectToStream).
func (f *FileObjectStorage) setStreamLocation(id, kind, location string) {
	f.streamLocations.Store(streamLocationKey(id, kind), location)
}

// getStreamLocation returns "segmentPath::offset" if this object was written via stream; otherwise "".
// Checks in-memory first, then persistent registry (so retention/List work across processes).
func (f *FileObjectStorage) getStreamLocation(id, kind string) string {
	if v, ok := f.streamLocations.Load(streamLocationKey(id, kind)); ok {
		if s, _ := v.(string); s != emptyValue {
			return s
		}
	}
	if f.projectRoot != emptyValue {
		if loc := getStreamLocationFromPersistentRegistry(f.projectRoot, kind, id); loc != emptyValue {
			return loc
		}
	}
	return ""
}

// removeStreamLocation removes the stream location for an object (on delete).
func (f *FileObjectStorage) removeStreamLocation(id, kind string) {
	f.streamLocations.Delete(streamLocationKey(id, kind))
}

// listStreamIDsForKind returns all object IDs registered for the given kind (in-memory + persistent registry).
// Used by List to merge stream-backed IDs with CAS IDs when stream storage is enabled for the kind.
func (f *FileObjectStorage) listStreamIDsForKind(kind string) []string {
	seen := make(map[string]bool)
	prefix := kind + "\x00"

	// Load deleted set to exclude soft-deleted objects
	var deletedMap map[string]bool
	if f.projectRoot != emptyValue {
		_, deletedMap = readStreamRegistryJSONLIntoMaps(f.projectRoot, kind)
	}

	f.streamLocations.Range(func(key, value interface{}) bool {
		if k, ok := key.(string); ok && len(k) > len(prefix) && k[:len(prefix)] == prefix {
			id := k[len(prefix):]
			if deletedMap == nil || !deletedMap[id] {
				seen[id] = true
			}
		}
		return true
	})
	if f.projectRoot != emptyValue {
		for _, id := range ListStreamIDsFromPersistentRegistry(f.projectRoot, kind) {
			if deletedMap == nil || !deletedMap[id] {
				seen[id] = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

// BuildHighVolumeCacheEntriesFromStream returns high-volume cache entries for the given kind from the stream location registry.
// Used when building the high-volume event cache so Count/OldestIDs include stream-backed objects after rebuild or restart.
// Respects ctx deadline; caps entries at maxStreamEntriesForCacheBuild to avoid long builds.
func (f *FileObjectStorage) BuildHighVolumeCacheEntriesFromStream(ctx context.Context, kind string, logger logging.Logger) ([]*HighVolumeEventCacheEntry, error) {
	if !StreamStorageEnabledForKind(kind) {
		return nil, nil
	}
	ids := f.listStreamIDsForKind(kind)
	if len(ids) == 0 {
		return nil, nil
	}
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 2*time.Minute && len(ids) > maxStreamEntriesForCacheBuild {
			ids = ids[:maxStreamEntriesForCacheBuild]
			if logger != nil {
				StorageLog(logger).Info(LogEventStorageHighVolumeCacheBuildingFromStreamCappedInfo).
					Kind(kind).
					Int("capped", maxStreamEntriesForCacheBuild).
					Log()
			}
		}
	}
	var entries []*HighVolumeEventCacheEntry
	for _, id := range ids {
		if ctx != nil && ctx.Err() != nil {
			return entries, ctx.Err()
		}
		loc := f.getStreamLocation(id, kind)
		if loc == emptyValue {
			continue
		}
		segmentPath, offset, ok := StreamPathAndOffset(loc)
		if !ok || segmentPath == emptyValue {
			continue
		}
		obj, err := ReadRecordAt(segmentPath, offset)
		if err != nil {
			if logger != nil {
				StorageLog(logger).Debug(LogEventStorageHighVolumeCacheSkipStreamRecordDebug).
					ObjectID(id).
					Kind(kind).
					WithError(err).
					Log()
			}
			continue
		}
		// Do not call AppendStreamLocationToRegistry here. New stream objects are already registered
		// on write (object_storage_file_create). Re-appending the same id on every cache build was
		// duplicative; each append also invalidates the in-process stream registry snapshot, so the
		// next getStreamLocation re-read the full registry file from disk — O(n²) I/O for n IDs.
		var createdAt time.Time
		if s := objects.GetString(obj, objects.FieldKeyCreatedAt); s != emptyValue {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				createdAt = t
			}
		}
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		eventType, _ := obj[objects.FieldKeyEventType].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		mtime := time.Now().UTC()
		if info, err := fileutil.Stat(segmentPath); err == nil {
			mtime = info.ModTime()
		}
		entries = append(entries, &HighVolumeEventCacheEntry{
			ID: id, Kind: kind, CreatedAt: createdAt, EventType: eventType, Status: status,
			FilePath: loc, MTime: mtime, Exists: true,
		})
	}
	return entries, nil
}
