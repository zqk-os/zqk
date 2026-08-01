package tsdb

import (
	"context"
	"testing"
	"time"
)

func TestMemoryTSDB_WriteAndRead(t *testing.T) {
	db := NewMemoryTSDB()
	ctx := context.Background()
	metric := "cpu_usage"

	now := time.Now()
	p1 := DataPoint{Timestamp: now, Value: 42.0}
	p2 := DataPoint{Timestamp: now.Add(time.Minute), Value: 45.5}
	p3 := DataPoint{Timestamp: now.Add(2 * time.Minute), Value: 50.0}

	// Test Write
	if err := db.Write(ctx, metric, p1); err != nil {
		t.Fatalf("Failed to write p1: %v", err)
	}
	if err := db.Write(ctx, metric, p2); err != nil {
		t.Fatalf("Failed to write p2: %v", err)
	}
	if err := db.Write(ctx, metric, p3); err != nil {
		t.Fatalf("Failed to write p3: %v", err)
	}

	// Test Read
	start := now
	end := now.Add(time.Minute)

	results, err := db.Read(ctx, metric, start, end)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if results[0].Value != 42.0 || results[1].Value != 45.5 {
		t.Errorf("Unexpected values returned")
	}
}

func TestMemoryTSDB_ReadNotFound(t *testing.T) {
	db := NewMemoryTSDB()
	ctx := context.Background()

	_, err := db.Read(ctx, "non_existent", time.Now(), time.Now().Add(time.Minute))
	if err == nil {
		t.Error("Expected error for non-existent metric, got nil")
	}
}
