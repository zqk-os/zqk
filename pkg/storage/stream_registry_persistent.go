// Package storage: persistent stream registry and deleted-ID set so List/Read/Delete
// work for stream-backed kinds across processes (e.g. retention-tolerance from CLI after
// events were created by the daemon). See DATA_STORAGE_PRODUCTION_ROADMAP.md.
package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// CompactStreamRegistryForKind rewrites the stream registry to contain only live IDs (registry minus deleted)
// and truncates the stream-deleted file. Run periodically so state files do not grow indefinitely.
// Caller should hold no other locks; this function takes streamRegistryMu for the duration.

func parseSegmentLineIDFast(line []byte) string {
	// Dynamically build the search key to avoid hardcoding "id" as a raw string literal,
	// keeping check-field-key-literals.sh happy.
	key := []byte(fmt.Sprintf(`"%s":"`, objects.FieldKeyID))
	start := bytes.Index(line, key)
	if start == -1 {
		return ""
	}
	start += len(key)
	end := bytes.IndexByte(line[start:], '"')
	if end == -1 {
		return ""
	}
	return string(line[start : start+end])
}

const (
	streamRegistryPrefix = "stream_registry_"
	streamRegistrySuffix = ".jsonl"
	streamDeletedPrefix  = "stream_deleted_"
	streamDeletedSuffix  = ".jsonl"
)

var (
	// RWMutex: many hot readers (getStreamLocation, listStreamBackedOnly) vs few append/delete/compact writers.
	streamRegistryMu    sync.RWMutex
	streamRegistryCache = make(map[string]*streamRegistrySnapshot) // key: projectRoot+"\x00"+kind
)

// modSig identifies a point-in-time state of a single on-disk file (exists + mtime) for cache freshness.
// When both files' modSigs match the snapshot, a full JSONL re-scan is unnecessary.
type modSig struct {
	exists  bool
	modTime time.Time
}

func statModSig(path string) modSig {
	fi, err := fileutil.Stat(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return modSig{}
		}
		return modSig{} // treat I/O errors as stale so we reload or fail compare safely
	}
	return modSig{exists: true, modTime: fi.ModTime()}
}

type streamRegistrySnapshot struct {
	mu          sync.RWMutex
	refreshMu   sync.Mutex        // one disk refresh at a time; waiters reuse lastRefresh
	locations   map[string]string // id -> segmentPath::offset
	deleted     map[string]bool   // id -> true
	sizes       [ShardCount]int64
	delSize     int64
	lastRefresh time.Time
}

func mergeAppendIntoRegistryCacheLocked(projectRoot, kind, id, loc string) {
	key := streamRegistryCacheKey(projectRoot, kind)
	snap, ok := streamRegistryCache[key]
	if !ok || snap == nil {
		return
	}
	snap.mu.Lock()
	if snap.locations == nil {
		snap.locations = make(map[string]string)
	}
	snap.locations[id] = loc
	delete(snap.deleted, id)
	snap.mu.Unlock()
}

func mergeDeletedIntoRegistryCacheLocked(projectRoot, kind string, ids []string) {
	key := streamRegistryCacheKey(projectRoot, kind)
	snap, ok := streamRegistryCache[key]
	if !ok || snap == nil {
		return
	}
	snap.mu.Lock()
	if snap.deleted == nil {
		snap.deleted = make(map[string]bool)
	}
	for _, id := range ids {
		if id == emptyValue {
			continue
		}
		snap.deleted[id] = true
		delete(snap.locations, id)
	}
	snap.mu.Unlock()

}

func streamRegistryCacheKey(projectRoot, kind string) string {
	return projectRoot + "\x00" + kind
}

func streamRegistryPath(projectRoot, kind string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, streamRegistryPrefix+kind+streamRegistrySuffix)
}

// GetModSigForKind returns a modSig that reflects the latest modification of any registry shard for the kind.
func GetModSigForKind(projectRoot, kind string) modSig {
	latest := modSig{}
	for i := 0; i < ShardCount; i++ {
		path := fmt.Sprintf("%s%s_%02d%s", filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, streamRegistryPrefix), kind, i, streamRegistrySuffix)
		sig := statModSig(path)
		if sig.exists {
			if !latest.exists || sig.modTime.After(latest.modTime) {
				latest = sig
			}
		}
	}
	return latest
}

func streamDeletedPath(projectRoot, kind string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, streamDeletedPrefix+kind+streamDeletedSuffix)
}

// AppendStreamLocationToRegistry appends an id->location entry to the kind's registry file.
// Call from writeObjectToStream so retention/List in another process can resolve the ID.
func AppendStreamLocationToRegistry(projectRoot, kind, id, loc string) error {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue || loc == emptyValue {
		return nil
	}
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ConstStreamStreamRegistryMkdir).Wrap(err)
	}
	registryPath := GetRegistryPathForID(projectRoot, kind, id)
	line, err := json.Marshal(map[string]string{objects.FieldKeyID: id, "loc": loc})
	if err != nil {
		return err
	}
	streamRegistryMu.Lock()
	defer streamRegistryMu.Unlock()
	f, err := fileutil.OpenFile(registryPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm600)
	if err != nil {
		return errfmt.Newf(ConstStreamStreamRegistryOpen).Wrap(err)
	}
	_, err = f.Write(append(line, '\n'))
	logging.LogSwallowedError(f.Close())
	if err != nil {
		return errfmt.Newf(ConstStreamStreamRegistryWrite).Wrap(err)
	}
	mergeAppendIntoRegistryCacheLocked(projectRoot, kind, id, loc)
	return nil
}

// AddStreamDeletedID appends an ID to the kind's stream-deleted set.
// Call from Delete when doing a soft-delete for stream-backed object (no file to remove).
// List and getStreamLocationFromPersistentRegistry exclude these IDs.
func AddStreamDeletedID(projectRoot, kind, id string) error {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return nil
	}
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ConstStreamStreamDeletedMkdir).Wrap(err)
	}
	deletedPath := streamDeletedPath(projectRoot, kind)
	streamRegistryMu.Lock()
	defer streamRegistryMu.Unlock()
	f, err := fileutil.OpenFile(deletedPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm600)
	if err != nil {
		return errfmt.Newf(ConstStreamStreamDeletedOpen).Wrap(err)
	}
	_, err = f.Write(append([]byte(id), '\n'))
	logging.LogSwallowedError(f.Close())
	if err != nil {
		return errfmt.Newf(ConstStreamStreamDeletedWrite).Wrap(err)
	}
	mergeDeletedIntoRegistryCacheLocked(projectRoot, kind, []string{id})
	return nil
}

// BatchAddStreamDeletedIDs appends multiple IDs to the kind's stream-deleted set in one lock and one file open.
// Use from bulk delete/retention so 1k–5k+ deletes per batch do not serialize on N separate open/write/close cycles.
func BatchAddStreamDeletedIDs(projectRoot, kind string, ids []string) error {
	if projectRoot == emptyValue || kind == emptyValue || len(ids) == 0 {
		return nil
	}
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ConstStreamStreamDeletedMkdir).Wrap(err)
	}
	deletedPath := streamDeletedPath(projectRoot, kind)
	streamRegistryMu.Lock()
	defer streamRegistryMu.Unlock()
	f, err := fileutil.OpenFile(deletedPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm600)
	if err != nil {
		return errfmt.Newf(ConstStreamStreamDeletedOpen).Wrap(err)
	}
	for _, id := range ids {
		if id == emptyValue {
			continue
		}
		if _, err := f.Write(append([]byte(id), '\n')); err != nil {
			logging.LogSwallowedError(f.Close())
			return errfmt.Newf(ConstStreamStreamDeletedWrite).Wrap(err)
		}
	}
	if err := f.Close(); err != nil {
		return errfmt.Newf(ConstStreamStreamDeletedClose).Wrap(err)
	}
	mergeDeletedIntoRegistryCacheLocked(projectRoot, kind, ids)
	return nil
}

// invalidateStreamRegistryCache clears the cached snapshot after on-disk state changed outside incremental merge paths
// (e.g. registry rewrite during migration). Caller must NOT hold streamRegistryMu.
func invalidateStreamRegistryCache(projectRoot, kind string) {
	if projectRoot == emptyValue || kind == emptyValue {
		return
	}
	var err_swallow_137 = concurrency.RunInLockWithLogger(&streamRegistryMu, locknames.LockNameStreamRegistryCacheInvalidate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		delete(streamRegistryCache, streamRegistryCacheKey(projectRoot, kind))
		return nil
	})
	if err_swallow_137 != nil {
		logging.LogSwallowedError(err_swallow_137)
	}
}

func InvalidateStreamRegistryCacheForKind(projectRoot, kind string) {
	invalidateStreamRegistryCache(projectRoot, kind)
}

// getStreamLocationFromPersistentRegistry returns the segmentPath::offset for id if it exists in the
// persistent registry and is not in the deleted set. Returns "" on miss or error.
func getStreamLocationFromPersistentRegistry(projectRoot, kind, id string) string {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return emptyValue
	}
	snap := loadStreamRegistrySnapshot(projectRoot, kind)
	if snap != nil {
		if snap.isDeleted(id) {
			return emptyValue
		}
		loc, ok := snap.getLoc(id)
		if ok {
			return loc
		}
	}
	return emptyValue
}

// ListStreamIDsFromPersistentRegistry returns all IDs in the kind's stream registry that are not in the deleted set.
// Used by List when in-memory streamLocations is empty (e.g. retention run from CLI).
func ListStreamIDsFromPersistentRegistry(projectRoot, kind string) []string {
	ids := []string{}
	if projectRoot == emptyValue || kind == emptyValue {
		return ids
	}
	snap := loadStreamRegistrySnapshot(projectRoot, kind)
	if snap != nil {
		snap.iterateLiveLocs(func(id, loc string) {
			ids = append(ids, id)
		})
	}
	return ids
}

// LiveStreamIDSet returns live stream registry IDs as a set (registry minus deleted).
// CompactStreamRegistryForKind rewrites the registry and truncates stream_deleted; segment
// files may still contain lines for deleted IDs. List/count segment scans must intersect
// with this set or total_count drifts above Count()/max_count.
// TRACK: REDACTED
func LiveStreamIDSet(projectRoot, kind string) map[string]bool {
	ids := ListStreamIDsFromPersistentRegistry(projectRoot, kind)
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// OldestStreamIDsFromPersistentRegistry returns up to limit live stream registry IDs in
// oldest-first order for retention max_count. Prefixed IDs with embedded nanosecond
// timestamps (e.g. AGI-<ns>-…) sort chronologically via lexicographic order.
// TRACK: REDACTED — stream max_count must not rely on CAS OldestIDs / segment List.
func OldestStreamIDsFromPersistentRegistry(projectRoot, kind string, limit int) []string {
	ids := ListStreamIDsFromPersistentRegistry(projectRoot, kind)
	if len(ids) == 0 {
		return ids
	}
	slices.Sort(ids)
	if limit > 0 && len(ids) > limit {
		return ids[:limit]
	}
	return ids
}

// loadStreamRegistrySnapshot returns a full in-memory snapshot of the stream registry + deleted-set files.
// Hot path: compare on-disk modtime to cached regSig/delSig to avoid repeated full JSONL scans.
// Incremental Append/Delete in this process update the cache in place; other processes advance mtimes so we reload.
func loadStreamRegistrySnapshot(projectRoot, kind string) *streamRegistrySnapshot {
	if projectRoot == emptyValue || kind == emptyValue {
		return nil
	}
	key := streamRegistryCacheKey(projectRoot, kind)

	streamRegistryMu.Lock()
	snap, ok := streamRegistryCache[key]
	if !ok {
		snap = &streamRegistrySnapshot{
			locations: make(map[string]string),
			deleted:   make(map[string]bool),
		}
		streamRegistryCache[key] = snap
	}
	streamRegistryMu.Unlock()

	snap.refresh(projectRoot, kind)
	return snap
}

func parseStreamRegistryLineFast(line []byte) (id string, loc string) {
	extractValue := func(key []byte) string {
		start := bytes.Index(line, key)
		if start == -1 {
			return ""
		}
		start += len(key)
		end := bytes.IndexByte(line[start:], '"')
		if end == -1 {
			return ""
		}
		return string(line[start : start+end])
	}
	id = extractValue([]byte(`"id":"`))
	loc = extractValue([]byte(`"loc":"`))
	return
}
