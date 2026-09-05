package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestReconstructStateAtTimestamp_NoChanges(t *testing.T) {
	// Test case: Object hasn't changed since snapshot
	testStorage := &TestStorage{objects: make(map[string]map[string]any)}
	logger := logging.GetLoggerFromProfile("test")

	// Create a test object
	testObj := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "Test Object",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2030-01-05T10:00:00Z",
	}

	snapshotTime := time.Date(2030, 1, 5, 10, 0, 0, 0, time.UTC)

	// Reconstruct (should return current state since no changes)
	reconstructed, err := ReconstructStateAtTimestamp(
		pkgctx.NewSystemContext(),
		testStorage,
		testObj,
		"TEST-001",
		"test_object",
		snapshotTime,
		logger,
	)

	if err != nil {
		t.Fatalf("ReconstructStateAtTimestamp failed: %v", err)
	}

	if reconstructed == nil {
		t.Fatal("Reconstructed state is nil")
	}

	if reconstructed[objects.FieldKeyTitle] != "Test Object" {
		t.Errorf("Expected title 'Test Object', got %v", reconstructed[objects.FieldKeyTitle])
	}
}

// TestStorage is a test implementation of ObjectStorageProvider with proper List support
type TestStorage struct {
	objects map[string]map[string]any
}

func (ts *TestStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	ts.objects[id] = obj
	return nil
}

func (ts *TestStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return ts.objects[id], nil
}

func (ts *TestStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	obj := ts.objects[id]
	for k, v := range updates {
		obj[k] = v
	}
	return nil
}

func (ts *TestStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	delete(ts.objects, id)
	return nil
}

//nolint:gocritic // Test mock signature follows interface (value filter)
func (ts *TestStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	var results []map[string]any
	for _, obj := range ts.objects {
		// Filter by kind
		if objKind, ok := obj[objects.FieldKeyKind].(string); ok && objKind == filter.Kind {
			// Check object_ref filter if present
			if objectRef, ok := filter.Filters[objects.FieldKeyObjectRef].(string); ok {
				if objRef, ok := obj[objects.FieldKeyObjectRef].(string); ok && objRef == objectRef {
					// Check created_at filter if present
					if timeFilter, ok := filter.Filters[objects.FieldKeyCreatedAt].(map[string]any); ok {
						objTimeStr, _ := obj[objects.FieldKeyCreatedAt].(string)
						objTime, err := time.Parse(time.RFC3339, objTimeStr)
						if err != nil {
							continue // Skip if can't parse time
						}
						if gt, ok := timeFilter["$gt"].(string); ok {
							gtTime, err := time.Parse(time.RFC3339, gt)
							if err != nil {
								continue // Skip if can't parse time
							}
							if !objTime.After(gtTime) {
								continue // Skip if not after the threshold
							}
						}
					}
					results = append(results, obj)
				}
			} else {
				// No object_ref filter, include all of this kind
				results = append(results, obj)
			}
		}
	}
	return &QueryResult{Objects: results}, nil
}

func (ts *TestStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	return &QueryResult{}, nil
}

//nolint:gocritic // Test mock signature follows interface (value query)
func (ts *TestStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	return &SearchResult{}, nil
}

func (ts *TestStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return nil, nil
}

func (ts *TestStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	return &BulkResult{}, nil
}

func (ts *TestStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	return &BulkResult{}, nil
}

func (ts *TestStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	return &BulkResult{}, nil
}

func (ts *TestStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	return &BulkResult{}, nil
}

func (ts *TestStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id, relationshipType string, depth int) ([]map[string]any, error) {
	return nil, nil
}

func (ts *TestStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return nil, nil
}

func (ts *TestStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id, direction string) ([]map[string]any, error) {
	return nil, nil
}

func (ts *TestStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	return nil
}

func (ts *TestStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	return nil
}

//nolint:gocritic // Test mock signature follows interface (value filter)
func (ts *TestStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return &AggregateResult{}, nil
}

func (ts *TestStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	_, ok := ts.objects[id]
	return ok, nil
}

//nolint:gocritic // Test mock signature follows interface (value filter)
func (ts *TestStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	return len(ts.objects), nil
}

func TestReconstructStateAtTimestamp_WithUpdate(t *testing.T) {
	// Test case: Object was updated after snapshot, need to reverse the update
	testStorage := &TestStorage{objects: make(map[string]map[string]any)}
	logger := logging.GetLoggerFromProfile("test")

	// Create change journal entry for an update
	previousState := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "Original Title",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2030-01-05T10:00:00Z",
	}

	changeEntry := map[string]any{
		objects.FieldKeyID:            "CHA-001",
		objects.FieldKeyKind:          "change_journal_entry",
		objects.FieldKeyObjectRef:     "test_object:TEST-001",
		objects.FieldKeyChangeType:    "update",
		objects.FieldKeyPreviousState: previousState,
		objects.FieldKeyCreatedAt:     "2030-01-05T11:00:00Z", // After snapshot
	}

	// Current state (after update)
	currentObj := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "Updated Title",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2030-01-05T11:00:00Z",
	}

	// Add change journal entry to storage
	testStorage.objects["CHA-001"] = changeEntry

	snapshotTime := time.Date(2030, 1, 5, 10, 0, 0, 0, time.UTC)

	// Reconstruct
	reconstructed, err := ReconstructStateAtTimestamp(
		pkgctx.NewSystemContext(),
		testStorage,
		currentObj,
		"TEST-001",
		"test_object",
		snapshotTime,
		logger,
	)

	if err != nil {
		t.Fatalf("ReconstructStateAtTimestamp failed: %v", err)
	}

	if reconstructed == nil {
		t.Fatal("Reconstructed state is nil")
	}

	// Should have original title
	if reconstructed[objects.FieldKeyTitle] != "Original Title" {
		t.Errorf("Expected title 'Original Title', got %v", reconstructed[objects.FieldKeyTitle])
	}
}

func TestReconstructStateAtTimestamp_WithDelete(t *testing.T) {
	// Test case: Object was deleted after snapshot, need to restore it
	testStorage := &TestStorage{objects: make(map[string]map[string]any)}
	logger := logging.GetLoggerFromProfile("test")

	// Previous state before deletion
	previousState := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "Deleted Object",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2030-01-05T10:00:00Z",
	}

	changeEntry := map[string]any{
		objects.FieldKeyID:            "CHA-001",
		objects.FieldKeyKind:          "change_journal_entry",
		objects.FieldKeyObjectRef:     "test_object:TEST-001",
		objects.FieldKeyChangeType:    "delete",
		objects.FieldKeyPreviousState: previousState,
		objects.FieldKeyCreatedAt:     "2030-01-05T11:00:00Z", // After snapshot
	}

	// Current state (object doesn't exist, but we'll pass nil or empty)
	currentObj := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "Deleted Object",
		objects.FieldKeyStatus:    objects.ObjectStatusDeleted,
		objects.FieldKeyUpdatedAt: "2030-01-05T11:00:00Z",
	}

	// Add change journal entry to storage
	testStorage.objects["CHA-001"] = changeEntry

	snapshotTime := time.Date(2030, 1, 5, 10, 0, 0, 0, time.UTC)

	// Reconstruct
	reconstructed, err := ReconstructStateAtTimestamp(
		pkgctx.NewSystemContext(),
		testStorage,
		currentObj,
		"TEST-001",
		"test_object",
		snapshotTime,
		logger,
	)

	if err != nil {
		t.Fatalf("ReconstructStateAtTimestamp failed: %v", err)
	}

	if reconstructed == nil {
		t.Fatal("Reconstructed state is nil")
	}

	// Should have restored previous state
	if reconstructed[objects.FieldKeyTitle] != "Deleted Object" {
		t.Errorf("Expected title 'Deleted Object', got %v", reconstructed[objects.FieldKeyTitle])
	}
}

func TestReconstructStateAtTimestamp_WithCreateAfterSnapshot(t *testing.T) {
	// Test case: Object was created after snapshot, should return error
	testStorage := &TestStorage{objects: make(map[string]map[string]any)}
	logger := logging.GetLoggerFromProfile("test")

	// Change entry for create
	changeEntry := map[string]any{
		objects.FieldKeyID:         "CHA-001",
		objects.FieldKeyKind:       "change_journal_entry",
		objects.FieldKeyObjectRef:  "test_object:TEST-001",
		objects.FieldKeyChangeType: "create",
		objects.FieldKeyCreatedAt:  "2030-01-05T11:00:00Z", // After snapshot
	}

	currentObj := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "New Object",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2030-01-05T11:00:00Z",
	}

	// Add change journal entry to storage
	testStorage.objects["CHA-001"] = changeEntry

	snapshotTime := time.Date(2030, 1, 5, 10, 0, 0, 0, time.UTC)

	// Reconstruct should fail
	_, err := ReconstructStateAtTimestamp(
		pkgctx.NewSystemContext(),
		testStorage,
		currentObj,
		"TEST-001",
		"test_object",
		snapshotTime,
		logger,
	)

	if err == nil {
		t.Fatal("Expected error for object created after snapshot, got nil")
	}

	if !errorContains(err, "was created after snapshot timestamp") {
		t.Errorf("Expected error about object created after snapshot, got: %v", err)
	}
}

func TestReconstructStateAtTimestamp_MultipleUpdates(t *testing.T) {
	// Test case: Multiple updates after snapshot, need to reverse all
	testStorage := &TestStorage{objects: make(map[string]map[string]any)}
	logger := logging.GetLoggerFromProfile("test")

	// State at snapshot
	stateAtSnapshot := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "Snapshot Title",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2030-01-05T10:00:00Z",
	}

	// First update (T2)
	stateAfterFirstUpdate := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "First Update",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2030-01-05T11:00:00Z",
	}

	changeEntry1 := map[string]any{
		objects.FieldKeyID:            "CHA-001",
		objects.FieldKeyKind:          "change_journal_entry",
		objects.FieldKeyObjectRef:     "test_object:TEST-001",
		objects.FieldKeyChangeType:    "update",
		objects.FieldKeyPreviousState: stateAtSnapshot,
		objects.FieldKeyCreatedAt:     "2030-01-05T11:00:00Z",
	}

	// Second update (T3)
	currentState := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyTitle:     "Second Update",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2030-01-05T12:00:00Z",
	}

	changeEntry2 := map[string]any{
		objects.FieldKeyID:            "CHA-002",
		objects.FieldKeyKind:          "change_journal_entry",
		objects.FieldKeyObjectRef:     "test_object:TEST-001",
		objects.FieldKeyChangeType:    "update",
		objects.FieldKeyPreviousState: stateAfterFirstUpdate,
		objects.FieldKeyCreatedAt:     "2030-01-05T12:00:00Z",
	}

	// Add change journal entries to storage
	testStorage.objects["CHA-001"] = changeEntry1
	testStorage.objects["CHA-002"] = changeEntry2

	snapshotTime := time.Date(2030, 1, 5, 10, 0, 0, 0, time.UTC)

	// Reconstruct
	reconstructed, err := ReconstructStateAtTimestamp(
		pkgctx.NewSystemContext(),
		testStorage,
		currentState,
		"TEST-001",
		"test_object",
		snapshotTime,
		logger,
	)

	if err != nil {
		t.Fatalf("ReconstructStateAtTimestamp failed: %v", err)
	}

	if reconstructed == nil {
		t.Fatal("Reconstructed state is nil")
	}

	// Should have snapshot title after reversing both updates
	if reconstructed[objects.FieldKeyTitle] != "Snapshot Title" {
		t.Errorf("Expected title 'Snapshot Title', got %v", reconstructed[objects.FieldKeyTitle])
	}
}

// Helper function to check if string contains substring
func errorContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	for i := 0; i <= len(errStr)-len(substr); i++ {
		if errStr[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
func (ts *TestStorage) Shutdown(ctx context.Context) error { return nil }

func TestChangeJournalReconstructionService_LifetimeCounters(t *testing.T) {
	testStorage := &TestStorage{objects: make(map[string]map[string]any)}
	logger := logging.GetLoggerFromProfile("test")
	service := NewChangeJournalReconstructionService(testStorage)

	runs, reversed := service.GetReconstructionStats()
	if runs != 0 || reversed != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", runs, reversed)
	}

	snapshotTime := time.Date(2030, 1, 5, 10, 0, 0, 0, time.UTC)
	afterTime := time.Date(2030, 1, 5, 11, 0, 0, 0, time.UTC)

	// Add change journal entry after snapshot
	entry := map[string]any{
		objects.FieldKeyID:         "CJE-001",
		objects.FieldKeyKind:       objects.KindChangeJournalEntry,
		objects.FieldKeyObjectRef:  "test_object:TEST-001",
		objects.FieldKeyChangeType: OpUpdate,
		objects.FieldKeyCreatedAt:  afterTime.Format(time.RFC3339),
		objects.FieldKeyPreviousState: map[string]any{
			objects.FieldKeyID:    "TEST-001",
			objects.FieldKeyKind:  "test_object",
			objects.FieldKeyTitle: "Original Title",
		},
	}
	_ = testStorage.Create(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext(), entry)

	currentState := map[string]any{
		objects.FieldKeyID:    "TEST-001",
		objects.FieldKeyKind:  "test_object",
		objects.FieldKeyTitle: "Updated Title",
	}

	reconstructed, err := service.ReconstructStateAtTimestamp(
		pkgctx.NewSystemContext(),
		currentState,
		"TEST-001",
		"test_object",
		snapshotTime,
		logger,
	)
	if err != nil {
		t.Fatalf("ReconstructStateAtTimestamp failed: %v", err)
	}
	if reconstructed == nil {
		t.Fatal("Reconstructed state is nil")
	}
	if reconstructed[objects.FieldKeyTitle] != "Original Title" {
		t.Errorf("Expected title 'Original Title', got %v", reconstructed[objects.FieldKeyTitle])
	}

	runs, reversed = service.GetReconstructionStats()
	if runs != 1 || reversed != 1 {
		t.Errorf("expected reconstructionsRun=1 entriesReversed=1, got runs=%d reversed=%d", runs, reversed)
	}
}
