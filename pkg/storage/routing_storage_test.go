package storage

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

type recorderStorage struct {
	NoopObjectStorage
	name          string
	lastID        string
	lastKind      string
	lastMethod    string
	lastObj       map[string]any
	lastBulkCount int
}

func (m *recorderStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.lastMethod = "Create"
	m.lastObj = obj
	return nil
}

func (m *recorderStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.lastMethod = "Read"
	m.lastID = id
	return nil, nil
}

func (m *recorderStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.lastMethod = "Update"
	m.lastID = id
	m.lastObj = updates
	return nil
}

func (m *recorderStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	m.lastMethod = "Delete"
	m.lastID = id
	return nil
}

func (m *recorderStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	m.lastMethod = "List"
	m.lastKind = filter.Kind
	return &QueryResult{}, nil
}

func (m *recorderStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	m.lastMethod = "BulkCreate"
	m.lastBulkCount = len(objects)
	if len(objects) > 0 {
		m.lastObj = objects[0]
	}
	return &BulkResult{}, nil
}

func TestRoutingObjectStorage(t *testing.T) {
	mockDefault := &recorderStorage{name: "default"}
	mockKindA := &recorderStorage{name: "kindA"}

	factory := &StorageFactory{
		defaultStorage: mockDefault,
		providers: map[string]ObjectStorageProvider{
			"goal": mockKindA, // Using a real kind that InferKindFromID might return
		},
	}

	router := NewRoutingObjectStorage(factory)
	// Inject pattern for testing since we don't load real specs in this unit test
	router.idValidator.InjectPatternForTest("goal", "^GOAL-\\d{3}$", "GOAL-")

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{AccountID: "test"}

	// Test Create - should route based on object kind
	obj := map[string]any{objects.FieldKeyKind: "goal", "stewardship_profile": "active"}
	err := router.Create(ctx, secCtx, obj)
	if err != nil {
		t.Errorf("Create failed: %v", err)
	}
	if mockKindA.lastMethod != "Create" {
		t.Errorf("Expected Create on mockKindA, got %s on %s", mockKindA.lastMethod, mockKindA.name)
	}

	// Test Read - should route based on ID prefix
	// GOAL- is prefix for goal
	_, err = router.Read(ctx, secCtx, "GOAL-001")
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}
	if mockKindA.lastMethod != "Read" {
		t.Errorf("Expected Read on mockKindA, got %s", mockKindA.lastMethod)
	}
	if mockKindA.lastID != "GOAL-001" {
		t.Errorf("Expected ID GOAL-001, got %s", mockKindA.lastID)
	}

	// Test List - should route based on filter kind
	_, err = router.List(ctx, secCtx, nil, ListFilter{Kind: "goal"})
	if err != nil {
		t.Errorf("List failed: %v", err)
	}
	if mockKindA.lastMethod != "List" {
		t.Errorf("Expected List on mockKindA, got %s", mockKindA.lastMethod)
	}

	// Test BulkCreate - should route based on first object
	objs := []map[string]any{{objects.FieldKeyKind: "goal", "stewardship_profile": "active"}}
	_, err = router.BulkCreate(ctx, secCtx, objs)
	if err != nil {
		t.Errorf("BulkCreate failed: %v", err)
	}
	if mockKindA.lastMethod != "BulkCreate" {
		t.Errorf("Expected BulkCreate on mockKindA, got %s", mockKindA.lastMethod)
	}

	// Test default routing
	_, err = router.Read(ctx, secCtx, "UNK-001")
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}
	if mockDefault.lastMethod != "Read" {
		t.Errorf("Expected Read on mockDefault, got %s", mockDefault.lastMethod)
	}
}
