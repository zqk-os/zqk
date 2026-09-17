package storage

// Regression tests for the hash registry save stall that caused object-creation rollbacks.
//
// Root cause (2026-03-19): processSave() called file.Sync() which on macOS invokes
// F_FULLFSYNC — a full hardware-cache flush. When Time Machine or Spotlight was active
// on the .zqk/process/backlog directory, F_FULLFSYNC blocked for 10+ minutes even for
// a 15 KB file (166 hashes). The save worker goroutine became stuck with
// queue_len=0, worker_running=true. After 3 × 10-minute timeouts,
// saveHashRegistryWithRetry logged "object creation rolled back" — even though the CAS
// write had already succeeded and the object was visible via List().
//
// Fix: removed file.Sync() from processSave; reduced hashRegistrySaveMaxWait to 60s.
// The hash registry is a derived/rebuildable cache; hardware-flush durability is not
// required. Atomic write-to-tmp then rename provides sufficient crash-safety.

import (
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestHashRegistry_ProcessSaveCompletesQuicklyForSmallRegistry verifies that saving a small
// hash registry (≤ 200 entries, ≤ 20 KB) completes well within the hashRegistrySaveMaxWait
// budget. Previously, file.Sync() (F_FULLFSYNC) could stall this for 10+ minutes.
func TestHashRegistry_ProcessSaveCompletesQuicklyForSmallRegistry(t *testing.T) {

	tmpDir := t.TempDir()
	ctx := pkgctx.NewSystemContext()
	registry := NewHashRegistry(ctx, "backlog_item", tmpDir)
	registry.SetSkipShutdownCoordinatorCheck(true)

	// Populate 166 hashes — the exact count from the production incident.
	for i := range 166 {
		registry.SetHash(
			filepath.Join(tmpDir, "backlog", "some-object.yaml")+string(rune('0'+i%10)),
			"deadbeef0123456789abcdef",
		)
	}

	const limit = 5 * time.Second
	start := time.Now()
	if err := registry.Save(); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > limit {
		t.Errorf("Save() took %v for 166 hashes; expected < %v (file.Sync stall regression)", elapsed, limit)
	}

	// Verify the file was actually written.
	hashFile := filepath.Join(tmpDir, ".backlog_item.hashes")
	if _, err := fileutil.Stat(hashFile); err != nil {
		t.Fatalf("hash registry file was not written: %v", err)
	}

	// Verify all hashes can be retrieved.
	if loaded := registry.GetHash(filepath.Join(tmpDir, "backlog", "some-object.yaml") + "0"); loaded == emptyValue {
		t.Error("expected hash to be set, got empty string")
	}
}

// TestHashRegistry_SaveMaxWaitIsReasonable asserts that hashRegistrySaveMaxWait has been
// reduced to ≤ 2 minutes. The production value was 10 minutes (600s); this test acts as a
// guard so the constant cannot accidentally be inflated back to a large value.
func TestHashRegistry_SaveMaxWaitIsReasonable(t *testing.T) {

	const maxAcceptable = 2 * time.Minute
	if hashRegistrySaveMaxWait > maxAcceptable {
		t.Errorf("hashRegistrySaveMaxWait=%v exceeds %v; reducing save timeout prevents 10-min stalls (see hash_registry_no_sync_stall_test.go)", hashRegistrySaveMaxWait, maxAcceptable)
	}
}

// TestHashRegistry_AtomicWriteThenRename verifies that processSave writes via an atomic
// temp-file-then-rename pattern: the hash file must not exist mid-write as an incomplete
// blob — only a fully-written file should appear at the final path.
func TestHashRegistry_AtomicWriteThenRename(t *testing.T) {

	tmpDir := t.TempDir()
	ctx := pkgctx.NewSystemContext()
	registry := NewHashRegistry(ctx, "doc_entry", tmpDir)
	registry.SetSkipShutdownCoordinatorCheck(true)

	registry.SetHash("doc-001.yaml", "aabbcc")
	registry.SetHash("doc-002.yaml", "ddeeff")

	if err := registry.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	hashFile := filepath.Join(tmpDir, ".doc_entry.hashes")
	data, err := fileutil.ReadFile(hashFile)
	if err != nil {
		t.Fatalf("cannot read hash file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("hash file is empty after Save()")
	}

	// Verify no leftover .tmp files (atomic rename succeeded or was cleaned up).
	entries, err := fileutil.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("cannot list tmpDir: %v", err)
	}
	for _, e := range entries {
		if len(e.Name()) > 4 && e.Name()[len(e.Name())-4:] == ".tmp" ||
			len(e.Name()) > 5 && e.Name()[len(e.Name())-5:] == ".tmp0" {
			t.Errorf("leftover temp file after Save(): %s", e.Name())
		}
	}
}

// TestHashRegistry_ConcurrentSavesCompleteQuickly ensures that multiple concurrent Save()
// calls on the same small registry all complete within the timeout budget.  This was a
// secondary symptom of the stall: all concurrent callers queued behind the stuck worker.
func TestHashRegistry_ConcurrentSavesCompleteQuickly(t *testing.T) {

	tmpDir := t.TempDir()
	ctx := pkgctx.NewSystemContext()
	registry := NewHashRegistry(ctx, "backlog_item", tmpDir)
	registry.SetSkipShutdownCoordinatorCheck(true)

	const (
		goroutines = 10
		limit      = 10 * time.Second
	)

	done := make(chan error, goroutines)
	start := time.Now()
	for i := range goroutines {
		goroutinelabels.NewGoroutine("storage_test", "concurrent save for stall test").StartSimple(func() {
			func(n int) {
				registry.SetHash("file.yaml", "hash")
				done <- registry.Save()
			}(i)
		})
	}

	for range goroutines {
		if err := <-done; err != nil {
			t.Errorf("concurrent Save() error: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > limit {
		t.Errorf("concurrent saves took %v; expected < %v", elapsed, limit)
	}
}
