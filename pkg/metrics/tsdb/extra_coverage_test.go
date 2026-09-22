package tsdb

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTSDB_EdgeCases(t *testing.T) {
	// Empty root error
	if _, err := OpenFileTSDB(""); err == nil {
		t.Fatal("expected error for empty root")
	}

	root := filepath.Join(t.TempDir(), "tsdb_extra")
	db, err := OpenFileTSDB(root)
	if err != nil {
		t.Fatalf("OpenFileTSDB failed: %v", err)
	}

	ctx := context.Background()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Write with canceled context
	if err := db.Write(canceledCtx, "cpu", DataPoint{Value: 1.0}); err == nil {
		t.Fatal("expected error on canceled context Write")
	}

	// Write with empty metric name
	if err := db.Write(ctx, "", DataPoint{Value: 1.0}); err == nil {
		t.Fatal("expected error on empty metric name Write")
	}

	// Write with zero timestamp (should default to time.Now().UTC())
	if err := db.Write(ctx, "memory", DataPoint{Value: 50.0}); err != nil {
		t.Fatalf("Write with zero timestamp failed: %v", err)
	}

	// Read with canceled context
	if _, err := db.Read(canceledCtx, "memory", time.Now().Add(-time.Hour), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected error on canceled context Read")
	}

	// Read with empty metric name
	if _, err := db.Read(ctx, "", time.Now().Add(-time.Hour), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected error on empty metric name Read")
	}

	// Read non-existent metric
	if _, err := db.Read(ctx, "non_existent", time.Now().Add(-time.Hour), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected error for non-existent metric")
	}

	// Read existing metric
	pts, err := db.Read(ctx, "memory", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(pts) != 1 {
		t.Fatalf("expected 1 point, got %d", len(pts))
	}

	// Close
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Calling Close again should be a no-op / error-free
	if err := db.Close(); err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}
}

func TestMemoryTSDB_ReadEdgeCases(t *testing.T) {
	db := NewMemoryTSDB()

	ctx := context.Background()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Read with canceled context
	if _, err := db.Read(canceledCtx, "cpu", time.Now().Add(-time.Hour), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected error on canceled context Read")
	}

	// Read non-existent metric
	if _, err := db.Read(ctx, "non_existent", time.Now().Add(-time.Hour), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected error on non-existent metric Read")
	}

	// Write and Read existing
	now := time.Now().UTC()
	if err := db.Write(ctx, "disk", DataPoint{Timestamp: now, Value: 100.0}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	res, err := db.Read(ctx, "disk", now.Add(-time.Second), now.Add(time.Second))
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 point, got %d", len(res))
	}

	// Close memory TSDB
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestOpen_EmptyString(t *testing.T) {
	db, err := Open("   ")
	if err != nil {
		t.Fatalf("Open with whitespace failed: %v", err)
	}
	if _, ok := db.(*MemoryTSDB); !ok {
		t.Fatalf("expected MemoryTSDB, got %T", db)
	}
}
