package system

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// GatherObjectCountByKindForReport returns a single source of truth for object counts by kind,
// aligned with object-count-report and retention targets. Use this for both object-count-report
// and improvement-report so counts are deterministic and comparable.
//
// When noCache is false: uses object ID cache CountByKind() and merges high-volume cache
// counts for stream-backed kinds (audit_event, change_journal_entry, etc.). Ensures
// high-volume cache is ready so stream-backed counts are accurate.
// When noCache is true: returns nil so callers use storage.Count() per kind instead.
func GatherObjectCountByKindForReport(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, noCache bool) map[string]int {
	if projectRoot == emptyValue {
		return nil
	}
	if noCache {
		return nil
	}
	ensureCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	_ = storage.EnsureHighVolumeEventCacheReady(ensureCtx, projectRoot, storageProvider, false)
	cancel()

	objCache := GetGlobalObjectIDCache()
	if objCache == nil || !TryLoadObjectIDCacheOnly(projectRoot) {
		return nil
	}
	raw := objCache.CountByKind()
	if raw == nil {
		return nil
	}
	// Clamp negative counts to 0 (safety net)
	out := make(map[string]int, len(raw))
	for k, n := range raw {
		if n < 0 {
			n = 0
		}
		out[k] = n
	}
	// Merge stream-backed kind counts from high-volume cache so object count matches stream storage.
	hvCache := storage.GetGlobalHighVolumeEventCache()
	if hvCache != nil && hvCache.IsPopulatedForProject(projectRoot) {
		for _, kind := range storage.StreamStorageEnabledKindsList() {
			out[kind] = hvCache.CountByKind(kind)
		}
	}
	return out
}

// StreamBackedKindsForReport returns the list of kinds that use stream storage (for report annotations).
func StreamBackedKindsForReport() []string {
	return storage.StreamStorageEnabledKindsList()
}

// StreamBackedDirsForReport returns dir names under .zqk/process where object count is from stream storage.
func StreamBackedDirsForReport() []string {
	dirs := make(map[string]bool)
	for _, kind := range storage.StreamStorageEnabledKindsList() {
		if d := objects.GetDirectoryFromKind(kind); d != emptyValue {
			dirs[d] = true
		}
	}
	out := make([]string, 0, len(dirs))
	for d := range dirs {
		out = append(out, d)
	}
	return out
}
