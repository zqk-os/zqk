package tsdb

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTSDB_WriteAndRead(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tsdb")
	db, err := OpenFileTSDB(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	start := time.Unix(1_700_000_000, 0).UTC()
	points := []DataPoint{
		{Timestamp: start, Value: 42.5},
		{Timestamp: start.Add(10 * time.Minute), Value: 43.25},
		{Timestamp: start.Add(20 * time.Minute), Value: 44.0},
	}
	for _, p := range points {
		if err := db.Write(ctx, "cpu_usage", p); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	got, err := db.Read(ctx, "cpu_usage", start.Add(-time.Minute), start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(points) {
		t.Fatalf("got %d points want %d", len(got), len(points))
	}
	for i := range points {
		if !got[i].Timestamp.Equal(points[i].Timestamp) {
			t.Fatalf("ts %d: got %v want %v", i, got[i].Timestamp, points[i].Timestamp)
		}
		if math.Abs(got[i].Value-points[i].Value) > 0.001 {
			t.Fatalf("val %d: got %v want %v", i, got[i].Value, points[i].Value)
		}
	}
}

func TestOpen_MemoryAndFile(t *testing.T) {
	mem, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mem.(*MemoryTSDB); !ok {
		t.Fatalf("want MemoryTSDB, got %T", mem)
	}
	_ = mem.Close()

	file, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := file.(*FileTSDB); !ok {
		t.Fatalf("want FileTSDB, got %T", file)
	}
	_ = file.Close()
}

func TestMemoryTSDB_ContextCanceled(t *testing.T) {
	db := NewMemoryTSDB()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := db.Write(ctx, "m", DataPoint{Timestamp: time.Now(), Value: 1}); err == nil {
		t.Fatal("want canceled write error")
	}
}
