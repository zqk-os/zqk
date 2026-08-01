package rollback

import (
	"context"
	"os"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

const testEmptyID = ""

func TestCapture_Disabled(t *testing.T) {
	t.Parallel()
	os.Setenv(zqkenv.RollbackCaptureDisabled(), "1")
	defer os.Unsetenv(zqkenv.RollbackCaptureDisabled())
	// Reset config cache by running DefaultConfig after env set
	_ = DefaultConfig()

	dir := t.TempDir()
	id, err := Capture(dir, ScopeTypeLifecycle, "plan:1", func() ([]ObjectState, error) {
		return []ObjectState{{Kind: "backlog_item", ID: "ITEM-1", State: map[string]any{objects.FieldKeyStatus: "complete"}}}, nil
	})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if id != testEmptyID {
		t.Errorf("Capture with disabled: expected empty ID, got %q", id)
	}
}

func TestCapture_Apply_List(t *testing.T) {
	t.Parallel()
	if os.Getenv(zqkenv.RollbackCaptureDisabled()) == "1" {
		t.Skip("capture disabled by env")
	}
	dir := t.TempDir()
	states := []ObjectState{
		{Kind: "backlog_item", ID: "ITEM-1", State: map[string]any{objects.FieldKeyID: "ITEM-1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: "complete"}},
	}
	id, err := Capture(dir, ScopeTypeMaintenance, "task-1", func() ([]ObjectState, error) { return states, nil })
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if id == testEmptyID {
		t.Error("Capture: expected non-empty ID")
	}

	metas, err := List(dir, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 || metas[0].ID != id {
		t.Errorf("List: got %+v", metas)
	}

	// Apply requires a storage provider that can Update; we only verify Get returns the point
	p, err := Get(dir, id)
	if err != nil || p == nil {
		t.Fatalf("Get: %v", err)
	}
	if len(p.ObjectStates) != 1 || p.ObjectStates[0].ID != "ITEM-1" {
		t.Errorf("Get: got %+v", p)
	}
}

func TestSnapshotObjectStates_EmptyRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	got, err := SnapshotObjectStates(ctx, nil, nil)
	if err != nil {
		t.Fatalf("SnapshotObjectStates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 states, got %d", len(got))
	}
}

func TestSnapshotObjectStates_MockStorage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mock := &mockRollbackStorage{
		objects: map[string]map[string]any{
			"ITEM-1": {objects.FieldKeyID: "ITEM-1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: "complete"},
			"PLAN-1": {objects.FieldKeyID: "PLAN-1", objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "active"},
		},
	}
	refs := []ObjectRef{{Kind: "backlog_item", ID: "ITEM-1"}, {Kind: "priority_plan", ID: "PLAN-1"}}
	got, err := SnapshotObjectStates(ctx, mock, refs)
	if err != nil {
		t.Fatalf("SnapshotObjectStates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 states, got %d", len(got))
	}
	byID := make(map[string]ObjectState)
	for _, s := range got {
		byID[s.ID] = s
	}
	if byID["ITEM-1"].State[objects.FieldKeyStatus] != "complete" || byID["PLAN-1"].State[objects.FieldKeyStatus] != "active" {
		t.Errorf("SnapshotObjectStates: got %+v", got)
	}
}

type mockRollbackStorage struct {
	objects map[string]map[string]any
}

func (m *mockRollbackStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockRollbackStorage) GenerateID(ctx context.Context, kind string) (string, error) {
	return "MOCK-123", nil
}

func (m *mockRollbackStorage) Create(context.Context, *pkgctx.SecurityContext, map[string]any) error {
	return nil
}
func (m *mockRollbackStorage) Update(context.Context, *pkgctx.SecurityContext, string, map[string]any) error {
	return nil
}
func (m *mockRollbackStorage) Delete(context.Context, *pkgctx.SecurityContext, string, bool) error {
	return nil
}
func (m *mockRollbackStorage) List(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storage.ListFilter) (*storage.QueryResult, error) {
	// Return empty list so ReconstructStateAtTimestamp returns current state (no journal entries after snapshot).
	return &storage.QueryResult{Objects: []map[string]any{}}, nil
}
func (m *mockRollbackStorage) Count(context.Context, *pkgctx.SecurityContext, storage.ListFilter) (int, error) {
	return 0, nil
}
func (m *mockRollbackStorage) GetRelated(context.Context, *pkgctx.SecurityContext, string, string, int) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockRollbackStorage) GetPath(context.Context, *pkgctx.SecurityContext, string, string) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockRollbackStorage) GetNeighbors(context.Context, *pkgctx.SecurityContext, string, string) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockRollbackStorage) Exists(context.Context, *pkgctx.SecurityContext, string) (bool, error) {
	return false, nil
}
func (m *mockRollbackStorage) BeginTransaction(context.Context) (storage.ObjectTransaction, error) {
	return nil, nil
}
func (m *mockRollbackStorage) Query(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storage.Query) (*storage.QueryResult, error) {
	return nil, nil
}
func (m *mockRollbackStorage) Aggregate(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storage.ListFilter, []storage.Aggregation) (*storage.AggregateResult, error) {
	return nil, nil
}
func (m *mockRollbackStorage) Search(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storage.SearchQuery) (*storage.SearchResult, error) {
	return nil, nil
}
func (m *mockRollbackStorage) BulkCreate(context.Context, *pkgctx.SecurityContext, []map[string]any) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockRollbackStorage) BulkUpdate(context.Context, *pkgctx.SecurityContext, []storage.BulkUpdateItem) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockRollbackStorage) BulkGet(context.Context, *pkgctx.SecurityContext, []string) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockRollbackStorage) BulkDelete(context.Context, *pkgctx.SecurityContext, []string, bool) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockRollbackStorage) Move(context.Context, *pkgctx.SecurityContext, string, string, bool) error {
	return nil
}
func (m *mockRollbackStorage) Rename(context.Context, *pkgctx.SecurityContext, string, string, bool) error {
	return nil
}

// TestApplyReconstruct_NoJournalEntries runs ApplyReconstruct when there are no change journal
// entries after the target timestamp (reconstruction returns current state, then we "apply" it).
func TestApplyReconstruct_NoJournalEntries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mock := &mockRollbackStorage{
		objects: map[string]map[string]any{
			"ITEM-1": {objects.FieldKeyID: "ITEM-1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: "complete"},
		},
	}
	refs := []ObjectRef{{Kind: "backlog_item", ID: "ITEM-1"}}
	targetTimestamp := time.Now().UTC().Add(-time.Hour)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	err := ApplyReconstruct(ctx, t.TempDir(), targetTimestamp, refs, mock, logger)
	if err != nil {
		t.Fatalf("ApplyReconstruct: %v", err)
	}
}

func (m *mockRollbackStorage) Shutdown(context.Context) error { return nil }
