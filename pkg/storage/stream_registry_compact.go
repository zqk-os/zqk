// Extracted from stream_registry_persistent.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func CompactStreamRegistryForKind(projectRoot, kind string) error {
	if projectRoot == emptyValue || kind == emptyValue {
		return nil
	}
	key := streamRegistryCacheKey(projectRoot, kind)

	// Step 1: Pre-refresh the snapshot outside the global lock.
	// For massive kinds (e.g. change_journal_entry), parsing 400MB of JSON lines can take 30s.
	// We MUST NOT hold the global streamRegistryMu during this time, or the whole system freezes.
	// snap is thread-safe internally (uses snap.mu).
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

	// Parse the massive file while the rest of the system runs normally.
	snap.refresh(projectRoot, kind)

	// Step 1.5: Merge older daily segment files and get new locations
	relocations, err := MergeDailySegmentsForKind(projectRoot, kind, snap)
	if err != nil {
		logger := logging.GetLoggerFromProfile("system")
		StorageLog(logger).Warn("Failed to merge daily stream segments during compaction").
			Kind(kind).
			WithError(err).
			Log()
	}

	// Step 2: Now acquire the global lock to prevent concurrent appends.
	// Call refresh again to catch any tiny delta that was appended while we were parsing.
	// This second refresh will be instant because snap.sizes tracked what we already read.
	streamRegistryMu.Lock()
	defer streamRegistryMu.Unlock()
	snap.refresh(projectRoot, kind)

	locations := make(map[string]string)
	deleted := make(map[string]bool)
	snap.iterateLocs(func(id, loc string) {
		locations[id] = loc
	})
	snap.iterateDeleted(func(id string) {
		deleted[id] = true
	})

	// Live = registry IDs not in deleted
	liveByShard := make(map[int][]struct{ id, loc string })
	for id, loc := range locations {
		if !deleted[id] {
			if newLoc, ok := relocations[id]; ok {
				loc = newLoc
			}
			// Determine which shard this ID belongs to
			shard := 0
			for i := 0; i < len(id); i++ {
				shard += int(id[i])
			}
			shard %= ShardCount
			liveByShard[shard] = append(liveByShard[shard], struct{ id, loc string }{id, loc})
		}
	}

	// Write new sharded registry files (overwrite existing shards with only live entries)
	for i := 0; i < ShardCount; i++ {
		shardPath := fmt.Sprintf("%s%s_%02d%s", filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, streamRegistryPrefix), kind, i, streamRegistrySuffix)
		tempShard := shardPath + ".tmp"

		live := liveByShard[i]
		if len(live) == 0 {
			// If no live entries for this shard, just ensure file exists/is empty or remove it?
			// Standard: keep the shard file but truncate it.
			if err := fileutil.WriteFile(tempShard, nil, paths.FilePerm600); err != nil {
				return errfmt.Newf(ConstStreamStreamRegistryCompactCreateTemp).Wrap(err)
			}
		} else {
			f, err := fileutil.OpenFile(tempShard, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_TRUNC, paths.FilePerm600)
			if err != nil {
				return errfmt.Newf(ConstStreamStreamRegistryCompactCreateTemp).Wrap(err)
			}
			for _, e := range live {
				line, _ := json.Marshal(map[string]string{objects.FieldKeyID: e.id, "loc": e.loc})
				if _, err := f.Write(append(line, '\n')); err != nil {
					_ = f.Close()
					_ = fileutil.Remove(tempShard)
					return err
				}
			}
			if err := f.Close(); err != nil {
				_ = fileutil.Remove(tempShard)
				return err
			}
		}

		if err := fileutil.Rename(tempShard, shardPath); err != nil {
			_ = fileutil.Remove(tempShard)
			return errfmt.Newf(ConstStreamStreamRegistryCompactRename).Wrap(err)
		}
	}

	// Also clear the legacy (unsharded) registry file if it exists and we have sharded state
	legacyPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, streamRegistryPrefix+kind+streamRegistrySuffix)
	if _, err := fileutil.Stat(legacyPath); err == nil {
		if err := fileutil.Truncate(legacyPath, 0); err != nil {
			logger := logging.GetLoggerFromProfile("system")
			StorageLog(logger).Warn("Failed to truncate legacy stream registry during compaction").
				Kind(kind).
				WithError(err).
				Log()
		}
	}

	// Truncate deleted file (all those IDs are no longer in registry)
	deletedPath := streamDeletedPath(projectRoot, kind)
	if _, err := fileutil.Stat(deletedPath); err == nil {
		if err := fileutil.Truncate(deletedPath, 0); err != nil {
			return errfmt.Newf(ConstStreamStreamDeletedCompactRename).Wrap(err)
		}
	}

	// Update high volume event cache with relocations
	if len(relocations) > 0 {
		cache := GetGlobalHighVolumeEventCache()
		if cache != nil {
			for id, newLoc := range relocations {
				if entry, exists := cache.Get(id); exists {
					entry.FilePath = newLoc
					cache.Set(entry)
				}
			}
			_ = cache.SaveCache(projectRoot)
		}
	}

	// Under same lock: drop in-process snapshot; on-disk state was fully rewritten.
	delete(streamRegistryCache, streamRegistryCacheKey(projectRoot, kind))
	return nil
}

// MergeDailySegmentsForKind merges daily process-specific segment files (e.g. YYYY-MM-DD_pidXXXXX_stream.json)
// into unified YYYY-MM-DD_stream.json files. It only keeps records that are live according to the registry snapshot.
// Today's live-PID shards are left in place (writers still append); dead-PID shards from today are merged.
// It returns a map from ID to new location format (path::offset) for all updated entries.
// TRACK: BLI-1785905541906569000-074e24d7
func MergeDailySegmentsForKind(projectRoot, kind string, snap *streamRegistrySnapshot) (map[string]string, error) {
	relocations := make(map[string]string)
	dir, err := GetStreamSegmentDir(projectRoot, kind)
	if err != nil {
		return nil, err
	}
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	filesByDate := make(map[string][]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, streamFileSuffix) && !strings.HasSuffix(name, legacyStreamFileSuffix) {
			continue
		}
		if len(name) < 10 {
			continue
		}
		datePrefix := name[:10]
		// Verify YYYY-MM-DD prefix
		if datePrefix[4] != '-' || datePrefix[7] != '-' {
			continue
		}
		filesByDate[datePrefix] = append(filesByDate[datePrefix], filepath.Join(dir, name))
	}

	todayBase := zqktime.NowLayoutUTC(zqktime.LayoutDate)

	for date, files := range filesByDate {
		mergeFiles, skip := streamMergeFilesForDate(date, todayBase, files)
		if skip {
			continue
		}
		// We always merge/compact to ensure deleted records are actually removed from disk.
		// Even if there is only 1 file, rewriting it reclaims space from any records
		// that are no longer in the live snapshot.
		unifiedPath := filepath.Join(dir, fmt.Sprintf("%s%s", date, streamFileSuffix))
		tempPath := unifiedPath + ".tmp"
		fOut, err := fileutil.OpenSecureTrunc(tempPath)
		if err != nil {
			return nil, err
		}

		liveCount := 0
		var currentOffset int64
		var mergeErr error

		mu := streamMutexForKind(kind)
		mu.Lock()

		for _, file := range mergeFiles {
			fIn, err := fileutil.OpenRead(file)
			if err != nil {
				continue
			}
			sc := bufio.NewScanner(fIn)
			buf := make([]byte, 0, 1024*1024)
			sc.Buffer(buf, 1024*1024)
			for sc.Scan() {
				line := sc.Bytes()
				id := parseSegmentLineIDFast(line)
				if id == "" {
					continue
				}

				isLive := false
				snap.mu.RLock()
				if _, ok := snap.locations[id]; ok {
					if !snap.deleted[id] {
						isLive = true
					}
				}
				snap.mu.RUnlock()

				if isLive {
					writeLine := append(line, '\n')
					if _, err := fOut.Write(writeLine); err != nil {
						mergeErr = err
						_ = fIn.Close()
						break
					}
					relocations[id] = FormatStreamLocation(unifiedPath, currentOffset)
					currentOffset += int64(len(writeLine))
					liveCount++
				}
			}
			_ = fIn.Close()
			if mergeErr != nil {
				break
			}
		}
		mu.Unlock()

		_ = fOut.Close()
		if mergeErr != nil {
			logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
			return nil, mergeErr
		}

		if liveCount == 0 {
			logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
			for _, file := range mergeFiles {
				logging.LogSwallowedError(fileutil.RemoveFile(file))
			}
		} else {
			if err := fileutil.RenameFile(tempPath, unifiedPath); err != nil {
				logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
				return nil, err
			}
			for _, file := range mergeFiles {
				if filepath.Clean(file) != filepath.Clean(unifiedPath) {
					logging.LogSwallowedError(fileutil.RemoveFile(file))
				}
			}
		}
	}

	return relocations, nil
}
