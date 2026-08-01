package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestEmbeddedTSDBProvider(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zqk-tsdb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	provider := NewEmbeddedTSDBProvider(tempDir)
	ctx := context.Background()

	if err := provider.Initialize(ctx); err != nil {
		t.Fatalf("failed to initialize provider: %v", err)
	}

	// 1. Write Data Points
	pts := []TSDBPoint{
		{
			Measurement: "cpu_usage",
			Tags:        map[string]string{"host": "server-1", "region": "us-east"},
			Fields:      map[string]any{"value": 0.85, "cores": 8},
			Timestamp:   time.Now().Add(-2 * time.Hour),
		},
		{
			Measurement: "cpu_usage",
			Tags:        map[string]string{"host": "server-2", "region": "us-east"},
			Fields:      map[string]any{"value": 0.45, "cores": 4},
			Timestamp:   time.Now().Add(-1 * time.Hour),
		},
		{
			Measurement: "memory_usage",
			Tags:        map[string]string{"host": "server-1"},
			Fields:      map[string]any{"value": 16.0},
			Timestamp:   time.Now(),
		},
	}

	if err := provider.WriteBatch(ctx, pts); err != nil {
		t.Fatalf("failed to write batch: %v", err)
	}

	// Make sure the provider flushes/closes so the query can read properly (though it reads what's on disk anyway)
	if err := provider.logFile.Sync(); err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// 2. Test Basic Query
	t.Run("BasicQuery", func(t *testing.T) {
		res, err := provider.Query(ctx, TSDBQuery{
			Measurement: "cpu_usage",
		})
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(res.Points) != 2 {
			t.Errorf("expected 2 cpu_usage points, got %d", len(res.Points))
		}
	})

	// 3. Test Tag Filter Query
	t.Run("TagFilterQuery", func(t *testing.T) {
		res, err := provider.Query(ctx, TSDBQuery{
			Measurement: "cpu_usage",
			Tags:        map[string]string{"host": "server-1"},
		})
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(res.Points) != 1 {
			t.Errorf("expected 1 point, got %d", len(res.Points))
		}
		if res.Points[0].Fields["value"].(float64) != 0.85 {
			t.Errorf("expected value 0.85, got %v", res.Points[0].Fields["value"])
		}
	})

	// 4. Test RPN Filtering
	t.Run("RPNFilterQuery", func(t *testing.T) {
		res, err := provider.Query(ctx, TSDBQuery{
			Measurement:   "cpu_usage",
			RPNExpression: "value 0.5 >",
		})
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(res.Points) != 1 {
			t.Errorf("expected 1 point, got %d", len(res.Points))
		}
		if res.Points[0].Tags["host"] != "server-1" {
			t.Errorf("expected server-1, got %v", res.Points[0].Tags["host"])
		}
	})

	// 5. Test Aggregation (Mean)
	t.Run("AggregationMeanQuery", func(t *testing.T) {
		res, err := provider.Query(ctx, TSDBQuery{
			Measurement: "cpu_usage",
			GroupBy:     []string{"region"},
			Aggregation: "mean",
			Field:       "value",
		})
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(res.Points) != 1 {
			t.Errorf("expected 1 aggregated point, got %d", len(res.Points))
		}
		// (0.85 + 0.45) / 2 = 0.65
		if res.Points[0].Fields["value"].(float64) != 0.65 {
			t.Errorf("expected mean 0.65, got %v", res.Points[0].Fields["value"])
		}
		if res.Points[0].Tags[objects.FieldKeyGroup] != "us-east|" {
			t.Errorf("expected group 'us-east|', got '%v'", res.Points[0].Tags[objects.FieldKeyGroup])
		}
	})

	if err := provider.Close(ctx); err != nil {
		t.Fatalf("failed to close provider: %v", err)
	}

	if !provider.IsClosed() {
		t.Errorf("expected IsClosed to be true after Close")
	}

	// Verify file was written
	files, err := filepath.Glob(filepath.Join(tempDir, "metrics-*.tsdb"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no tsdb files found in temp dir")
	}
}

func TestEmbeddedTSDBProvider_IsClosed(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zqk-tsdb-close-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	provider := NewEmbeddedTSDBProvider(tempDir)
	if provider.IsClosed() {
		t.Errorf("expected new provider IsClosed to be false")
	}

	ctx := context.Background()
	_ = provider.Initialize(ctx)
	_ = provider.Close(ctx)

	if !provider.IsClosed() {
		t.Errorf("expected provider IsClosed to be true after Close")
	}
}
