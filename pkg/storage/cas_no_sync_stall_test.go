package storage

// Regression tests for CAS writeFileWithSync stalling on macOS F_FULLFSYNC.
//
// Root cause (2026-03-19): writeFileWithSync() called tmp.Sync() and then a
// directory Sync() after hardlinking the temp file. On macOS, both calls invoke
// F_FULLFSYNC — a full hardware-cache flush — which can stall 1-2 seconds per
// file. For 327 bulk CAS creates (scan-tests --all), this meant 5-10 minutes of
// blocking in the main goroutine, causing the command to hang and get killed.
//
// Fix: removed tmp.Sync() and the directory Sync() from writeFileWithSync.
// APFS journaling provides sufficient crash-safety; application-level fsync is
// not required for CAS objects (rebuild via `system check --auto-fix` if needed).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCAS_WriteFileWithSync_CompletesQuicklyForBulkCreate verifies that creating
// 327 CAS objects (the scan-tests --all bundle count) completes well under 60 seconds.
// Previously, each tmp.Sync() stalled ~1-2 s on macOS, totalling 5-10 minutes.
func TestCAS_WriteFileWithSync_CompletesQuicklyForBulkCreate(t *testing.T) {

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "scheduler_jobs")

	const (
		objectCount = 327
		limit       = 60 * time.Second
	)

	casQueue := NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := NewContentAddressableStorage(kindDir, "scheduler_job", casQueue)

	start := time.Now()
	for i := range objectCount {
		data := casTestMakeJobYAML(i)
		hash := CalculateSHA256Hash(data)
		destPath := filepath.Join(kindDir, hash+".yaml")
		if err := cas.writeFileWithSync(destPath, data); err != nil {
			t.Fatalf("writeFileWithSync(%d): %v", i, err)
		}
	}
	elapsed := time.Since(start)

	if elapsed > limit {
		t.Errorf("writeFileWithSync for %d objects took %v; expected < %v (F_FULLFSYNC regression)", objectCount, elapsed, limit)
	}
}

// TestCAS_WriteFileWithSync_NoTmpFilesLeaked verifies that the temp file created
// during writeFileWithSync is cleaned up after a successful write.
func TestCAS_WriteFileWithSync_NoTmpFilesLeaked(t *testing.T) {

	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "scheduler_jobs")
	casQueue := NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := NewContentAddressableStorage(kindDir, "scheduler_job", casQueue)

	data := []byte("kind: scheduler_job\nid: SCH-run-test\n")
	hash := CalculateSHA256Hash(data)
	finalPath := filepath.Join(kindDir, hash+".yaml")
	if err := cas.writeFileWithSync(finalPath, data); err != nil {
		t.Fatalf("writeFileWithSync: %v", err)
	}

	entries, err := os.ReadDir(kindDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") || strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after writeFileWithSync: %s", e.Name())
		}
	}
}

// casTestMakeJobYAML returns a small but distinct YAML payload for each index
// so each CAS create produces a different hash (different file on disk).
func casTestMakeJobYAML(i int) []byte {
	id := "SCH-run-bundle-"
	n := i
	if n == 0 {
		id += "0"
	} else {
		buf := make([]byte, 0, 8)
		for n > 0 {
			buf = append([]byte{byte('0' + n%10)}, buf...)
			n /= 10
		}
		id += string(buf)
	}
	return []byte("kind: scheduler_job\nid: " + id + "\nstatus: active\n")
}
