package system

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/interactive"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// mockObjectStorageProvider is a simple mock for testing
type mockObjectStorageProvider struct {
	objects map[string]map[string]any
}

func newMockObjectStorageProvider() *mockObjectStorageProvider {
	return &mockObjectStorageProvider{
		objects: make(map[string]map[string]any),
	}
}

func (m *mockObjectStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, exists := m.objects[id]; exists {
		// Return a copy to avoid mutations
		copy := make(map[string]any)
		maps.Copy(copy, obj)
		return copy, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (m *mockObjectStorageProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id, ok := obj[objects.FieldKeyID].(string)
	if !ok || id == emptyValue {
		return fmt.Errorf("object missing id")
	}
	m.objects[id] = obj
	return nil
}

func (m *mockObjectStorageProvider) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	obj, exists := m.objects[id]
	if !exists {
		return storage.ErrObjectNotFound
	}
	maps.Copy(obj, updates)
	return nil
}

func (m *mockObjectStorageProvider) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	delete(m.objects, id)
	return nil
}

func (m *mockObjectStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, query storage.Query) (*storage.QueryResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, query storage.SearchQuery) (*storage.SearchResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) BeginTransaction(ctx context.Context) (storage.ObjectTransaction, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, rows []map[string]any) (*storage.BulkResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []storage.BulkUpdateItem) (*storage.BulkResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*storage.BulkResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*storage.BulkResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	_, exists := m.objects[id]
	return exists, nil
}

func (m *mockObjectStorageProvider) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	obj, exists := m.objects[oldID]
	if !exists {
		return storage.ErrObjectNotFound
	}
	delete(m.objects, oldID)
	obj[objects.FieldKeyID] = newID
	m.objects[newID] = obj
	return nil
}

func (m *mockObjectStorageProvider) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter, aggregations []storage.Aggregation) (*storage.AggregateResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockObjectStorageProvider) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	return fmt.Errorf("not implemented")
}

func TestUpdateLoopProcessor_ProcessUpdateLoop_Basic(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("Specs directory not available (e.g. under bundler): %v", err)
		}
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	mockStorage := newMockObjectStorageProvider()
	processor := NewUpdateLoopProcessor(generator, mockStorage)

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "test:user",
	}

	// Create a test object
	objectID := "TEST-001"
	testObj := map[string]any{
		objects.FieldKeyID:       objectID,
		objects.FieldKeyKind:     "criteria",
		objects.FieldKeyTitle:    "Original Title",
		objects.FieldKeyStatus:   objects.ObjectStatusNotStarted,
		objects.FieldKeyCategory: "acceptance",
	}
	mockStorage.objects[objectID] = testObj

	// Test: Update title field
	providedValues := map[string]any{
		objects.FieldKeyTitle: "Updated Title",
	}

	loopState, err := processor.ProcessUpdateLoop(ctx, secCtx, objectID, []string{objects.FieldKeyTitle}, providedValues)
	if err != nil {
		t.Fatalf("ProcessUpdateLoop failed: %v", err)
	}

	// Verify state
	if loopState.ObjectID != objectID {
		t.Errorf("ObjectID = %q, want %q", loopState.ObjectID, objectID)
	}
	if loopState.Kind != "criteria" {
		t.Errorf("Kind = %q, want %q", loopState.Kind, "criteria")
	}

	// Verify changed fields
	if len(loopState.ChangedFields) != 1 {
		t.Errorf("ChangedFields length = %d, want 1", len(loopState.ChangedFields))
	}
	if len(loopState.ChangedFields) > 0 && loopState.ChangedFields[0] != objects.FieldKeyTitle {
		t.Errorf("ChangedFields[0] = %q, want %q", loopState.ChangedFields[0], objects.FieldKeyTitle)
	}

	// Verify current values
	if currentTitle, exists := loopState.CurrentValues[objects.FieldKeyTitle]; !exists {
		t.Error("CurrentValues missing title")
	} else if currentTitle != "Original Title" {
		t.Errorf("CurrentValues[title] = %q, want %q", currentTitle, "Original Title")
	}
}

func TestUpdateLoopProcessor_DetectChangedFields(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("Specs directory not available (e.g. under bundler): %v", err)
		}
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	mockStorage := newMockObjectStorageProvider()
	processor := NewUpdateLoopProcessor(generator, mockStorage)

	existingObj := map[string]any{
		objects.FieldKeyTitle:    "Original Title",
		objects.FieldKeyStatus:   objects.ObjectStatusNotStarted,
		objects.FieldKeyCategory: "acceptance",
	}

	providedValues := map[string]any{
		objects.FieldKeyTitle:  "Updated Title",                // Changed
		objects.FieldKeyStatus: objects.ObjectStatusNotStarted, // Same (no change)
	}

	changedFields := processor.detectChangedFields(existingObj, providedValues, []string{objects.FieldKeyTitle, objects.FieldKeyStatus})

	// Should only detect title as changed
	if len(changedFields) != 1 {
		t.Errorf("ChangedFields length = %d, want 1", len(changedFields))
	}
	if len(changedFields) > 0 && changedFields[0] != objects.FieldKeyTitle {
		t.Errorf("ChangedFields[0] = %q, want %q", changedFields[0], objects.FieldKeyTitle)
	}
}

func TestUpdateLoopProcessor_DetectChangedFields_IgnoresImmutable(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("Specs directory not available (e.g. under bundler): %v", err)
		}
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	mockStorage := newMockObjectStorageProvider()
	processor := NewUpdateLoopProcessor(generator, mockStorage)

	existingObj := map[string]any{
		objects.FieldKeyID:    "TEST-001",
		objects.FieldKeyKind:  "criteria",
		objects.FieldKeyTitle: "Original Title",
	}

	providedValues := map[string]any{
		objects.FieldKeyID:    "TEST-002",      // Tried to change ID (immutable)
		objects.FieldKeyKind:  "backlog_item",  // Tried to change kind (immutable)
		objects.FieldKeyTitle: "Updated Title", // Changed title (mutable)
	}

	changedFields := processor.detectChangedFields(existingObj, providedValues, []string{objects.FieldKeyID, objects.FieldKeyKind, objects.FieldKeyTitle})

	// Should only detect title as changed (id and kind are immutable)
	if len(changedFields) != 1 {
		t.Errorf("ChangedFields length = %d, want 1", len(changedFields))
	}
	if len(changedFields) > 0 && changedFields[0] != objects.FieldKeyTitle {
		t.Errorf("ChangedFields[0] = %q, want %q", changedFields[0], objects.FieldKeyTitle)
	}
}

func TestUpdateLoopProcessor_GeneratePreviewDiff(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("Specs directory not available (e.g. under bundler): %v", err)
		}
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	mockStorage := newMockObjectStorageProvider()
	processor := NewUpdateLoopProcessor(generator, mockStorage)

	existingObj := map[string]any{
		objects.FieldKeyTitle:  "Original Title",
		objects.FieldKeyStatus: objects.ObjectStatusNotStarted,
	}

	providedValues := map[string]any{
		objects.FieldKeyTitle:  "Updated Title",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}

	changedFields := []string{objects.FieldKeyTitle, objects.FieldKeyStatus}

	diff := processor.generatePreviewDiff(existingObj, providedValues, changedFields)

	// Verify diff contains expected information
	if diff == emptyValue {
		t.Error("PreviewDiff is empty")
	}
	if !strings.Contains(diff, "Changes to be applied") {
		t.Error("PreviewDiff missing header")
	}
	if !strings.Contains(diff, "title") {
		t.Error("PreviewDiff missing title change")
	}
	if !strings.Contains(diff, "status") {
		t.Error("PreviewDiff missing status change")
	}
}

func TestUpdateLoopProcessor_BuildCurrentValues(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("Specs directory not available (e.g. under bundler): %v", err)
		}
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	mockStorage := newMockObjectStorageProvider()
	processor := NewUpdateLoopProcessor(generator, mockStorage)

	existingObj := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "criteria",
		objects.FieldKeyTitle:     "Original Title",
		objects.FieldKeyStatus:    objects.ObjectStatusNotStarted,
		objects.FieldKeyCreatedAt: "2025-01-01T00:00:00Z", // Immutable
	}

	// Test with specific fields
	currentValues := processor.buildCurrentValues(existingObj, []string{objects.FieldKeyTitle, objects.FieldKeyStatus})

	if len(currentValues) != 2 {
		t.Errorf("CurrentValues length = %d, want 2", len(currentValues))
	}
	if currentValues[objects.FieldKeyTitle] != "Original Title" {
		t.Errorf("CurrentValues[title] = %q, want %q", currentValues[objects.FieldKeyTitle], "Original Title")
	}
	if currentValues[objects.FieldKeyStatus] != "not_started" {
		t.Errorf("CurrentValues[status] = %q, want %q", currentValues[objects.FieldKeyStatus], "not_started")
	}

	// Test that immutable fields are excluded
	if _, exists := currentValues[objects.FieldKeyID]; exists {
		t.Error("CurrentValues should not contain immutable field id")
	}
	if _, exists := currentValues[objects.FieldKeyCreatedAt]; exists {
		t.Error("CurrentValues should not contain immutable field created_at")
	}
}

func TestUpdateLoopProcessor_BuildCurrentValues_AllFields(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("Specs directory not available (e.g. under bundler): %v", err)
		}
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	mockStorage := newMockObjectStorageProvider()
	processor := NewUpdateLoopProcessor(generator, mockStorage)

	existingObj := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "criteria",
		objects.FieldKeyTitle:     "Original Title",
		objects.FieldKeyStatus:    objects.ObjectStatusNotStarted,
		objects.FieldKeyCreatedAt: "2025-01-01T00:00:00Z", // Immutable
	}

	// Test with no specific fields (all updatable fields)
	currentValues := processor.buildCurrentValues(existingObj, []string{})

	// Should include updatable fields but not immutable ones
	if _, exists := currentValues[objects.FieldKeyID]; exists {
		t.Error("CurrentValues should not contain immutable field id")
	}
	if _, exists := currentValues[objects.FieldKeyKind]; exists {
		t.Error("CurrentValues should not contain immutable field kind")
	}
	if _, exists := currentValues[objects.FieldKeyCreatedAt]; exists {
		t.Error("CurrentValues should not contain immutable field created_at")
	}

	// Should include mutable fields
	if _, exists := currentValues[objects.FieldKeyTitle]; !exists {
		t.Error("CurrentValues should contain updatable field title")
	}
	if _, exists := currentValues[objects.FieldKeyStatus]; !exists {
		t.Error("CurrentValues should contain updatable field status")
	}
}

func (m *mockObjectStorageProvider) Shutdown(ctx context.Context) error {
	return nil
}
