package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	traceBaseName     = "mcp-trace.log"
	testRoundtripMsg  = "verify-content-roundtrip"
	concurrencyProbe  = "concurrency-probe"
	rotationPayload   = "rotate-payload"
)

// rotatedTraceFiles returns all rotated trace files in dir for the given
// active base file name (pattern "<base>-<timestamp>-<n><ext>").
func rotatedTraceFiles(t *testing.T, dir string, base string) []string {
	t.Helper()
	baseName := filepath.Base(base)
	ext := filepath.Ext(baseName)
	baseNoExt := strings.TrimSuffix(baseName, ext)
	matches, err := filepath.Glob(filepath.Join(dir, baseNoExt+"-*-*"+ext))
	if err != nil {
		t.Fatalf("glob rotated trace files: %v", err)
	}
	return matches
}

// activeAndRotatedContent concatenates the active trace file (if any) with
// every rotated file's content so assertions can verify no bytes were lost.
func activeAndRotatedContent(t *testing.T, dir string, base string) string {
	t.Helper()
	var all strings.Builder
	if data, rerr := os.ReadFile(filepath.Clean(base)); rerr == nil { // #nosec G304
		all.Write(data)
	}
	for _, p := range rotatedTraceFiles(t, dir, base) {
		data, rerr := os.ReadFile(filepath.Clean(p)) // #nosec G304
		if rerr != nil {
			t.Fatalf("read rotated file %s: %v", p, rerr)
		}
		all.Write(data)
	}
	return all.String()
}

// TestRollingTraceWriter_DefaultConfig verifies the documented defaults.
func TestRollingTraceWriter_DefaultConfig(t *testing.T) {
	cfg := DefaultRollingTraceConfig()
	if cfg.MaxSize != 10*1024*1024 {
		t.Errorf("default MaxSize: want 10485760, got %d", cfg.MaxSize)
	}
	if cfg.MaxFiles != 5 {
		t.Errorf("default MaxFiles: want 5, got %d", cfg.MaxFiles)
	}
}

// TestRollingTraceWriter_ZeroConfigFallsBackToDefaults verifies a zero-valued
// config is coerced to defaults instead of mis-configuring the writer.
func TestRollingTraceWriter_ZeroConfigFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, traceBaseName)

	w, err := NewRollingTraceWriter(base, RollingTraceConfig{})
	if err != nil {
		t.Fatalf("NewRollingTraceWriter(zero config): %v", err)
	}
	defer w.Close()

	if w.maxSize != DefaultRollingTraceConfig().MaxSize {
		t.Errorf("maxSize fallback: want %d, got %d", DefaultRollingTraceConfig().MaxSize, w.maxSize)
	}
	if w.maxFiles != DefaultRollingTraceConfig().MaxFiles {
		t.Errorf("maxFiles fallback: want %d, got %d", DefaultRollingTraceConfig().MaxFiles, w.maxFiles)
	}
}

// TestRollingTraceWriter_WritePersistsContent verifies a write/sync/close
// roundtrip is durably observable in the active file.
func TestRollingTraceWriter_WritePersistsContent(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, traceBaseName)

	w, err := NewRollingTraceWriter(base, RollingTraceConfig{
		MaxSize:  1 * 1024 * 1024,
		MaxFiles: 3,
	})
	if err != nil {
		t.Fatalf("NewRollingTraceWriter: %v", err)
	}

	payload := "[" + testRoundtripMsg + "]\n"
	n, werr := w.Write([]byte(payload))
	if werr != nil {
		t.Fatalf("Write: %v", werr)
	}
	if n != len(payload) {
		t.Errorf("Write: want %d bytes, got %d", len(payload), n)
	}

	if err := w.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, rerr := os.ReadFile(filepath.Clean(base)) // #nosec G304
	if rerr != nil {
		t.Fatalf("read active trace file: %v", rerr)
	}
	if !strings.Contains(string(data), testRoundtripMsg) {
		t.Errorf("active trace file missing roundtrip payload")
	}
}

// TestRollingTraceWriter_RotatesWhenMaxSizeExceeded verifies that a write
// exceeding MaxSize triggers rotation, that the payload remains recoverable
// across active+rotated files, and that a fresh active file is re-opened.
func TestRollingTraceWriter_RotatesWhenMaxSizeExceeded(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, traceBaseName)

	const maxSize = 64
	w, err := NewRollingTraceWriter(base, RollingTraceConfig{
		MaxSize:  int64(maxSize),
		MaxFiles: 5,
	})
	if err != nil {
		t.Fatalf("NewRollingTraceWriter: %v", err)
	}
	defer w.Close()

	big := "[" + rotationPayload + "]" + strings.Repeat("x", 512) + "\n"
	if _, werr := w.Write([]byte(big)); werr != nil {
		t.Fatalf("Write past MaxSize: %v", werr)
	}

	rotated := rotatedTraceFiles(t, dir, traceBaseName)
	if len(rotated) == 0 {
		t.Fatalf("expected at least one rotated file after exceeding MaxSize, none found in %s", dir)
	}

	if _, serr := os.Stat(base); serr != nil {
		t.Errorf("active trace file missing after rotation: %v", serr)
	}

	content := activeAndRotatedContent(t, dir, base)
	if !strings.Contains(content, rotationPayload) {
		t.Errorf("payload not recoverable across active+rotated files after rotation")
	}
}

// TestRollingTraceWriter_RetentionCapsRotatedFiles verifies the cleanup
// routine keeps at most MaxFiles rotated files after repeated rotations.
func TestRollingTraceWriter_RetentionCapsRotatedFiles(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, traceBaseName)

	const maxSize = 64
	const maxFiles = 2

	w, err := NewRollingTraceWriter(base, RollingTraceConfig{
		MaxSize:  int64(maxSize),
		MaxFiles: maxFiles,
	})
	if err != nil {
		t.Fatalf("NewRollingTraceWriter: %v", err)
	}
	defer w.Close()

	payload := "[" + rotationPayload + "]" + strings.Repeat("y", 512) + "\n"
	const rounds = 8
	for i := 0; i < rounds; i++ {
		if _, werr := w.Write([]byte(payload)); werr != nil {
			t.Fatalf("rotation round %d: %v", i, werr)
		}
	}

	rotated := rotatedTraceFiles(t, dir, traceBaseName)
	if len(rotated) > maxFiles {
		t.Errorf("retention: want at most %d rotated files, got %d: %v", maxFiles, len(rotated), rotated)
	}
}

// TestRollingTraceWriter_MixedWorkload_NoContentLoss simulates a mixed
// workload (small writes, oversize writes, more small writes) and asserts the
// union of surviving files still contains the expected marker count, proving
// rotation cleanup does not destroy data that was written before an
// oversize flush forced a rotation of the in-progress file.
func TestRollingTraceWriter_MixedWorkload_NoContentLoss(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, traceBaseName)

	const maxSize = 128
	w, err := NewRollingTraceWriter(base, RollingTraceConfig{
		MaxSize:  int64(maxSize),
		MaxFiles: 10,
	})
	if err != nil {
		t.Fatalf("NewRollingTraceWriter: %v", err)
	}
	defer w.Close()

	marker := "[" + concurrencyProbe + "]"
	// 3 small writes, then one 1KB oversize write (forces rotation), then 3 more.
	small := marker + "\n"
	oversize := marker + strings.Repeat("z", 1024) + "\n"

	for i := 0; i < 3; i++ {
		if _, werr := w.Write([]byte(small)); werr != nil {
			t.Fatalf("small write %d: %v", i, werr)
		}
	}
	if _, werr := w.Write([]byte(oversize)); werr != nil {
		t.Fatalf("oversize write: %v", werr)
	}
	for i := 0; i < 3; i++ {
		if _, werr := w.Write([]byte(small)); werr != nil {
			t.Fatalf("post-rotation write %d: %v", i, werr)
		}
	}

	content := activeAndRotatedContent(t, dir, base)
	got := strings.Count(content, concurrencyProbe)
	want := 7
	if got < want {
		t.Errorf("mixed workload: want %d probes retained, got %d", want, got)
	}
}

// TestRollingTraceWriter_CloseIdempotent verifies Close can be called
// multiple times without error (double-close must be a no-op).
func TestRollingTraceWriter_CloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, traceBaseName)

	w, err := NewRollingTraceWriter(base, RollingTraceConfig{
		MaxSize:  1024,
		MaxFiles: 2,
	})
	if err != nil {
		t.Fatalf("NewRollingTraceWriter: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close should be a no-op, got: %v", err)
	}
}
