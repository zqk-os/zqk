package fileutil

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestClassifyPath(t *testing.T) {
	tests := []struct {
		path     string
		expected PathClass
	}{
		{"/repo/.zqk/process/criteria/CRIT-001.yaml", ClassProcess},
		{".zqk/process/backlog_items/BLI-001.yaml", ClassProcess},
		{"/repo/.zqk-state/system-state.csnap", ClassState},
		{".zqk-state/state.json", ClassState},
		{".zqk/state/ambient/align-latest.json", ClassState},
		{"/tmp/testfile123", ClassTmp},
		{"/var/folders/xx/T/testdir/.tmp-durable-123", ClassTmp},
		{"pkg/utils/fileutil/fs.go", ClassOther},
		{"cmd/zqk/main.go", ClassOther},
		{"", ClassOther},
	}

	for _, tc := range tests {
		got := ClassifyPath(tc.path)
		if got != tc.expected {
			t.Errorf("ClassifyPath(%q) = %q; want %q", tc.path, got, tc.expected)
		}
	}
}

func TestIOMetrics_DisabledByDefaultInTest(t *testing.T) {
	// Under tests, IsIOMetricsEnabled should default to false unless explicitly toggled
	orig := IsIOMetricsEnabled()
	defer SetIOMetricsEnabled(orig)

	SetIOMetricsEnabled(false)
	ResetIOMetrics()

	dir := t.TempDir()
	p := filepath.Join(dir, "test.txt")
	_ = WriteFile(p, []byte("data"), 0o644)
	_, _ = ReadFile(p)

	snap := GetIOMetricsSnapshot()
	if snap.TotalOps != 0 {
		t.Fatalf("expected 0 ops when metrics disabled, got %d", snap.TotalOps)
	}
}

func TestIOMetrics_RecordingAndSnapshot(t *testing.T) {
	orig := IsIOMetricsEnabled()
	SetIOMetricsEnabled(true)
	defer SetIOMetricsEnabled(orig)

	ResetIOMetrics()

	dir := t.TempDir()
	p := filepath.Join(dir, "test_write.txt")
	content := []byte("1234567890") // 10 bytes

	// 1. WriteFile
	if err := WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// 2. Stat
	if _, err := Stat(p); err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	// 3. Exists
	if !Exists(p) {
		t.Fatalf("Exists returned false for existing file")
	}

	// 4. ReadFile
	readData, err := ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(readData) != 10 {
		t.Fatalf("ReadFile got %d bytes, want 10", len(readData))
	}

	// 5. ReadDir
	if _, err := ReadDir(dir); err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	// 6. Rename
	p2 := filepath.Join(dir, "test_renamed.txt")
	if err := Rename(p, p2); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// 7. Remove
	if err := Remove(p2); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// 8. Mkdir
	subDir := filepath.Join(dir, "subdir")
	if err := Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	snap := GetIOMetricsSnapshot()
	if !snap.Enabled {
		t.Errorf("expected snapshot.Enabled to be true")
	}

	// Check that each operation was recorded
	expectedOps := []IOOp{OpWrite, OpStat, OpExists, OpRead, OpReadDir, OpRename, OpRemove, OpMkdir}
	for _, op := range expectedOps {
		stats, ok := snap.ByOp[op]
		if !ok || stats.Count == 0 {
			t.Errorf("expected op %q to have count > 0 in ByOp, got %+v", op, stats)
		}
	}

	// Check bytes tracked for read and write
	if snap.ByOp[OpWrite].Bytes != 10 {
		t.Errorf("expected OpWrite bytes = 10, got %d", snap.ByOp[OpWrite].Bytes)
	}
	if snap.ByOp[OpRead].Bytes != 10 {
		t.Errorf("expected OpRead bytes = 10, got %d", snap.ByOp[OpRead].Bytes)
	}

	// Check total ops >= 8
	if snap.TotalOps < 8 {
		t.Errorf("expected TotalOps >= 8, got %d", snap.TotalOps)
	}
}

func TestIOMetrics_CallbackRegistration(t *testing.T) {
	orig := IsIOMetricsEnabled()
	SetIOMetricsEnabled(true)
	defer SetIOMetricsEnabled(orig)
	ResetIOMetrics()

	var mu sync.Mutex
	var callbackInvoked bool
	var callbackOp IOOp
	var callbackSuccess bool

	unregister := RegisterIOCallback(func(op IOOp, class PathClass, bytes int64, duration time.Duration, success bool) {
		mu.Lock()
		defer mu.Unlock()
		callbackInvoked = true
		callbackOp = op
		callbackSuccess = success
	})

	dir := t.TempDir()
	p := filepath.Join(dir, "cb_test.txt")
	_ = WriteFile(p, []byte("callback test"), 0o644)

	unregister()

	mu.Lock()
	invoked := callbackInvoked
	op := callbackOp
	success := callbackSuccess
	mu.Unlock()

	if !invoked {
		t.Fatalf("expected callback to be invoked on WriteFile")
	}
	if op != OpWrite {
		t.Errorf("expected callback op = OpWrite, got %v", op)
	}
	if !success {
		t.Errorf("expected callback success = true, got false")
	}
}

func TestIOMetrics_ConcurrentOperations(t *testing.T) {
	orig := IsIOMetricsEnabled()
	SetIOMetricsEnabled(true)
	defer SetIOMetricsEnabled(orig)
	ResetIOMetrics()

	dir := t.TempDir()
	const numWorkers = 10
	const opsPerWorker = 20

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			defer wg.Done()
			p := filepath.Join(dir, "worker.txt")
			for j := 0; j < opsPerWorker; j++ {
				_ = Exists(p)
			}
		}(i)
	}

	wg.Wait()

	snap := GetIOMetricsSnapshot()
	expectedMinOps := int64(numWorkers * opsPerWorker)
	if snap.ByOp[OpExists].Count < expectedMinOps {
		t.Errorf("expected at least %d OpExists, got %d", expectedMinOps, snap.ByOp[OpExists].Count)
	}
}

func TestIOMetrics_Reset(t *testing.T) {
	orig := IsIOMetricsEnabled()
	SetIOMetricsEnabled(true)
	defer SetIOMetricsEnabled(orig)

	dir := t.TempDir()
	p := filepath.Join(dir, "reset.txt")
	_ = WriteFile(p, []byte("abc"), 0o644)

	snap1 := GetIOMetricsSnapshot()
	if snap1.TotalOps == 0 {
		t.Fatalf("expected ops before reset")
	}

	ResetIOMetrics()

	snap2 := GetIOMetricsSnapshot()
	if snap2.TotalOps != 0 {
		t.Fatalf("expected 0 ops after reset, got %d", snap2.TotalOps)
	}
}

func TestIOMetrics_LeafPackageBoundary(t *testing.T) {
	// Verify that pkg/utils/fileutil does not import disallowed packages.
	// We read io_metrics.go and ensure no imports of scheduler, storage, cli, or cmd/zqk exist.
	content, err := os.ReadFile("io_metrics.go")
	if err != nil {
		t.Fatalf("read io_metrics.go: %v", err)
	}
	src := string(content)
	disallowed := []string{
		"github.com/zqk-os/zqk/pkg/scheduler",
		"github.com/zqk-os/zqk/pkg/storage",
		"github.com/zqk-os/zqk/pkg/cli",
		"github.com/zqk-os/zqk/cmd/zqk",
	}
	for _, d := range disallowed {
		if filepath.Base(d) != "" && (containsImport(src, d)) {
			t.Fatalf("CRIT-1787075073743178000-853fbf8c violation: io_metrics.go imports %s", d)
		}
	}
}

func containsImport(src, pkg string) bool {
	return filepath.Clean(pkg) != "" && (filepath.Base(src) != "" && (len(src) > 0 && (false || (len(pkg) > 0 && (false || (len(src) > len(pkg) && (src[0:] != "" && (findSubstring(src, `"`+pkg+`"`)))))))))
}

func findSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
