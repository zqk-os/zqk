package walutil

import (
	"bufio"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type failSyncWriter struct {
	err error
}

func (w *failSyncWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestReadWriteJSONFile_ErrorsAndEdgeCases(t *testing.T) {
	dir := t.TempDir()

	// Reading a directory path causes ReadFile error that is not IsNotExist
	var out map[string]any
	if err := ReadJSONFile(dir, &out); err == nil {
		t.Fatal("expected error reading directory as JSON file, got nil")
	}

	// Marshaling unsupported type triggers marshal error
	unsupported := map[string]any{"bad": math.NaN()}
	if err := WriteJSONFileAtomic(filepath.Join(dir, "bad.json"), unsupported, paths.FilePerm644); err == nil {
		t.Fatal("expected error marshaling NaN, got nil")
	}

	// Writing to an impossible path
	if err := WriteJSONFileAtomic(filepath.Join(dir, "no-such-dir", "sub", "x.json"), map[string]string{"k": "v"}, paths.FilePerm644); err != nil {
		// WriteDurableFile creates parent directories, so test writing where parent is a regular file
		parentFile := filepath.Join(dir, "blocked_dir")
		if werr := fileutil.WriteFile(parentFile, []byte("blocker"), paths.FilePerm644); werr != nil {
			t.Fatalf("failed to write blocker file: %v", werr)
		}
		blockedPath := filepath.Join(parentFile, "child.json")
		if berr := WriteJSONFileAtomic(blockedPath, map[string]string{"k": "v"}, paths.FilePerm644); berr == nil {
			t.Fatal("expected error writing atomic JSON under file path, got nil")
		}
	}
}

func TestAppendJSONLine_LifecycleAndErrors(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "append_test.jsonl")

	// Successful multiple appends
	for i := 1; i <= 3; i++ {
		entry := map[string]any{"seq": i, "msg": "entry"}
		if err := AppendJSONLine(logPath, entry); err != nil {
			t.Fatalf("AppendJSONLine(%d) failed: %v", i, err)
		}
	}

	// Verify scanned lines count
	var scannedCount int
	if err := ScanFileLines(logPath, 1024, func(line []byte) {
		scannedCount++
	}); err != nil {
		t.Fatalf("ScanFileLines failed: %v", err)
	}
	if scannedCount != 3 {
		t.Fatalf("expected 3 scanned lines, got %d", scannedCount)
	}

	// Marshal error
	if err := AppendJSONLine(logPath, map[string]any{"bad": math.NaN()}); err == nil {
		t.Fatal("expected marshal error for NaN, got nil")
	}

	// Directory creation error (parent path is a file)
	blocker := filepath.Join(dir, "file_as_dir")
	if err := fileutil.WriteFile(blocker, []byte("data"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}
	impossiblePath := filepath.Join(blocker, "sub", "test.jsonl")
	if err := AppendJSONLine(impossiblePath, map[string]any{"seq": 1}); err == nil {
		t.Fatal("expected MkdirAll error, got nil")
	}

	// OpenFile error (path is an existing directory)
	subDir := filepath.Join(dir, "subdir_as_target")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := AppendJSONLine(subDir, map[string]any{"seq": 1}); err == nil {
		t.Fatal("expected OpenFile error on directory target, got nil")
	}
}

func TestOpenJSONLineWAL_DirectoryAndFileErrors(t *testing.T) {
	dir := t.TempDir()

	// Cause MkdirAll to fail by placing a regular file at .zqk
	zqkFile := filepath.Join(dir, paths.ProjectDataDir)
	if err := fileutil.WriteFile(zqkFile, []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	_, err := OpenJSONLineWAL[map[string]any](dir, "test.jsonl", ".ck", 0)
	if err == nil {
		t.Fatal("expected error opening WAL when .zqk is a file, got nil")
	}

	// Cause OpenFile to fail by creating target WAL file as a directory
	dir2 := t.TempDir()
	walDir := filepath.Join(dir2, paths.ProjectDataDir, paths.WalDir)
	if merr := fileutil.MkdirAll(walDir, paths.DirPerm755); merr != nil {
		t.Fatalf("failed to mkdir walDir: %v", merr)
	}
	targetDir := filepath.Join(walDir, "wal_as_dir.jsonl")
	if merr := fileutil.MkdirAll(targetDir, paths.DirPerm755); merr != nil {
		t.Fatalf("failed to create target as dir: %v", merr)
	}

	_, err = OpenJSONLineWAL[map[string]any](dir2, "wal_as_dir.jsonl", ".ck", 0)
	if err == nil {
		t.Fatal("expected error when WAL file is an existing directory, got nil")
	}
}

func TestOpenJSONLineWAL_ReadLastSeqError(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, paths.ProjectDataDir, paths.WalDir)
	if err := fileutil.MkdirAll(walDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create walDir: %v", err)
	}
	walPath := filepath.Join(walDir, "oversized.jsonl")
	// Scanner buffer cap is 4096; a line exceeding 8192 bytes will exceed maxLineSize
	hugeLine := append([]byte("{\"seq\":1,\"data\":\""), append(make([]byte, 8192), []byte("\"}\n")...)...)
	if err := fileutil.WriteFile(walPath, hugeLine, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write oversized WAL file: %v", err)
	}

	// Opening with maxLineSize=5000 will cause ReadLastSeqFromJSONLines to return ErrTooLong
	_, err := OpenJSONLineWAL[map[string]any](dir, "oversized.jsonl", ".ck", 5000)
	if err == nil {
		t.Fatal("expected error from ReadLastSeqFromJSONLines due to oversized line, got nil")
	}

	// Test nil nextSeqFn fallback error path in openJSONLineWALWithNextSeqAndSyncDir
	_, err = openJSONLineWALWithNextSeqAndSyncDir[map[string]any](
		dir,
		"oversized.jsonl",
		".ck",
		5000,
		nil,
		fileutil.SyncDir,
	)
	if err == nil {
		t.Fatal("expected error from nil nextSeqFn fallback on oversized WAL file, got nil")
	}
}

func TestJSONLineWAL_AppendAndSyncErrors(t *testing.T) {
	dir := t.TempDir()
	w, err := OpenJSONLineWAL[map[string]any](dir, "append_err.jsonl", ".ck", 0)
	if err != nil {
		t.Fatalf("OpenJSONLineWAL failed: %v", err)
	}
	defer func() {
		if cerr := w.Close(); cerr != nil {
			t.Logf("close error in defer: %v", cerr)
		}
	}()

	// Marshal error on Append
	err = w.Append(func(seq int64, ts time.Time) map[string]any {
		return map[string]any{"seq": seq, "val": math.NaN()}
	}, 0)
	if err == nil {
		t.Fatal("expected marshal error on Append with NaN, got nil")
	}

	// Append with failing writer
	failW := &JSONLineWAL[map[string]any]{
		bw:      bufio.NewWriterSize(&failSyncWriter{err: errors.New("write failure")}, 1),
		nextSeq: 1,
	}
	if aerr := failW.Append(func(seq int64, ts time.Time) map[string]any {
		return map[string]any{"seq": seq}
	}, 0); aerr == nil {
		t.Fatal("expected write error on failW.Append, got nil")
	}

	// Sync with failing writer
	failSync := &JSONLineWAL[map[string]any]{
		bw: bufio.NewWriter(&failSyncWriter{err: errors.New("sync failure")}),
	}
	if serr := failSync.bw.WriteByte('x'); serr != nil {
		t.Fatalf("failed to buffer byte: %v", serr)
	}
	if serr := failSync.Sync(); serr == nil {
		t.Fatal("expected sync error from failSync.Sync, got nil")
	}
}

func TestJSONLineWAL_ConcurrentAppenders(t *testing.T) {
	dir := t.TempDir()
	w, err := OpenJSONLineWAL[map[string]any](dir, "concurrent.jsonl", ".ck", 0)
	if err != nil {
		t.Fatalf("OpenJSONLineWAL failed: %v", err)
	}

	const goroutines = 8
	const recordsPerGoroutine = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("wal_concurrent_writer", "concurrent WAL append testing")
		builder.WithWaitGroup(&wg).StartSimple(func() {
			for j := 0; j < recordsPerGoroutine; j++ {
				appErr := w.Append(func(seq int64, ts time.Time) map[string]any {
					return map[string]any{
						"seq":    seq,
						"worker": workerID,
						"iter":   j,
					}
				}, 0)
				if appErr != nil {
					t.Errorf("concurrent append failed: %v", appErr)
					return
				}
			}
		})
	}

	wg.Wait()
	if err := w.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify all records were written
	walPath := filepath.Join(dir, paths.ProjectDataDir, paths.WalDir, "concurrent.jsonl")
	lastSeq, err := ReadLastSeqFromJSONLines(walPath, 0)
	if err != nil {
		t.Fatalf("ReadLastSeqFromJSONLines failed: %v", err)
	}
	expectedTotal := int64(goroutines * recordsPerGoroutine)
	if lastSeq != expectedTotal {
		t.Fatalf("expected lastSeq %d, got %d", expectedTotal, lastSeq)
	}
}

func TestScanFileLines_Errors(t *testing.T) {
	dir := t.TempDir()

	// Path is a directory: Open returns error that is not IsNotExist
	err := ScanFileLines(dir, 1024, func(line []byte) {})
	if err == nil {
		t.Fatal("expected error scanning lines from directory path, got nil")
	}

	// Oversized line returns scanner error (must exceed defaultScannerBufCap 4096)
	oversizedPath := filepath.Join(dir, "large.txt")
	line := append(make([]byte, 8192), '\n')
	if werr := fileutil.WriteFile(oversizedPath, line, paths.FilePerm644); werr != nil {
		t.Fatalf("failed to write test file: %v", werr)
	}

	err = ScanFileLines(oversizedPath, 5000, func(l []byte) {})
	if err == nil {
		t.Fatal("expected scanner error for line exceeding maxLineSize, got nil")
	}
}

func TestExtractSeq_VariantsAndFallbacks(t *testing.T) {
	// ExtractSeqFromJSON with float64, int64, int, and other types
	if seq := ExtractSeqFromJSON([]byte(`{"seq": 100}`)); seq != 100 {
		t.Fatalf("expected 100, got %d", seq)
	}
	if seq := ExtractSeqFromJSON([]byte(`{"seq": "invalid_type"}`)); seq != 0 {
		t.Fatalf("expected 0 for string seq, got %d", seq)
	}
	if seq := ExtractSeqFromJSON([]byte(`{"seq": true}`)); seq != 0 {
		t.Fatalf("expected 0 for boolean seq, got %d", seq)
	}

	// ExtractSeqFromAnyJSON with unmarshable value
	if seq := ExtractSeqFromAnyJSON(map[string]any{"bad": math.NaN()}); seq != 0 {
		t.Fatalf("expected 0 for unmarshable input, got %d", seq)
	}

	// ExtractSeqFromJSONLine edge cases
	// No "seq" key
	if seq := ExtractSeqFromJSONLine([]byte(`{"other": 123}`)); seq != 0 {
		t.Fatalf("expected 0, got %d", seq)
	}
	// "seq" followed by non-digits
	if seq := ExtractSeqFromJSONLine([]byte(`{"seq": "abc"}`)); seq != 0 {
		t.Fatalf("expected 0 for non-numeric seq, got %d", seq)
	}
	// "seq" with integer overflow
	if seq := ExtractSeqFromJSONLine([]byte(`{"seq": 999999999999999999999999999999999999999999999999}`)); seq != 0 {
		t.Fatalf("expected 0 for overflowing seq, got %d", seq)
	}
}

func TestParseReplayCursorCheckpoint_InvalidCases(t *testing.T) {
	// Non-JSON, non-numeric input
	_, err := ParseReplayCursorCheckpoint([]byte("not_a_number"))
	if err == nil {
		t.Fatal("expected error parsing non-numeric string, got nil")
	}

	// Empty byte slice returns zero cursor
	c, err := ParseReplayCursorCheckpoint([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error on empty bytes: %v", err)
	}
	if c.Seq != 0 || c.Offset != 0 {
		t.Fatalf("expected zero cursor, got %+v", c)
	}
}

func TestReplayFromCursor_EdgeCases(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "replay_edges.wal")

	// Test CRLF handling: records with \r\n line endings
	crlfData := []byte("{\"seq\":1,\"msg\":\"one\"}\r\n{\"seq\":2,\"msg\":\"two\"}\r\n")
	if err := fileutil.WriteFile(walPath, crlfData, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write CRLF file: %v", err)
	}

	var delivered []int64
	parseFn := func(b []byte) (*map[string]any, error) {
		var m map[string]any
		if uerr := json.Unmarshal(b, &m); uerr != nil {
			return nil, uerr
		}
		return &m, nil
	}
	extractFn := func(m *map[string]any) int64 {
		return int64((*m)["seq"].(float64))
	}

	stats, err := ReplayFromCursor[map[string]any](
		walPath, 0, ReplayCursor{},
		parseFn, extractFn,
		func(m *map[string]any) error {
			delivered = append(delivered, extractFn(m))
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ReplayFromCursor with CRLF failed: %v", err)
	}
	if stats.Delivered != 2 || len(delivered) != 2 {
		t.Fatalf("expected 2 delivered records from CRLF, got delivered=%d list=%v", stats.Delivered, delivered)
	}

	// Test cursor seek bounds fallback (negative offset and offset > size)
	negStats, err := ReplayFromCursor[map[string]any](
		walPath, 0, ReplayCursor{Offset: -10},
		parseFn, extractFn,
		func(*map[string]any) error { return nil },
	)
	if err != nil {
		t.Fatalf("ReplayFromCursor with negative offset failed: %v", err)
	}
	if negStats.Delivered != 2 {
		t.Fatalf("expected fallback to 0 delivered 2 records, got %d", negStats.Delivered)
	}

	// Test already applied records skipped by extractSeq fallback
	// (When JSON line does not have standard "seq" key in raw string, but custom record has it)
	customPath := filepath.Join(dir, "custom_skip.wal")
	customData := []byte("{\"_s\":1}\n{\"_s\":2}\n{\"_s\":3}\n")
	if err := fileutil.WriteFile(customPath, customData, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write custom WAL: %v", err)
	}

	customParse := func(b []byte) (*map[string]any, error) {
		var m map[string]any
		if uerr := json.Unmarshal(b, &m); uerr != nil {
			return nil, uerr
		}
		return &m, nil
	}
	customExtract := func(m *map[string]any) int64 {
		return int64((*m)["_s"].(float64))
	}

	var customDelivered []int64
	cStats, err := ReplayFromCursor[map[string]any](
		customPath, 0, ReplayCursor{Seq: 2},
		customParse, customExtract,
		func(m *map[string]any) error {
			customDelivered = append(customDelivered, customExtract(m))
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ReplayFromCursor with custom seq failed: %v", err)
	}
	if cStats.Delivered != 1 || len(customDelivered) != 1 || customDelivered[0] != 3 {
		t.Fatalf("expected 1 record delivered (seq 3), got %v (stats: %+v)", customDelivered, cStats)
	}
}

func TestQuarantineCorruptedWALLine_Scenarios(t *testing.T) {
	// Empty path or payload is a no-op
	quarantineCorruptedWALLine("", []byte("corrupt"), "empty path test")
	quarantineCorruptedWALLine("/some/path", nil, "empty payload test")

	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	// Normal quarantine write
	quarantineCorruptedWALLine(walPath, []byte("invalid payload"), "corrupted line test")
	quarantineDir := filepath.Join(dir, "quarantine")
	entries, err := os.ReadDir(quarantineDir)
	if err != nil {
		t.Fatalf("failed to read quarantine directory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected quarantine file to be created, none found")
	}

	// Quarantine directory creation failure (quarantine exists as a file)
	dir2 := t.TempDir()
	blockerFile := filepath.Join(dir2, "quarantine")
	if err := fileutil.WriteFile(blockerFile, []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}
	walPath2 := filepath.Join(dir2, "wal.log")
	// Should log error and return without panicking
	quarantineCorruptedWALLine(walPath2, []byte("corrupt data"), "blocked quarantine dir")
}
