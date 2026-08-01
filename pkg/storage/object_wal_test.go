// Unit tests for object WAL (write-ahead log). Validate append, sync, checkpoint, and replay.
// Write-behind is disabled in most tests via NewFileObjectStorageForTest; these tests exercise
// the WAL in isolation to prevent regression.

package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestObjectWAL_AppendAndSync(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	defer wal.Close()

	rec := &WALRecord{Op: "create", Kind: "backlog_item", ID: "bli-001", DataB64: "e2lkOiBibGktMDAxfQ=="}
	if err := wal.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if rec.Seq == 0 {
		t.Error("expected Seq to be set by Append")
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	data, err := os.ReadFile(wal.GetPath())
	if err != nil {
		t.Fatalf("ReadFile WAL: %v", err)
	}
	// WAL uses v2 compact format; parse first line (may contain newline)
	firstLine := strings.TrimSpace(strings.Split(string(data), "\n")[0])
	records, err := parseWALLine([]byte(firstLine))
	if err != nil || len(records) != 1 {
		t.Fatalf("parseWALLine: err=%v records=%d", err, len(records))
	}
	decoded := records[0]
	if decoded.Op != "create" || decoded.Kind != "backlog_item" || decoded.ID != "bli-001" || decoded.Seq != rec.Seq {
		t.Errorf("decoded record: op=%q kind=%q id=%q seq=%d", decoded.Op, decoded.Kind, decoded.ID, decoded.Seq)
	}
}

func TestObjectWAL_AppendBatchAndReplay(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	defer wal.Close()

	recs := []*WALRecord{
		{Op: "create", Kind: "backlog_item", ID: "bli-001"},
		{Op: "create", Kind: "backlog_item", ID: "bli-002"},
		{Op: "update", Kind: "backlog_item", ID: "bli-001", DataB64: "e2lkOiBibGktMDAxfQ=="},
	}
	if err := wal.AppendBatch(recs); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	wal.Close()

	var replayed []string
	err = ReplayWAL(tmpDir, -1, func(r *WALRecord) error {
		replayed = append(replayed, r.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWAL: %v", err)
	}
	if len(replayed) != 3 || replayed[0] != "bli-001" || replayed[1] != "bli-002" || replayed[2] != "bli-001" {
		t.Errorf("ReplayWAL: replayed %v, want [bli-001 bli-002 bli-001]", replayed)
	}
}

func TestObjectWAL_AuditEventCompactRoundTrip(t *testing.T) {
	// audit_event uses positional encoding (values only); verify round-trip.
	tmpDir := t.TempDir()
	yamlPayload := []byte(`id: AUD-001
kind: audit_event
event_type: object_creation
operation: Created backlog item
status: completed
created_at: "2030-02-24T10:00:00Z"
`)
	rec := &WALRecord{
		Op:      "create",
		Kind:    "audit_event",
		ID:      "AUD-001",
		DataB64: base64.StdEncoding.EncodeToString(yamlPayload),
	}
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	if err := wal.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	_ = wal.Sync()
	wal.Close()

	var replayed *WALRecord
	err = ReplayWAL(tmpDir, -1, func(r *WALRecord) error {
		replayed = r
		return nil
	})
	if err != nil || replayed == nil {
		t.Fatalf("ReplayWAL: err=%v replayed=%v", err, replayed)
	}
	decoded, err := replayed.DecodeRecordData()
	if err != nil {
		t.Fatalf("DecodeRecordData: %v", err)
	}
	// Decoded YAML may differ in key order; check key fields.
	if !strings.Contains(string(decoded), "AUD-001") || !strings.Contains(string(decoded), "audit_event") ||
		!strings.Contains(string(decoded), "object_creation") || !strings.Contains(string(decoded), "completed") {
		t.Errorf("round-trip payload missing expected content: %q", decoded)
	}
}

func TestObjectWAL_CheckpointReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	if err := WriteAppliedSeq(tmpDir, 42); err != nil {
		t.Fatalf("WriteAppliedSeq: %v", err)
	}
	seq, err := ReadAppliedSeq(tmpDir)
	if err != nil {
		t.Fatalf("ReadAppliedSeq: %v", err)
	}
	if seq != 42 {
		t.Errorf("ReadAppliedSeq: got %d, want 42", seq)
	}
	otherDir := t.TempDir()
	seq2, err := ReadAppliedSeq(otherDir)
	if err != nil {
		t.Fatalf("ReadAppliedSeq (missing): %v", err)
	}
	if seq2 != 0 {
		t.Errorf("ReadAppliedSeq (missing file): got %d, want 0", seq2)
	}
}

func TestObjectWAL_ReplaySkipsApplied(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	rec1 := &WALRecord{Op: "create", Kind: "backlog_item", ID: "bli-001"}
	rec2 := &WALRecord{Op: "create", Kind: "backlog_item", ID: "bli-002"}
	_ = wal.Append(rec1)
	_ = wal.Append(rec2)
	_ = wal.Sync()
	wal.Close()

	if err := WriteAppliedSeq(tmpDir, rec1.Seq); err != nil {
		t.Fatalf("WriteAppliedSeq: %v", err)
	}
	var replayed []string
	err = ReplayWAL(tmpDir, rec1.Seq, func(rec *WALRecord) error {
		replayed = append(replayed, rec.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWAL: %v", err)
	}
	if len(replayed) != 1 || replayed[0] != "bli-002" {
		t.Errorf("ReplayWAL: replayed %v, want [bli-002]", replayed)
	}
}

func TestReplayWALChunk_LimitsReplay(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	for i := 1; i <= 5; i++ {
		_ = wal.Append(&WALRecord{Op: "create", Kind: "backlog_item", ID: fmt.Sprintf("bli-%03d", i)})
	}
	_ = wal.Sync()
	wal.Close()

	var replayed []string
	replayed1, lastSeq1, err := ReplayWALChunk(tmpDir, 0, 2, func(rec *WALRecord) error {
		replayed = append(replayed, rec.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWALChunk first: %v", err)
	}
	if replayed1 != 2 || len(replayed) != 2 {
		t.Errorf("first chunk: replayed=%d len(replayed)=%d, want 2", replayed1, len(replayed))
	}

	replayed2, _, err := ReplayWALChunk(tmpDir, lastSeq1, 2, func(rec *WALRecord) error {
		replayed = append(replayed, rec.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWALChunk second: %v", err)
	}
	if replayed2 != 2 || len(replayed) != 4 {
		t.Errorf("second chunk: replayed=%d len(replayed)=%d, want 2 and 4", replayed2, len(replayed))
	}

	replayed3, _, err := ReplayWALChunk(tmpDir, lastSeq1+2, 2, func(rec *WALRecord) error {
		replayed = append(replayed, rec.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWALChunk third: %v", err)
	}
	if replayed3 != 1 || len(replayed) != 5 {
		t.Errorf("third chunk: replayed=%d len(replayed)=%d, want 1 and 5", replayed3, len(replayed))
	}
}

func TestObjectWAL_NewObjectWAL_EmptyProjectRoot(t *testing.T) {
	_, err := NewObjectWAL("")
	if err == nil {
		t.Error("NewObjectWAL with empty project root should error")
	}
}

func TestWALRecord_DecodeRecordData(t *testing.T) {
	rec := &WALRecord{DataB64: "e2lkOiB0ZXN0fQ=="}
	data, err := rec.DecodeRecordData()
	if err != nil {
		t.Fatalf("DecodeRecordData: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty decoded data")
	}
	rec2 := &WALRecord{}
	data2, err := rec2.DecodeRecordData()
	if err != nil {
		t.Fatalf("DecodeRecordData empty: %v", err)
	}
	if data2 != nil {
		t.Errorf("DecodeRecordData empty: got %q", data2)
	}
}

func TestObjectWAL_DirCreated(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	defer wal.Close()
	walDir := filepath.Dir(wal.GetPath())
	if fi, err := os.Stat(walDir); err != nil || !fi.IsDir() {
		t.Errorf("WAL dir %q missing or not dir: %v", walDir, err)
	}
}

// TestObjectWAL_ReplayLargeLine verifies that a WAL line larger than bufio.Scanner's default
// 64KB limit can be replayed (avoids "token too long" error when records have large data_base64).
func TestObjectWAL_ReplayLargeLine(t *testing.T) {
	tmpDir := t.TempDir()
	// Build a record whose JSON line exceeds 64KB (default bufio.MaxScanTokenSize)
	largePayload := strings.Repeat("x", 70*1024) // 70KB
	rec := &WALRecord{
		Op:      "create",
		Kind:    "backlog_item",
		ID:      "bli-large",
		DataB64: base64.StdEncoding.EncodeToString([]byte(largePayload)),
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(line) < 64*1024 {
		t.Skip("JSON line is under 64KB on this platform; increase payload to exercise large-line path")
	}
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	if err := wal.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	_ = wal.Sync()
	wal.Close()

	var replayed []string
	err = ReplayWAL(tmpDir, -1, func(r *WALRecord) error {
		replayed = append(replayed, r.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWAL (large line): %v", err)
	}
	if len(replayed) != 1 || replayed[0] != "bli-large" {
		t.Errorf("ReplayWAL: replayed %v, want [bli-large]", replayed)
	}
}

// TestObjectWAL_RejectLongID verifies that Append and AppendBatch reject records with object ID longer than validation.MaxObjectIDLength.
func TestObjectWAL_RejectLongID(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	defer wal.Close()

	longID := strings.Repeat("x", validation.MaxObjectIDLength+1)
	rec := &WALRecord{Op: "create", Kind: "backlog_item", ID: longID}
	if err := wal.Append(rec); err == nil {
		t.Fatal("Append with too-long ID should fail")
	}

	recs := []*WALRecord{
		{Op: "create", Kind: "backlog_item", ID: "bli-001"},
		{Op: "create", Kind: "backlog_item", ID: longID},
	}
	if err := wal.AppendBatch(recs); err == nil {
		t.Fatal("AppendBatch with too-long ID should fail")
	}
}

// TestObjectWAL_RejectOversizedLine verifies that Append rejects a record whose marshalled line would exceed maxWALLineSize.
func TestObjectWAL_RejectOversizedLine(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	defer wal.Close()

	// Payload large enough that base64-encoded it pushes the compact line over 2 MiB.
	largePayload := strings.Repeat("y", (2*1024*1024)-1024) // just under 2 MiB raw; with compact framing and base64 we exceed 2 MiB line
	rec := &WALRecord{
		Op:      "create",
		Kind:    "backlog_item",
		ID:      "bli-huge",
		DataB64: base64.StdEncoding.EncodeToString([]byte(largePayload)),
	}
	if err := wal.Append(rec); err == nil {
		t.Fatal("Append with payload that would exceed maxWALLineSize should fail")
	}
}

// TestObjectWAL_MigrateEmptyCanonicalWithLegacyData verifies that an empty object.wal created before
// legacy object_wal could be renamed does not block migration (recycle/touch footgun).
func TestObjectWAL_MigrateEmptyCanonicalWithLegacyData(t *testing.T) {
	tmpDir := t.TempDir()
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	if err := os.MkdirAll(walDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacyPath := filepath.Join(walDir, legacyObjectWALFileName)
	payload := []byte("legacy-wal-line\n")
	if err := os.WriteFile(legacyPath, payload, paths.FilePerm644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	canonicalPath := filepath.Join(walDir, objectWALFileName)
	if err := os.WriteFile(canonicalPath, nil, paths.FilePerm600); err != nil {
		t.Fatalf("write empty canonical: %v", err)
	}
	ckPath := canonicalPath + objectWALCheckpointExt
	if err := os.WriteFile(ckPath, []byte(`{"applied_seq":1}`), paths.FilePerm600); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}

	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	defer wal.Close()

	if _, err := os.Stat(legacyPath); err == nil {
		t.Fatal("legacy object_wal should have been renamed away")
	}
	data, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("canonical content = %q; want %q", data, payload)
	}
}
