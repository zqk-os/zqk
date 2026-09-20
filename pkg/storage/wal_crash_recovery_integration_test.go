package storage_test

import (
	"encoding/base64"
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"
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

func TestWALCompactionAndCheckpointCrashRecovery(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Initialize WAL and append 4 records with synchronous durability
	wal, err := storage.NewObjectWAL(tempDir)
	if err != nil {
		t.Fatalf("NewObjectWAL failed: %v", err)
	}

	for i := 1; i <= 4; i++ {
		rec := &storage.WALRecord{
			Kind:    "backlog_item",
			ID:      "BLI-DURABILITY-00" + string(rune('0'+i)),
			Op:      "create",
			DataB64: base64.StdEncoding.EncodeToString([]byte("title: Durability Item\n")),
		}
		if err := wal.AppendWithSync(rec); err != nil {
			t.Fatalf("AppendWithSync rec %d failed: %v", i, err)
		}
	}
	_ = wal.Close()

	// 2. Set checkpoint at seq=2 using WriteAppliedSeq
	if err := storage.WriteAppliedSeq(tempDir, 2); err != nil {
		t.Fatalf("WriteAppliedSeq failed: %v", err)
	}
	applied, err := storage.ReadAppliedSeq(tempDir)
	if err != nil || applied != 2 {
		t.Fatalf("ReadAppliedSeq got %d, err %v, want 2", applied, err)
	}

	// 3. Compact WAL: prunes records <= 2, keeps 3 and 4, syncs dir
	if err := storage.CompactWAL(tempDir); err != nil {
		t.Fatalf("CompactWAL failed: %v", err)
	}

	// 4. Reopen WAL and write records 5 and 6 under active execution
	wal2, err := storage.NewObjectWAL(tempDir)
	if err != nil {
		t.Fatalf("NewObjectWAL after compaction failed: %v", err)
	}
	for i := 5; i <= 6; i++ {
		rec := &storage.WALRecord{
			Kind:    "backlog_item",
			ID:      "BLI-DURABILITY-00" + string(rune('0'+i)),
			Op:      "create",
			DataB64: base64.StdEncoding.EncodeToString([]byte("title: Post-Compaction Item\n")),
		}
		if err := wal2.AppendWithSync(rec); err != nil {
			t.Fatalf("AppendWithSync rec %d failed: %v", i, err)
		}
	}
	_ = wal2.Close() // Simulate sudden shutdown/crash

	// 5. Replay from checkpoint seq=2
	var recoveredSeqs []int64
	replayedCount, lastSeq, err := storage.ReplayWALChunk(tempDir, 2, 0, func(rec *storage.WALRecord) error {
		recoveredSeqs = append(recoveredSeqs, rec.Seq)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWALChunk after crash failed: %v", err)
	}

	if replayedCount != 4 {
		t.Fatalf("expected 4 replayed records (3, 4, 5, 6), got %d (%v)", replayedCount, recoveredSeqs)
	}
	if lastSeq != 6 {
		t.Errorf("expected lastSeq == 6, got %d", lastSeq)
	}

	// Verify sequential continuity
	expectedSeqs := []int64{3, 4, 5, 6}
	for i, want := range expectedSeqs {
		if recoveredSeqs[i] != want {
			t.Errorf("record %d: got seq %d, want %d", i, recoveredSeqs[i], want)
		}
	}

	// Replay from terminal checkpoint seq=6 returns 0 new records
	replayedZero, _, err := storage.ReplayWALChunk(tempDir, 6, 0, func(rec *storage.WALRecord) error {
		t.Errorf("unexpected record replay for terminal checkpoint: %+v", rec)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayWALChunk from terminal checkpoint failed: %v", err)
	}
	if replayedZero != 0 {
		t.Errorf("expected 0 replayed records, got %d", replayedZero)
	}
}
