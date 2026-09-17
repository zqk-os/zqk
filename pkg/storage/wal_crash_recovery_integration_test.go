package storage_test

import (
	"encoding/base64"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"
)

func TestWALCrashRecoveryProof(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize WAL and write synchronously with AppendWithSync
	wal, err := storage.NewObjectWAL(tempDir)
	if err != nil {
		t.Fatalf("NewObjectWAL failed: %v", err)
	}

	rec1 := &storage.WALRecord{
		Kind:    "backlog_item",
		ID:      "BLI-TEST-CRASH-001",
		Op:      "create",
		DataB64: base64.StdEncoding.EncodeToString([]byte("title: Crash Recovery Item 1\n")),
	}

	if err := wal.AppendWithSync(rec1); err != nil {
		t.Fatalf("AppendWithSync rec1 failed: %v", err)
	}

	batch := []*storage.WALRecord{
		{
			Kind:    "criteria",
			ID:      "CRIT-TEST-CRASH-001",
			Op:      "create",
			DataB64: base64.StdEncoding.EncodeToString([]byte("title: Crash Recovery Criteria 1\n")),
		},
		{
			Kind:    "criteria",
			ID:      "CRIT-TEST-CRASH-002",
			Op:      "create",
			DataB64: base64.StdEncoding.EncodeToString([]byte("title: Crash Recovery Criteria 2\n")),
		},
	}

	if err := wal.AppendBatchWithSync(batch); err != nil {
		t.Fatalf("AppendBatchWithSync failed: %v", err)
	}

	// Simulate immediate crash / restart without graceful Close
	replayedCount := 0
	_, lastSeq, err := storage.ReplayWALChunk(tempDir, 0, 0, func(rec *storage.WALRecord) error {
		replayedCount++
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWALChunk after simulated crash failed: %v", err)
	}

	if replayedCount != 3 {
		t.Errorf("expected 3 replayed records, got %d", replayedCount)
	}
	if lastSeq < 3 {
		t.Errorf("expected lastSeq >= 3, got %d", lastSeq)
	}

	_ = wal.Close()
}
