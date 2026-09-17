package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestDSIAStorageProvider_InPlaceMutation(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := fileutil.MkdirTemp("", "dsia_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// Initial object
	obj := map[string]any{
		objects.FieldKeyKind: "test",
		objects.FieldKeyID:   "obj-123",
		objects.FieldKeyName: "Initial Name",
	}

	// 1. Create the object
	if err := provider.Create(ctx, nil, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Read and verify
	readObj, err := provider.Read(ctx, nil, "obj-123")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if readObj[objects.FieldKeyName] != "Initial Name" {
		t.Errorf("Expected name 'Initial Name', got '%v'", readObj[objects.FieldKeyName])
	}
	if readObj["sha256_checksum"] == nil || readObj["sha256_checksum"] == "" {
		t.Errorf("Expected checksum to be embedded, got empty")
	}
	initialChecksum := readObj["sha256_checksum"]

	// 2. Update the object
	updates := map[string]any{
		objects.FieldKeyKind: "test",
		objects.FieldKeyID:   "obj-123",
		objects.FieldKeyName: "Updated Name",
	}
	if err := provider.Update(ctx, nil, "obj-123", updates); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Read and verify update
	readUpdatedObj, err := provider.Read(ctx, nil, "obj-123")
	if err != nil {
		t.Fatalf("Read updated failed: %v", err)
	}
	if readUpdatedObj[objects.FieldKeyName] != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%v'", readUpdatedObj[objects.FieldKeyName])
	}
	if readUpdatedObj["sha256_checksum"] == initialChecksum {
		t.Errorf("Expected checksum to change after update")
	}

	// 3. Verify no orphaned files (only 1 file should exist in tests directory)
	testsDir := filepath.Join(tempDir, "tests")
	entries, err := fileutil.ReadDir(testsDir)
	if err != nil {
		t.Fatalf("Failed to read tests dir: %v", err)
	}

	fileCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			fileCount++
		}
	}

	if fileCount != 1 {
		t.Errorf("Expected exactly 1 file (no orphans), found %d", fileCount)
	}
	if entries[0].Name() != "obj-123.yaml" {
		t.Errorf("Expected file name 'obj-123.yaml', got '%s'", entries[0].Name())
	}
}

func TestDSIAStorageProvider_Exists(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_exists_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// Initially, it should not exist
	exists, err := provider.Exists(ctx, nil, "exists-123")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}
	if exists {
		t.Errorf("Expected object to not exist yet")
	}

	// Create object
	obj := map[string]any{
		objects.FieldKeyKind: "test",
		objects.FieldKeyID:   "exists-123",
	}
	if err := provider.Create(ctx, nil, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Now it should exist
	exists, err = provider.Exists(ctx, nil, "exists-123")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}
	if !exists {
		t.Errorf("Expected object to exist")
	}
}

func TestDSIAStorageProvider_BulkGet(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_bulk_get_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// Create 2 objects
	obj1 := map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "bulk-1", objects.FieldKeyTitle: "First"}
	obj2 := map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "bulk-2", objects.FieldKeyTitle: "Second"}
	if err := provider.Create(ctx, nil, obj1); err != nil {
		t.Fatalf("Create obj1 failed: %v", err)
	}
	if err := provider.Create(ctx, nil, obj2); err != nil {
		t.Fatalf("Create obj2 failed: %v", err)
	}

	// BulkGet with 2 valid IDs and 1 missing ID
	ids := []string{"bulk-1", "nonexistent-id", "bulk-2"}
	result, err := provider.BulkGet(ctx, nil, ids)
	if err != nil {
		t.Fatalf("BulkGet failed unexpectedly: %v", err)
	}

	if result.TotalCount != 3 {
		t.Errorf("Expected TotalCount 3, got %d", result.TotalCount)
	}
	if result.SuccessCount != 2 {
		t.Errorf("Expected SuccessCount 2, got %d", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Errorf("Expected FailureCount 1, got %d", result.FailureCount)
	}
	if len(result.Results) != 2 {
		t.Errorf("Expected 2 successful results, got %d", len(result.Results))
	}
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 bulk error, got %d", len(result.Errors))
	}
}

func TestDSIAStorageProvider_BulkDelete(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_bulk_delete_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// Create 2 objects
	obj1 := map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "del-1", objects.FieldKeyTitle: "First"}
	obj2 := map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "del-2", objects.FieldKeyTitle: "Second"}
	if err := provider.Create(ctx, nil, obj1); err != nil {
		t.Fatalf("Create obj1 failed: %v", err)
	}
	if err := provider.Create(ctx, nil, obj2); err != nil {
		t.Fatalf("Create obj2 failed: %v", err)
	}

	// BulkDelete with 2 valid IDs and 1 missing ID
	ids := []string{"del-1", "nonexistent-id", "del-2"}
	result, err := provider.BulkDelete(ctx, nil, ids, false)
	if err != nil {
		t.Fatalf("BulkDelete failed unexpectedly: %v", err)
	}

	if result.TotalCount != 3 {
		t.Errorf("Expected TotalCount 3, got %d", result.TotalCount)
	}
	if result.SuccessCount != 2 {
		t.Errorf("Expected SuccessCount 2, got %d", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Errorf("Expected FailureCount 1, got %d", result.FailureCount)
	}
	if len(result.Results) != 2 {
		t.Errorf("Expected 2 successful delete results, got %d", len(result.Results))
	}
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 bulk error, got %d", len(result.Errors))
	}

	// Verify del-1 and del-2 no longer exist
	exists1, _ := provider.Exists(ctx, nil, "del-1")
	if exists1 {
		t.Errorf("Expected del-1 to be deleted")
	}
	exists2, _ := provider.Exists(ctx, nil, "del-2")
	if exists2 {
		t.Errorf("Expected del-2 to be deleted")
	}
}

func TestDSIAStorageProvider_BulkCreate(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_bulk_create_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// 2 valid objects and 1 invalid object (missing kind)
	objs := []map[string]any{
		{objects.FieldKeyKind: "test", objects.FieldKeyID: "bc-1", objects.FieldKeyTitle: "First"},
		{objects.FieldKeyID: "bc-2-no-kind", objects.FieldKeyTitle: "Invalid"},
		{objects.FieldKeyKind: "test", objects.FieldKeyID: "bc-3", objects.FieldKeyTitle: "Third"},
	}

	result, err := provider.BulkCreate(ctx, nil, objs)
	if err != nil {
		t.Fatalf("BulkCreate failed unexpectedly: %v", err)
	}

	if result.TotalCount != 3 {
		t.Errorf("Expected TotalCount 3, got %d", result.TotalCount)
	}
	if result.SuccessCount != 2 {
		t.Errorf("Expected SuccessCount 2, got %d", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Errorf("Expected FailureCount 1, got %d", result.FailureCount)
	}

	// Verify bc-1 and bc-3 exist
	exists1, _ := provider.Exists(ctx, nil, "bc-1")
	if !exists1 {
		t.Errorf("Expected bc-1 to exist")
	}
	exists3, _ := provider.Exists(ctx, nil, "bc-3")
	if !exists3 {
		t.Errorf("Expected bc-3 to exist")
	}
}

func TestDSIAStorageProvider_BulkUpdate(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_bulk_update_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// Create 2 objects
	obj1 := map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "bu-1", objects.FieldKeyName: "Old 1"}
	obj2 := map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "bu-2", objects.FieldKeyName: "Old 2"}
	_ = provider.Create(ctx, nil, obj1)
	_ = provider.Create(ctx, nil, obj2)

	// BulkUpdate with 2 valid updates and 1 update for missing object
	updates := []BulkUpdateItem{
		{ID: "bu-1", Updates: map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "bu-1", objects.FieldKeyName: "New 1"}},
		{ID: "bu-missing", Updates: map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "bu-missing", objects.FieldKeyName: "Missing"}},
		{ID: "bu-2", Updates: map[string]any{objects.FieldKeyKind: "test", objects.FieldKeyID: "bu-2", objects.FieldKeyName: "New 2"}},
	}

	result, err := provider.BulkUpdate(ctx, nil, updates)
	if err != nil {
		t.Fatalf("BulkUpdate failed unexpectedly: %v", err)
	}

	if result.TotalCount != 3 {
		t.Errorf("Expected TotalCount 3, got %d", result.TotalCount)
	}
	if result.SuccessCount != 2 {
		t.Errorf("Expected SuccessCount 2, got %d", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Errorf("Expected FailureCount 1, got %d", result.FailureCount)
	}

	// Verify bu-1 updated name
	read1, _ := provider.Read(ctx, nil, "bu-1")
	if read1[objects.FieldKeyName] != "New 1" {
		t.Errorf("Expected name 'New 1', got '%v'", read1[objects.FieldKeyName])
	}
}

func TestDSIAStorageProvider_ListAndCount(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_list_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// Create objects of two kinds
	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "alpha", objects.FieldKeyID: "a-1", objects.FieldKeyName: "A1"})
	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "alpha", objects.FieldKeyID: "a-2", objects.FieldKeyName: "A2"})
	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "beta", objects.FieldKeyID: "b-1", objects.FieldKeyName: "B1"})

	// List alpha kind
	resAlpha, err := provider.List(ctx, nil, nil, ListFilter{Kind: "alpha"})
	if err != nil {
		t.Fatalf("List alpha failed: %v", err)
	}
	if len(resAlpha.Objects) != 2 {
		t.Errorf("Expected 2 alpha objects, got %d", len(resAlpha.Objects))
	}

	// Count alpha kind
	countAlpha, err := provider.Count(ctx, nil, ListFilter{Kind: "alpha"})
	if err != nil {
		t.Fatalf("Count alpha failed: %v", err)
	}
	if countAlpha != 2 {
		t.Errorf("Expected count 2 for alpha, got %d", countAlpha)
	}

	// List all kinds (empty Kind filter)
	resAll, err := provider.List(ctx, nil, nil, ListFilter{})
	if err != nil {
		t.Fatalf("List all failed: %v", err)
	}
	if len(resAll.Objects) != 3 {
		t.Errorf("Expected 3 total objects across kinds, got %d", len(resAll.Objects))
	}
}

func TestDSIAStorageProvider_Aggregate(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_agg_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "item", objects.FieldKeyID: "i-1", objects.FieldKeyScore: 10})
	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "item", objects.FieldKeyID: "i-2", objects.FieldKeyScore: 20})

	aggs := []Aggregation{
		{Function: AggregationCount, Alias: "item_count"},
	}

	res, err := provider.Aggregate(ctx, nil, nil, ListFilter{Kind: "item"}, aggs)
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}

	if res.Aggregations["item_count"] != 2 {
		t.Errorf("Expected item_count 2, got %v", res.Aggregations["item_count"])
	}
}

func TestDSIAStorageProvider_Query(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_query_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "query_kind", objects.FieldKeyID: "q-1", objects.FieldKeyTitle: "Q1"})
	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "query_kind", objects.FieldKeyID: "q-2", objects.FieldKeyTitle: "Q2"})

	q := Query{
		Kind: "query_kind",
		Type: QueryTypeFilter,
	}

	res, err := provider.Query(ctx, nil, nil, q)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(res.Objects) != 2 {
		t.Errorf("Expected 2 objects from query, got %d", len(res.Objects))
	}
}

func TestDSIAStorageProvider_Search(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_search_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "doc", objects.FieldKeyID: "doc-1", objects.FieldKeyTitle: "Quantum Operating System", objects.FieldKeyDescription: "Kernel architecture"})
	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "doc", objects.FieldKeyID: "doc-2", objects.FieldKeyTitle: "Database Storage", objects.FieldKeyDescription: "Relational engine"})

	// Search for "Quantum"
	sq := SearchQuery{
		Query: "Quantum",
	}

	res, err := provider.Search(ctx, nil, nil, sq)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if res.TotalCount != 1 {
		t.Errorf("Expected TotalCount 1, got %d", res.TotalCount)
	}
	if len(res.Objects) != 1 {
		t.Fatalf("Expected 1 match, got %d", len(res.Objects))
	}
	if res.Objects[0].Object[objects.FieldKeyID] != "doc-1" {
		t.Errorf("Expected match doc-1, got %v", res.Objects[0].Object[objects.FieldKeyID])
	}
	if res.Objects[0].Score < 0.5 {
		t.Errorf("Expected score >= 0.5, got %f", res.Objects[0].Score)
	}
}

func TestDSIAStorageProvider_Transaction(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_tx_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// Begin transaction
	tx, err := provider.BeginTransaction(ctx)
	if err != nil {
		t.Fatalf("BeginTransaction failed: %v", err)
	}

	// Create object in tx
	err = tx.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "tx_kind", objects.FieldKeyID: "tx-1", objects.FieldKeyTitle: "Staged"})
	if err != nil {
		t.Fatalf("tx.Create failed: %v", err)
	}

	// Read inside tx should see it
	staged, err := tx.Read(ctx, nil, "tx-1")
	if err != nil {
		t.Fatalf("tx.Read failed: %v", err)
	}
	if staged[objects.FieldKeyTitle] != "Staged" {
		t.Errorf("Expected title 'Staged', got %v", staged[objects.FieldKeyTitle])
	}

	// Read outside tx should NOT see it yet
	exists, _ := provider.Exists(ctx, nil, "tx-1")
	if exists {
		t.Errorf("Expected tx-1 to NOT exist on provider before commit")
	}

	// Commit tx
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("tx.Commit failed: %v", err)
	}

	// Now provider should see it
	existsAfter, _ := provider.Exists(ctx, nil, "tx-1")
	if !existsAfter {
		t.Errorf("Expected tx-1 to exist on provider after commit")
	}
}

func TestDSIAStorageProvider_GraphAndRefactoring(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "dsia_graph_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	provider := NewDSIAStorageProvider(tempDir)
	ctx := context.Background()

	// Create referenced object & parent object with reference
	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "target", objects.FieldKeyID: "target-1", objects.FieldKeyTitle: "Target"})
	_ = provider.Create(ctx, nil, map[string]any{objects.FieldKeyKind: "parent", objects.FieldKeyID: "parent-1", objects.FieldKeyTitle: "Parent", "target_ref": "target-1"})

	// Test GetRelated
	related, err := provider.GetRelated(ctx, nil, "parent-1", "target_ref", 1)
	if err != nil {
		t.Fatalf("GetRelated failed: %v", err)
	}
	if len(related) != 1 {
		t.Errorf("Expected 1 related object, got %d", len(related))
	}

	// Test Rename
	if err := provider.Rename(ctx, nil, "target-1", "target-renamed", false); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	existsOld, _ := provider.Exists(ctx, nil, "target-1")
	if existsOld {
		t.Errorf("Expected old ID to no longer exist after Rename")
	}
	existsNew, _ := provider.Exists(ctx, nil, "target-renamed")
	if !existsNew {
		t.Errorf("Expected renamed ID to exist")
	}

	// Test Move (change kind from target -> goal)
	if err := provider.Move(ctx, nil, "target-renamed", "goal", false); err != nil {
		t.Fatalf("Move failed: %v", err)
	}
	movedObj, err := provider.Read(ctx, nil, "target-renamed")
	if err != nil {
		t.Fatalf("Read after Move failed: %v", err)
	}
	if movedObj[objects.FieldKeyKind] != "goal" {
		t.Errorf("Expected kind 'goal', got '%v'", movedObj[objects.FieldKeyKind])
	}
}
// tdd refresh
