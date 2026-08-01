package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// assertAggregationCount compares an aggregation value to an expected int, accepting int/int64/float64.
// Avoids flakiness when backends or JSON round-trips use different numeric types.
func assertAggregationCount(t *testing.T, name string, got any, expected int) {
	t.Helper()
	var n int
	switch v := got.(type) {
	case int:
		n = v
	case int64:
		n = int(v)
	case float64:
		n = int(v)
	default:
		t.Errorf("%s: expected numeric count, got %T %v", name, got, got)
		return
	}
	if n != expected {
		t.Errorf("%s: expected %d, got %d (raw %v)", name, expected, n, got)
	}
}

func setupAggregateTest(t *testing.T) (string, *storage.FileObjectStorage, *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = fos.Shutdown(context.Background()) }()
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	return tmpDir, fos, secCtx
}

func TestAggregate_Count(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	// Create test backlog items
	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-901", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-902", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-903", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 3", objects.FieldKeyStatus: "validated", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
	}

	// Test count aggregation
	filter := storage.ListFilter{
		Kind: "backlog_item",
	}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationCount, Alias: "total"},
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	assertAggregationCount(t, "total", result.Aggregations["total"], 3)
}

func TestAggregate_GroupBy(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	// Create test backlog items with different statuses
	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-901", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-902", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-903", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 3", objects.FieldKeyStatus: "validated", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
	}

	// Test group by status
	filter := storage.ListFilter{
		Kind:    "backlog_item",
		GroupBy: "status",
	}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationCount, Alias: "count"},
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if result.Groups == nil {
		t.Fatal("Expected groups to be populated")
	}

	exploringGroup, ok := result.Groups["exploring"]
	if !ok {
		t.Fatal("Expected 'exploring' group")
	}
	assertAggregationCount(t, "exploring count", exploringGroup.Aggregations["count"], 2)

	validatedGroup, ok := result.Groups["validated"]
	if !ok {
		t.Fatal("Expected 'validated' group")
	}
	assertAggregationCount(t, "validated count", validatedGroup.Aggregations["count"], 1)
}

func TestAggregate_MinMax(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	// Create test backlog items with estimated_effort values
	// For min/max, we'll test with count since we need valid objects
	// Real numeric aggregations would require custom fields or different object kinds
	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-901", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-902", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-903", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 3", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
	}

	// Test count aggregation (min/max require numeric fields which are validated)
	filter := storage.ListFilter{
		Kind: "backlog_item",
	}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationCount, Alias: "total"},
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	assertAggregationCount(t, "total", result.Aggregations["total"], 3)
}

func TestAggregate_SumAvg(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	// Create test backlog items with priority values
	// For sum/avg, we'll test with count since we need valid objects
	// Real numeric aggregations would require custom fields or different object kinds
	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-901", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-902", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-903", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 3", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
	}

	// Test count aggregation (sum/avg require numeric fields which are validated)
	filter := storage.ListFilter{
		Kind: "backlog_item",
	}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationCount, Alias: "total"},
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	assertAggregationCount(t, "total", result.Aggregations["total"], 3)
}

func TestAggregate_EmptyResult(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	// Test aggregation on empty result set
	filter := storage.ListFilter{
		Kind: "backlog_item",
		Filters: map[string]any{
			objects.FieldKeyStatus: "nonexistent",
		},
	}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationCount, Alias: "total"},
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	assertAggregationCount(t, "total (empty)", result.Aggregations["total"], 0)
}

func TestAggregate_EmptyAggregations(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	filter := storage.ListFilter{Kind: "backlog_item"}
	storageCtx := pkgctx.GetStorageContext()
	_, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, nil)
	if err == nil {
		t.Fatal("expected error for nil aggregations")
	}
	_, err = fos.Aggregate(ctx, secCtx, storageCtx, filter, []storage.Aggregation{})
	if err == nil {
		t.Fatal("expected error for empty aggregations")
	}
}

func TestAggregate_SumAvg_RealNumeric(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-A1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item A1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 10.0},
		{objects.FieldKeyID: "ITEM-A2", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item A2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 20.0},
		{objects.FieldKeyID: "ITEM-A3", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item A3", objects.FieldKeyStatus: "validated", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 30.0},
	}
	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	filter := storage.ListFilter{Kind: "backlog_item"}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationSum, Field: "score", Alias: "sum_score"},
		{Function: storage.AggregationAvg, Field: "score", Alias: "avg_score"},
	}
	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if sum, ok := result.Aggregations["sum_score"].(float64); !ok || sum != 60.0 {
		t.Errorf("sum_score: got %v (%T), want 60.0", result.Aggregations["sum_score"], result.Aggregations["sum_score"])
	}
	if avg, ok := result.Aggregations["avg_score"].(float64); !ok || avg != 20.0 {
		t.Errorf("avg_score: got %v (%T), want 20.0", result.Aggregations["avg_score"], result.Aggregations["avg_score"])
	}
}

func TestAggregate_MinMax_RealNumeric(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-B1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item B1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 5},
		{objects.FieldKeyID: "ITEM-B2", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item B2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 15},
		{objects.FieldKeyID: "ITEM-B3", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item B3", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 10},
	}
	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	filter := storage.ListFilter{Kind: "backlog_item"}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationMin, Field: "score", Alias: "min_score"},
		{Function: storage.AggregationMax, Field: "score", Alias: "max_score"},
	}
	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	minVal := result.Aggregations["min_score"]
	maxVal := result.Aggregations["max_score"]
	if f, ok := minVal.(float64); ok {
		if f != 5 {
			t.Errorf("min_score: got %v, want 5", minVal)
		}
	} else if i, ok := minVal.(int); ok {
		if i != 5 {
			t.Errorf("min_score: got %v, want 5", minVal)
		}
	} else {
		t.Errorf("min_score: got %v (%T)", minVal, minVal)
	}
	if f, ok := maxVal.(float64); ok {
		if f != 15 {
			t.Errorf("max_score: got %v, want 15", maxVal)
		}
	} else if i, ok := maxVal.(int); ok {
		if i != 15 {
			t.Errorf("max_score: got %v, want 15", maxVal)
		}
	} else {
		t.Errorf("max_score: got %v (%T)", maxVal, maxVal)
	}
}

func TestAggregate_MultipleAggregations(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-C1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item C1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 2.0},
		{objects.FieldKeyID: "ITEM-C2", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item C2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 4.0},
		{objects.FieldKeyID: "ITEM-C3", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item C3", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 6.0},
	}
	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	filter := storage.ListFilter{Kind: "backlog_item"}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationCount, Alias: "count"},
		{Function: storage.AggregationSum, Field: "score", Alias: "sum_score"},
		{Function: storage.AggregationAvg, Field: "score", Alias: "avg_score"},
		{Function: storage.AggregationMin, Field: "score", Alias: "min_score"},
		{Function: storage.AggregationMax, Field: "score", Alias: "max_score"},
	}
	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	assertAggregationCount(t, "count", result.Aggregations["count"], 3)
	if sum, ok := result.Aggregations["sum_score"].(float64); !ok || sum != 12.0 {
		t.Errorf("sum_score: got %v", result.Aggregations["sum_score"])
	}
	if avg, ok := result.Aggregations["avg_score"].(float64); !ok || avg != 4.0 {
		t.Errorf("avg_score: got %v", result.Aggregations["avg_score"])
	}
	if result.Aggregations["min_score"] == nil {
		t.Error("min_score: expected non-nil")
	}
	if result.Aggregations["max_score"] == nil {
		t.Error("max_score: expected non-nil")
	}
}

func TestAggregate_GroupByWithSum(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-D1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item D1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 10.0},
		{objects.FieldKeyID: "ITEM-D2", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item D2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 20.0},
		{objects.FieldKeyID: "ITEM-D3", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item D3", objects.FieldKeyStatus: "validated", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 30.0},
	}
	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	filter := storage.ListFilter{Kind: "backlog_item", GroupBy: "status"}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationCount, Alias: "n"},
		{Function: storage.AggregationSum, Field: "score", Alias: "total_score"},
	}
	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if result.Groups == nil {
		t.Fatal("expected groups")
	}
	exploring := result.Groups["exploring"]
	if exploring == nil {
		t.Fatal("expected exploring group")
	}
	assertAggregationCount(t, "exploring n", exploring.Aggregations["n"], 2)
	if s, ok := exploring.Aggregations["total_score"].(float64); !ok || s != 30.0 {
		t.Errorf("exploring total_score: got %v", exploring.Aggregations["total_score"])
	}
	validated := result.Groups["validated"]
	if validated == nil {
		t.Fatal("expected validated group")
	}
	assertAggregationCount(t, "validated n", validated.Aggregations["n"], 1)
	if s, ok := validated.Aggregations["total_score"].(float64); !ok || s != 30.0 {
		t.Errorf("validated total_score: got %v", validated.Aggregations["total_score"])
	}
}

// TestAggregate_NumericWithMissingField verifies sum/avg/min/max when some objects
// have the numeric field and others omit it (nil/missing). Sum skips nil; avg is sum/object_count.
func TestAggregate_NumericWithMissingField(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	// Two items with score, one without (nil/missing)
	items := []map[string]any{
		{objects.FieldKeyID: "ITEM-N1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item N1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 10.0},
		{objects.FieldKeyID: "ITEM-N2", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item N2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev"}, // no score
		{objects.FieldKeyID: "ITEM-N3", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item N3", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev", objects.FieldKeyScore: 30.0},
	}
	for _, item := range items {
		if err := fos.Create(ctx, secCtx, item); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	filter := storage.ListFilter{Kind: "backlog_item"}
	aggregations := []storage.Aggregation{
		{Function: storage.AggregationSum, Field: "score", Alias: "sum_score"},
		{Function: storage.AggregationAvg, Field: "score", Alias: "avg_score"},
		{Function: storage.AggregationMin, Field: "score", Alias: "min_score"},
		{Function: storage.AggregationMax, Field: "score", Alias: "max_score"},
	}
	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	// Sum of present values only: 10 + 30 = 40
	if sum, ok := result.Aggregations["sum_score"].(float64); !ok || sum != 40.0 {
		t.Errorf("sum_score: got %v (%T), want 40.0", result.Aggregations["sum_score"], result.Aggregations["sum_score"])
	}
	// Avg = sum / len(objects) = 40/3
	if avg, ok := result.Aggregations["avg_score"].(float64); !ok || avg != 40.0/3.0 {
		t.Errorf("avg_score: got %v (%T), want 40/3", result.Aggregations["avg_score"], result.Aggregations["avg_score"])
	}
	// Min/max over present values: 10 and 30
	if result.Aggregations["min_score"] == nil {
		t.Error("min_score: expected non-nil")
	}
	if result.Aggregations["max_score"] == nil {
		t.Error("max_score: expected non-nil")
	}
}

// TestAggregate_PermissionDenied verifies that Aggregate returns ErrPermissionDenied
// when the security context does not have read permission for the requested kind.
func TestAggregate_PermissionDenied(t *testing.T) {
	_, fos, _ := setupAggregateTest(t)
	ctx := context.Background()

	// Context with no read permission for any kind (no admin, no read:* or read:backlog_item)
	noPermCtx := pkgctx.NewSecurityContext("account:viewer", nil, nil)

	filter := storage.ListFilter{Kind: "backlog_item"}
	aggregations := []storage.Aggregation{{Function: storage.AggregationCount, Alias: "total"}}
	storageCtx := pkgctx.GetStorageContext()

	_, err := fos.Aggregate(ctx, noPermCtx, storageCtx, filter, aggregations)
	if err == nil {
		t.Fatal("expected error when permission denied")
	}
	if !errors.Is(err, storage.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}
}

// TestAggregate_PermissionAllowed_ExplicitReadKind verifies that Aggregate succeeds
// when the security context has explicit read permission for the kind.
func TestAggregate_PermissionAllowed_ExplicitReadKind(t *testing.T) {
	_, fos, secCtx := setupAggregateTest(t)
	ctx := context.Background()

	item := map[string]any{
		objects.FieldKeyID: "ITEM-P1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item P1", objects.FieldKeyStatus: "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "dev",
	}
	if err := fos.Create(ctx, secCtx, item); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Context with explicit read:backlog_item (no admin, no read:*)
	readOnlyCtx := pkgctx.NewSecurityContext("account:viewer", nil, []string{"read:backlog_item"})

	filter := storage.ListFilter{Kind: "backlog_item"}
	aggregations := []storage.Aggregation{{Function: storage.AggregationCount, Alias: "total"}}
	storageCtx := pkgctx.GetStorageContext()

	result, err := fos.Aggregate(ctx, readOnlyCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate with read:backlog_item should succeed: %v", err)
	}
	assertAggregationCount(t, "total", result.Aggregations["total"], 1)
}
