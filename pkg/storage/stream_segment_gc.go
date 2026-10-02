// Package storage: stream segment file garbage collection.
// Removes segment files (.zqk/streams/<kind>/<date>_stream.json) that have no live registry
// entries pointing to them — i.e., every record they contain has been soft-deleted and the
// registry compacted. Without GC, segment files accumulate indefinitely even after all their
// entries cycle through create→delete→compact.
//
// Design:
//   - Today's unified file and live-PID shards stay (writers may still append).
//   - Dead-PID shards from today are eligible when unreferenced.
//   - The kind stream mutex is held during deletion to block concurrent appends to a file
//     we are about to remove.
//   - Non-fatal per-file errors are counted but do not abort the overall GC pass.
//   - GC is deliberately separate from CompactStreamRegistryForKind so callers control when
//     each step runs; maintenance runner and CLI both invoke them in sequence.

package storage

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// GCStreamSegmentResult carries the outcome of a stream segment GC pass.
type GCStreamSegmentResult struct {
	Kind         string
	FilesRemoved int
	BytesFreed   int64
	Errors       int
}

// GCOrphanedStreamSegmentsForKind deletes segment files for kind that have no live registry
// entries pointing to them. Call after CompactStreamRegistryForKind so the live set is fresh.
//
// A segment file is considered orphaned when:
//  1. No live entry in stream_registry_<kind>.jsonl has a "loc" pointing to it, AND
//  2. It is not today's active segment (today's file may still receive in-flight appends).
//
// Returns GCStreamSegmentResult with counts. Per-file removal errors are non-fatal (counted
// in Errors) so one unremovable file does not block the rest.
func GCOrphanedStreamSegmentsForKind(projectRoot, kind string) GCStreamSegmentResult {
	res := GCStreamSegmentResult{Kind: kind}
	if projectRoot == emptyValue || kind == emptyValue {
		return res
	}

	// Derive segment directory directly — no path alias cache required. GC may be called
	// before pre-warm (e.g. from CompactStreamRegistryForKind at maintenance time).
	segDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir, kind)

	// Load live registry snapshot to determine which segment files are still referenced.
	snap := loadStreamRegistrySnapshot(projectRoot, kind)

	referencedFiles := make(map[string]bool)
	streamRegistryMu.RLock()
	if snap != nil {
		snap.iterateLiveLocs(func(id, loc string) {
			segPath, _, ok := StreamPathAndOffset(loc)
			if ok && segPath != emptyValue {
				referencedFiles[filepath.Clean(segPath)] = true
			}
		})
	}
	streamRegistryMu.RUnlock()

	// Always protect today's unified segment. Live-PID shards are protected per-file below;
	// dead-PID shards from today may be GC'd if unreferenced.
	todayBase := zqktime.NowLayoutUTC(zqktime.LayoutDate)

	entries, err := fileutil.ReadDir(segDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return res
		}
		// Directory exists but is unreadable; treat as zero orphans rather than hard error.
		res.Errors++
		return res
	}

	// Hold the stream kind mutex while deleting so concurrent appends cannot race against us
	// (the write path holds this same mutex when appending to a segment file).
	streamMutexForKind(kind).Lock()
	defer streamMutexForKind(kind).Unlock()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, streamFileSuffix) && !strings.HasSuffix(name, legacyStreamFileSuffix) {
			continue
		}
		fullPath := filepath.Clean(filepath.Join(segDir, name))

		// Protect today's unified file and live-PID shards (writers still append).
		// Dead-PID shards from today are eligible once unreferenced (merge relocates them).
		// TRACK: follow-up in kernel backlog
		if strings.HasPrefix(name, todayBase) {
			if _, ok := streamSegmentWriterPID(name); !ok {
				continue
			}
			if streamSegmentWriterStillLive(name) {
				continue
			}
		}

		if referencedFiles[fullPath] {
			continue
		}
		// File has no live registry references and is not today's active segment.
		info, statErr := fileutil.Stat(fullPath)
		size := int64(0)
		if statErr == nil {
			size = info.Size()
		}
		if removeErr := fileutil.RemoveFile(fullPath); removeErr != nil {
			if !fileutil.IsNotExist(removeErr) {
				res.Errors++
			}
			continue
		}
		res.FilesRemoved++
		res.BytesFreed += size
	}
	return res
}

// GCOrphanedStreamSegmentsForAllKinds runs GCOrphanedStreamSegmentsForKind for every
// stream-backed kind returned by StreamStorageEnabledKindsList. Returns a summary result
// and a count of kinds that encountered at least one per-file error.
func GCOrphanedStreamSegmentsForAllKinds(projectRoot string) ([]GCStreamSegmentResult, int) {
	kinds := StreamStorageEnabledKindsList()
	results := make([]GCStreamSegmentResult, 0, len(kinds))
	errKinds := 0
	for _, kind := range kinds {
		r := GCOrphanedStreamSegmentsForKind(projectRoot, kind)
		results = append(results, r)
		if r.Errors > 0 {
			errKinds++
		}
	}
	return results, errKinds
}
