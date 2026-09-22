package wal

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraCoverage_ObjectWALRegistry(t *testing.T) {
	// Empty project root returns error
	walEmpty, err := AcquireObjectWAL("")
	if err == nil {
		t.Error("expected error for empty root")
		if walEmpty != nil {
			_ = ReleaseObjectWAL("", walEmpty)
		}
	}
	_ = ReleaseObjectWAL("", nil)

	tmpDir := t.TempDir()

	// Initial ref count
	if count := ObjectWALRefCountForTest(tmpDir); count != 0 {
		t.Errorf("expected ref count 0, got %d", count)
	}

	// First acquire
	w1, err := AcquireObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("AcquireObjectWAL(1): %v", err)
	}
	if count := ObjectWALRefCountForTest(tmpDir); count != 1 {
		t.Errorf("expected ref count 1, got %d", count)
	}

	// Second acquire (reuses instance)
	w2, err := AcquireObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("AcquireObjectWAL(2): %v", err)
	}
	if w1 != w2 {
		t.Errorf("expected identical WAL instances, got %p and %p", w1, w2)
	}
	if count := ObjectWALRefCountForTest(tmpDir); count != 2 {
		t.Errorf("expected ref count 2, got %d", count)
	}

	// Release non-registered instance
	otherWal, err := NewObjectWAL(t.TempDir())
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	if err := ReleaseObjectWAL(tmpDir, otherWal); err != nil {
		t.Errorf("ReleaseObjectWAL other: %v", err)
	}

	// Release one ref
	if err := ReleaseObjectWAL(tmpDir, w1); err != nil {
		t.Errorf("ReleaseObjectWAL(1): %v", err)
	}
	if count := ObjectWALRefCountForTest(tmpDir); count != 1 {
		t.Errorf("expected ref count 1 after first release, got %d", count)
	}

	// Release final ref
	if err := ReleaseObjectWAL(tmpDir, w2); err != nil {
		t.Errorf("ReleaseObjectWAL(2): %v", err)
	}
	if count := ObjectWALRefCountForTest(tmpDir); count != 0 {
		t.Errorf("expected ref count 0 after final release, got %d", count)
	}
}

func TestExtraCoverage_WriteBehindOwnerRegistry(t *testing.T) {
	// Empty checks
	if TryClaimWriteBehindOwner("", "owner1") {
		t.Error("expected false for empty root")
	}
	if TryClaimWriteBehindOwner("root", nil) {
		t.Error("expected false for nil owner")
	}
	ReleaseWriteBehindOwner("", "owner1")
	ReleaseWriteBehindOwner("root", nil)

	root := t.TempDir()
	owner1 := &struct{ name string }{"owner1"}
	owner2 := &struct{ name string }{"owner2"}

	if !TryClaimWriteBehindOwner(root, owner1) {
		t.Fatal("expected owner1 to claim successfully")
	}
	// Claiming again with same owner succeeds
	if !TryClaimWriteBehindOwner(root, owner1) {
		t.Fatal("expected owner1 re-claim to succeed")
	}
	// Different owner fails
	if TryClaimWriteBehindOwner(root, owner2) {
		t.Fatal("expected owner2 to fail claiming already-owned root")
	}

	LogWriteBehindOwnerSkipped(root)

	// Releasing different owner does not release
	ReleaseWriteBehindOwner(root, owner2)
	if TryClaimWriteBehindOwner(root, owner2) {
		t.Fatal("expected owner2 still unable to claim")
	}

	// Release actual owner
	ReleaseWriteBehindOwner(root, owner1)
	if !TryClaimWriteBehindOwner(root, owner2) {
		t.Fatal("expected owner2 to succeed after release")
	}
	ReleaseWriteBehindOwner(root, owner2)
}

func TestExtraCoverage_CompactInPlaceAndReopen(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	defer wal.Close()

	if cp := wal.GetCheckpointPath(); cp == "" {
		t.Error("expected non-empty checkpoint path")
	}

	recs := []*WALRecord{
		{Op: "create", Kind: "doc_entry", ID: "doc-001", DataB64: "e2lkOiAiZG9jLTAwMSJ9"},
		{Op: "create", Kind: "doc_entry", ID: "doc-002", DataB64: "e2lkOiAiZG9jLTAwMiJ9"},
	}
	if err := wal.AppendBatchWithSync(recs); err != nil {
		t.Fatalf("AppendBatchWithSync: %v", err)
	}

	singleRec := &WALRecord{Op: "create", Kind: "doc_entry", ID: "doc-003", DataB64: "e2lkOiAiZG9jLTAwMyJ9"}
	if err := wal.AppendWithSync(singleRec); err != nil {
		t.Fatalf("AppendWithSync: %v", err)
	}

	// Write checkpoint seq = 1
	if err := WriteAppliedSeq(tmpDir, recs[0].Seq); err != nil {
		t.Fatalf("WriteAppliedSeq: %v", err)
	}

	// Compact in place
	if err := wal.CompactInPlace(tmpDir); err != nil {
		t.Fatalf("CompactInPlace: %v", err)
	}

	// Reopen after compact directly
	if err := wal.ReopenAfterCompact(); err != nil {
		t.Fatalf("ReopenAfterCompact: %v", err)
	}
}

func TestExtraCoverage_CompactWAL_EdgeCases(t *testing.T) {
	// Empty project root
	if err := CompactWAL(""); err == nil {
		t.Error("expected error for empty project root")
	}

	// Project root with no WAL file
	emptyDir := t.TempDir()
	if err := CompactWAL(emptyDir); err != nil {
		t.Errorf("expected nil for missing WAL file, got: %v", err)
	}

	// Project root where all records are unapplied (keptCount == totalRecords)
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	rec := &WALRecord{Op: "create", Kind: "doc_entry", ID: "doc-100", DataB64: "e2lkOiAiZG9jLTEwMCJ9"}
	if err := wal.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	_ = wal.Sync()
	_ = wal.Close()

	// Compact when appliedSeq is 0 and records exist (no-op path)
	if err := CompactWAL(tmpDir); err != nil {
		t.Fatalf("CompactWAL with zero appliedSeq: %v", err)
	}
}

func TestExtraCoverage_ReadLastSeqFromTail_EdgeCases(t *testing.T) {
	// Non-existent
	if _, err := ReadLastSeqFromTail(filepath.Join(t.TempDir(), "nonexistent")); err == nil {
		t.Error("expected error for nonexistent")
	}

	// Empty file
	tmpDir := t.TempDir()
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	if err := fileutil.MkdirAll(walDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	walPath := filepath.Join(walDir, objectWALFileName)
	if err := fileutil.WriteFile(walPath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	seq, err := ReadLastSeqFromTail(tmpDir)
	if err != nil {
		t.Fatalf("ReadLastSeqFromTail empty file: %v", err)
	}
	if seq != 0 {
		t.Errorf("expected seq 0, got %d", seq)
	}

	// File with blank lines and valid record
	content := fmt.Sprintf("\n   \n{\"seq\":42,\"op\":\"create\",\"kind\":\"item\",\"id\":\"it-1\"}\n\n")
	if err := fileutil.WriteFile(walPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	seq, err = ReadLastSeqFromTail(tmpDir)
	if err != nil {
		t.Fatalf("ReadLastSeqFromTail: %v", err)
	}
	if seq != 42 {
		t.Errorf("expected seq 42, got %d", seq)
	}
}

func TestExtraCoverage_SanitizeTornTailOnOpen(t *testing.T) {
	tmpDir := t.TempDir()
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	_ = fileutil.MkdirAll(walDir, paths.DirPerm755)
	walPath := filepath.Join(walDir, objectWALFileName)

	// Write valid line followed by a torn line without trailing newline
	tornContent := "{\"seq\":1,\"op\":\"create\",\"kind\":\"item\",\"id\":\"it-1\"}\n{\"seq\":2,\"op\":\"torn..."
	if err := fileutil.WriteFile(walPath, []byte(tornContent), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// NewObjectWAL should sanitize the torn tail cleanly
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL with torn tail: %v", err)
	}
	_ = wal.Close()

	// Read content and ensure trailing newline
	cleaned, err := fileutil.ReadFile(walPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(cleaned) == 0 || cleaned[len(cleaned)-1] != '\n' {
		t.Errorf("expected cleaned file to end with newline, got %q", string(cleaned))
	}
}

func TestExtraCoverage_MigrateObjectWALToCanonicalNames(t *testing.T) {
	tmpDir := t.TempDir()
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	_ = fileutil.MkdirAll(walDir, paths.DirPerm755)

	// Case 1: Empty projectRoot returns nil
	if err := migrateObjectWALToCanonicalNames(""); err != nil {
		t.Errorf("expected nil for empty root, got %v", err)
	}

	// Case 2: Legacy object_wal exists with content, and empty object.wal exists
	legacyPath := filepath.Join(walDir, legacyObjectWALFileName)
	canonicalPath := filepath.Join(walDir, objectWALFileName)
	_ = fileutil.WriteFile(legacyPath, []byte("legacy data"), paths.FilePerm644)
	_ = fileutil.WriteFile(canonicalPath, []byte(""), paths.FilePerm644)

	if err := migrateObjectWALToCanonicalNames(tmpDir); err != nil {
		t.Fatalf("migrateObjectWALToCanonicalNames: %v", err)
	}

	// Legacy file should have been renamed to canonicalPath
	if fileutil.Exists(legacyPath) {
		t.Error("expected legacy path to be removed/renamed")
	}
	content, err := fileutil.ReadFile(canonicalPath)
	if err != nil || string(content) != "legacy data" {
		t.Errorf("expected canonical content 'legacy data', got %q, err %v", string(content), err)
	}

	// ReadAppliedSeq & WriteAppliedSeq edge cases
	if seq, err := ReadAppliedSeq(""); err != nil || seq != 0 {
		t.Errorf("expected (0, nil) for empty root, got (%d, %v)", seq, err)
	}
	if err := WriteAppliedSeq("", 100); err != nil {
		t.Errorf("expected nil for empty root WriteAppliedSeq, got %v", err)
	}
	// Corrupt checkpoint file
	ckPath := filepath.Join(walDir, objectWALFileName+objectWALCheckpointExt)
	_ = fileutil.WriteFile(ckPath, []byte("invalid json"), paths.FilePerm644)
	if _, err := ReadAppliedSeq(tmpDir); err == nil {
		t.Error("expected error for corrupt checkpoint file")
	}

	// ReplayWALChunk edge cases
	if replayed, _, err := ReplayWALChunk("", 0, 10, nil); err != nil || replayed != 0 {
		t.Errorf("expected (0, 0, nil) for empty root, got (%d, %v)", replayed, err)
	}

	// ReplayWALChunk with limit and with callback error
	chunkDir := t.TempDir()
	chunkWal, err := NewObjectWAL(chunkDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	_ = chunkWal.Append(&WALRecord{Op: "create", Kind: "doc", ID: "d-1"})
	_ = chunkWal.Append(&WALRecord{Op: "create", Kind: "doc", ID: "d-2"})
	_ = chunkWal.Sync()
	_ = chunkWal.Close()

	replayed, _, err := ReplayWALChunk(chunkDir, 0, 1, func(r *WALRecord) error { return nil })
	if err != nil || replayed != 1 {
		t.Errorf("expected 1 replayed with limit=1, got %d, err %v", replayed, err)
	}

	_, _, err = ReplayWALChunk(chunkDir, 0, 0, func(r *WALRecord) error {
		return fmt.Errorf("callback error")
	})
	if err == nil {
		t.Error("expected error when replay callback fails")
	}
}

func TestExtraCoverage_WALCompactFunctions(t *testing.T) {
	// GetMaxSeqFromLine
	validLine := []byte(`{"seq":99,"op":"create","kind":"item","id":"it-1"}`)
	if seq := GetMaxSeqFromLine(validLine); seq != 99 {
		t.Errorf("expected 99, got %d", seq)
	}
	if seq := GetMaxSeqFromLine([]byte("invalid json")); seq != 0 {
		t.Errorf("expected 0 for invalid json, got %d", seq)
	}

	// ParseWALLine
	recs, err := ParseWALLine(validLine)
	if err != nil || len(recs) != 1 {
		t.Fatalf("ParseWALLine: err=%v len=%d", err, len(recs))
	}
	if recs[0].Seq != 99 {
		t.Errorf("expected seq 99, got %d", recs[0].Seq)
	}
	if _, err := ParseWALLine([]byte("not valid")); err == nil {
		t.Error("expected error for invalid line")
	}

	// Test marshal and parse of different ops and audit_event kind
	for _, op := range []string{"create", "update", "delete", "custom_op"} {
		rec := &WALRecord{
			Op:      op,
			Kind:    "audit_event",
			ID:      "aud-1",
			Seq:     101,
			DataB64: "aWQ6IGF1ZC0xCmV2ZW50X3R5cGU6IHJlYWQ=", // base64 of id: aud-1\nevent_type: read
		}
		data, err := marshalCompactRecord(rec)
		if err != nil {
			t.Fatalf("marshalCompactRecord(%s): %v", op, err)
		}
		parsed, err := ParseWALLine(data)
		if err != nil || len(parsed) != 1 {
			t.Fatalf("ParseWALLine after marshal(%s): err=%v len=%d", op, err, len(parsed))
		}
		if parsed[0].Op != op {
			t.Errorf("expected op %s, got %s", op, parsed[0].Op)
		}
	}
}

func TestExtraCoverage_WaitForWALProcessingEventDriven(t *testing.T) {
	// Empty project root
	if err := WaitForWALProcessingEventDriven("", time.Second); err != nil {
		t.Errorf("expected nil for empty root, got %v", err)
	}

	// Zero or negative timeout
	if err := WaitForWALProcessingEventDriven(t.TempDir(), 0); err != nil {
		t.Errorf("expected nil for zero timeout, got %v", err)
	}
	if err := WaitForWALProcessingEventDriven(t.TempDir(), -1*time.Second); err != nil {
		t.Errorf("expected nil for negative timeout, got %v", err)
	}

	// Missing WAL file
	tmpDir := t.TempDir()
	if err := WaitForWALProcessingEventDriven(tmpDir, 50*time.Millisecond); err != nil {
		t.Errorf("expected nil for missing WAL file, got %v", err)
	}

	// Empty WAL file
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	_ = fileutil.MkdirAll(walDir, paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(walDir, objectWALFileName), []byte(""), paths.FilePerm644)
	if err := WaitForWALProcessingEventDriven(tmpDir, 50*time.Millisecond); err != nil {
		t.Errorf("expected nil for empty WAL file, got %v", err)
	}

	// Populated WAL where appliedSeq >= maxSeq (fast path)
	rec := &WALRecord{Op: "create", Kind: "doc_entry", ID: "doc-1", Seq: 5}
	walLine, _ := marshalCompactRecord(rec)
	_ = fileutil.WriteFile(filepath.Join(walDir, objectWALFileName), append(walLine, '\n'), paths.FilePerm644)
	_ = WriteAppliedSeq(tmpDir, 5)
	if err := WaitForWALProcessingEventDriven(tmpDir, 100*time.Millisecond); err != nil {
		t.Errorf("expected nil on fast path, got %v", err)
	}

	// Populated WAL where appliedSeq < maxSeq and timeout occurs
	_ = WriteAppliedSeq(tmpDir, 0)
	err := WaitForWALProcessingEventDriven(tmpDir, 10*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error when appliedSeq < maxSeq")
	}
}

func TestExtraCoverage_ReadLastSeqFromTail(t *testing.T) {
	// Empty project root
	seq, err := ReadLastSeqFromTail("")
	if err == nil || seq != 0 {
		t.Errorf("expected error and seq 0 on empty projectRoot, got seq=%d err=%v", seq, err)
	}

	// Missing file returns error
	tmpDir := t.TempDir()
	seq, err = ReadLastSeqFromTail(tmpDir)
	if err == nil {
		t.Errorf("expected error on missing file, got seq=%d", seq)
	}

	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	_ = fileutil.MkdirAll(walDir, paths.DirPerm755)
	walPath := filepath.Join(walDir, objectWALFileName)

	// Write compact v2 single records and a v2 batch record
	rec1 := &WALRecord{Op: "create", Kind: "doc_entry", ID: "doc-1", Seq: 12}
	line1, err := marshalCompactRecord(rec1)
	if err != nil {
		t.Fatalf("marshalCompactRecord: %v", err)
	}

	batchRecs := []*WALRecord{
		{Op: "update", Kind: "bli", ID: "bli-1", Seq: 15},
		{Op: "update", Kind: "bli", ID: "bli-2", Seq: 25},
	}
	batchBytes, err := marshalCompactBatch(batchRecs)
	if err != nil {
		t.Fatalf("marshalCompactBatch: %v", err)
	}

	var content []byte
	content = append(content, line1...)
	content = append(content, '\n', '\n') // includes empty line to verify trimming
	content = append(content, batchBytes...)
	content = append(content, '\n')

	if err := fileutil.WriteFile(walPath, content, paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	seq, err = ReadLastSeqFromTail(tmpDir)
	if err != nil {
		t.Fatalf("ReadLastSeqFromTail failed: %v", err)
	}
	if seq != 25 {
		t.Errorf("expected last seq 25, got %d", seq)
	}
}

