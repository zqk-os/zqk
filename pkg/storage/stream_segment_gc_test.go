package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// setupTestSegmentDir creates the segment directory for a kind under tmpDir and returns its path.
func setupTestSegmentDir(t *testing.T, tmpDir, kind string) string {
	t.Helper()
	dir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StreamsDir, kind)
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir segment dir: %v", err)
	}
	return dir
}

// writeSegmentFile creates a named segment file with dummy content in dir.
func writeSegmentFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(`{"id":"test","kind":"audit_aggregation_metric"}`+"\n"), paths.FilePerm600); err != nil {
		t.Fatalf("write segment file: %v", err)
	}
	return p
}

// TestGCOrphanedStreamSegments_RemovesOrphanedFiles verifies that segment files with no live
// registry entries are deleted and referenced files are preserved.
func TestGCOrphanedStreamSegments_RemovesOrphanedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	kind := "audit_aggregation_metric"
	segDir := setupTestSegmentDir(t, tmpDir, kind)

	// Write two old segment files (dates in the past).
	orphanFile := writeSegmentFile(t, segDir, "2030-03-03_stream.json")
	liveFile := writeSegmentFile(t, segDir, "2030-03-04_stream.json")

	// Set up registry: only liveFile is referenced by a live entry.
	liveID := "AAM-live-001"
	liveOffset := int64(0)
	liveLoc := FormatStreamLocation(liveFile, liveOffset)
	if err := AppendStreamLocationToRegistry(tmpDir, kind, liveID, liveLoc); err != nil {
		t.Fatalf("AppendStreamLocationToRegistry: %v", err)
	}
	// orphanFile has no registry entry at all.

	res := GCOrphanedStreamSegmentsForKind(tmpDir, kind)

	if res.FilesRemoved != 1 {
		t.Errorf("expected 1 file removed, got %d", res.FilesRemoved)
	}
	if res.BytesFreed <= 0 {
		t.Errorf("expected BytesFreed > 0, got %d", res.BytesFreed)
	}
	if res.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", res.Errors)
	}
	if _, err := os.Stat(orphanFile); !os.IsNotExist(err) {
		t.Errorf("orphaned segment file should have been deleted: %s", orphanFile)
	}
	if _, err := os.Stat(liveFile); err != nil {
		t.Errorf("live segment file should still exist: %v", err)
	}
}

// TestGCOrphanedStreamSegments_PreservesTodayFile ensures today's active segment file is
// never deleted, even when it has no registry entries yet (in-flight appends protection).
func TestGCOrphanedStreamSegments_PreservesTodayFile(t *testing.T) {
	tmpDir := t.TempDir()
	kind := "audit_aggregation_metric"
	segDir := setupTestSegmentDir(t, tmpDir, kind)

	// Create today's segment file with no registry entries.
	todayName := zqktime.NowLayoutUTC(zqktime.LayoutDate) + "_stream.json"
	todayFile := writeSegmentFile(t, segDir, todayName)

	res := GCOrphanedStreamSegmentsForKind(tmpDir, kind)

	if res.FilesRemoved != 0 {
		t.Errorf("today's segment file must not be removed, but FilesRemoved=%d", res.FilesRemoved)
	}
	if _, err := os.Stat(todayFile); err != nil {
		t.Errorf("today's segment file was unexpectedly deleted: %v", err)
	}
}

// TestGCOrphanedStreamSegments_AllOrphaned verifies that all old files are removed when no
// live registry entries exist (e.g. full retention cycle completed).
func TestGCOrphanedStreamSegments_AllOrphaned(t *testing.T) {
	tmpDir := t.TempDir()
	kind := "audit_aggregation_metric"
	segDir := setupTestSegmentDir(t, tmpDir, kind)

	// Write three old segment files, none referenced in the registry.
	for _, name := range []string{"2030-03-03_stream.json", "2030-03-04_stream.json", "2030-03-05_stream.json"} {
		writeSegmentFile(t, segDir, name)
	}

	res := GCOrphanedStreamSegmentsForKind(tmpDir, kind)

	if res.FilesRemoved != 3 {
		t.Errorf("expected 3 files removed, got %d", res.FilesRemoved)
	}
	remaining, _ := os.ReadDir(segDir)
	for _, e := range remaining {
		t.Errorf("unexpected file still present after GC: %s", e.Name())
	}
}

// TestGCOrphanedStreamSegments_EmptyDir verifies GC is a no-op when the segment directory
// does not exist.
func TestGCOrphanedStreamSegments_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	res := GCOrphanedStreamSegmentsForKind(tmpDir, "audit_aggregation_metric")
	if res.FilesRemoved != 0 || res.Errors != 0 {
		t.Errorf("expected no-op on missing dir, got removed=%d errors=%d", res.FilesRemoved, res.Errors)
	}
}

// TestGCOrphanedStreamSegments_SoftDeletedIDsStillOrphaned verifies that a file referenced
// only by soft-deleted IDs (in stream_deleted) is treated as orphaned and removed.
func TestGCOrphanedStreamSegments_SoftDeletedIDsStillOrphaned(t *testing.T) {
	tmpDir := t.TempDir()
	kind := "audit_aggregation_metric"
	segDir := setupTestSegmentDir(t, tmpDir, kind)

	oldFile := writeSegmentFile(t, segDir, "2030-03-03_stream.json")

	// Register an ID pointing to the old file, then soft-delete it.
	id := "AAM-deleted-001"
	loc := FormatStreamLocation(oldFile, 0)
	if err := AppendStreamLocationToRegistry(tmpDir, kind, id, loc); err != nil {
		t.Fatalf("AppendStreamLocationToRegistry: %v", err)
	}
	if err := AddStreamDeletedID(tmpDir, kind, id); err != nil {
		t.Fatalf("AddStreamDeletedID: %v", err)
	}

	// Compact registry first (removes deleted ID from live set).
	if err := CompactStreamRegistryForKind(tmpDir, kind); err != nil {
		t.Fatalf("CompactStreamRegistryForKind: %v", err)
	}

	res := GCOrphanedStreamSegmentsForKind(tmpDir, kind)

	if res.FilesRemoved != 1 {
		t.Errorf("expected 1 file removed after registry compaction, got %d", res.FilesRemoved)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("orphaned segment file should have been deleted after registry compaction")
	}
}
