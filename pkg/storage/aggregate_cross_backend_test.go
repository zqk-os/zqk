package storage_test

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// TestAggregate_CrossBackendConsistency verifies that file and graph backends
// return equivalent aggregation results for the same filter and aggregations.
// File storage uses real data; graph storage uses a mock configured to return
// the same logical result so we can assert result shape and values match.
func TestAggregate_CrossBackendConsistency(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	filter := storage.ListFilter{Kind: "backlog_item"}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationCount, Alias: "count"},
		{Function: storage.AggregationSum, Field: "score", Alias: "sum_score"},
		{Function: storage.AggregationAvg, Field: "score", Alias: "avg_score"},
	}

	// 1) File backend: real data (3 items with score 10, 20, 30)
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	fileStorage, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	for i, score := range []float64{10.0, 20.0, 30.0} {
		item := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("BLI-X%d", i+1),
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         fmt.Sprintf("Cross Item %d", i+1),
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "dev",
			objects.FieldKeyScore:         score,
		}
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, item, "")
	}
	fileResult, err := fileStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("File Aggregate: %v", err)
	}

	// 2) Graph backend: mock returning same logical result (count=3, sum=60, avg=20)
	// Cypher built by graph: MATCH (n:BacklogItem:Entity) RETURN COUNT(n) AS count, SUM(n.score) AS sum_score, AVG(n.score) AS avg_score
	query := "MATCH (n:BacklogItem:Entity) RETURN COUNT(n) AS count, SUM(n.score) AS sum_score, AVG(n.score) AS avg_score"
	mockConn := storage.NewMockGraphConnectionForAggregateWithQueryResult(query, &provider.QueryResult{
		Rows: []map[string]any{
			{"count": int64(3), "sum_score": 60.0, "avg_score": 20.0},
		},
	})
	graphStorage, err := storage.NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("NewGraphObjectStorage: %v", err)
	}
	graphResult, err := graphStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Graph Aggregate: %v", err)
	}

	// 3) Assert equivalent results (same keys, comparable values)
	assertAggregateResultsEquivalent(t, fileResult, graphResult, aggregations)
}

// assertAggregateResultsEquivalent compares two AggregateResults: same aggregation keys
// and values that are numerically equivalent (e.g. 3 and int64(3), 60.0 and 60.0).
func assertAggregateResultsEquivalent(t *testing.T, a, b *storage.AggregateResult, aggregations []storage.Aggregation) {
	t.Helper()
	if a == nil || b == nil {
		t.Fatal("nil result")
	}
	const emptyAlias = ""
	for _, agg := range aggregations {
		alias := agg.Alias
		if alias == emptyAlias {
			if agg.Field != emptyAlias {
				alias = string(agg.Function) + "_" + agg.Field
			} else {
				alias = string(agg.Function)
			}
		}
		va, oka := a.Aggregations[alias]
		vb, okb := b.Aggregations[alias]
		if !oka {
			t.Errorf("result A missing key %q", alias)
			continue
		}
		if !okb {
			t.Errorf("result B missing key %q", alias)
			continue
		}
		if !aggregationValuesEquivalent(va, vb) {
			t.Errorf("aggregation %q: file=%v (%T) graph=%v (%T)", alias, va, va, vb, vb)
		}
	}
}

func aggregationValuesEquivalent(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Normalize to float64 for numeric comparison
	var fa, fb float64
	switch v := a.(type) {
	case int:
		fa = float64(v)
	case int64:
		fa = float64(v)
	case float64:
		fa = v
	case float32:
		fa = float64(v)
	default:
		return reflect.DeepEqual(a, b)
	}
	switch v := b.(type) {
	case int:
		fb = float64(v)
	case int64:
		fb = float64(v)
	case float64:
		fb = v
	case float32:
		fb = float64(v)
	default:
		return false
	}
	return math.Abs(fa-fb) < 1e-9
}
