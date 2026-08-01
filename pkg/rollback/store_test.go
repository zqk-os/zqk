package rollback

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestNewStore_EmptyProjectRoot(t *testing.T) {
	t.Parallel()
	_, err := NewStore("")
	if err == nil {
		t.Error("NewStore(\"\") expected error")
	}
}

func TestStore_AppendListGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	p1 := &RollbackPoint{
		ID:           "rb-20260101-120000.000000000",
		Timestamp:    time.Now().UTC(),
		ScopeType:    ScopeTypeLifecycle,
		ScopeID:      "priority_plan:PLAN-1:complete",
		ObjectStates: []ObjectState{{Kind: "priority_plan", ID: "PLAN-1", State: map[string]any{objects.FieldKeyStatus: "active"}}},
	}
	if err := store.Append(p1); err != nil {
		t.Fatalf("Append: %v", err)
	}

	metas, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("List: got %d metas, want 1", len(metas))
	}
	if metas[0].ID != p1.ID || metas[0].ScopeID != p1.ScopeID || metas[0].ObjectCount != 1 {
		t.Errorf("List: got %+v", metas[0])
	}

	got, err := store.Get(p1.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != p1.ID || len(got.ObjectStates) != 1 {
		t.Errorf("Get: got %+v", got)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got, err := store.Get("rb-nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("Get: expected nil for missing id, got %+v", got)
	}
}

func TestStore_List_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	metas, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("List: got %d metas, want 0", len(metas))
	}
}

func TestStore_Retain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	// Append 3 points
	for i := 0; i < 3; i++ {
		p := &RollbackPoint{
			ID:        fmt.Sprintf("rb-20260101-12000%d.000000000", i),
			Timestamp: time.Now().UTC().Add(-time.Duration(i) * time.Hour),
			ScopeType: ScopeTypeMaintenance,
			ScopeID:   "task-1",
			ObjectStates: []ObjectState{
				{Kind: "backlog_item", ID: "ITEM-1", State: map[string]any{objects.FieldKeyStatus: "complete"}},
			},
		}
		if err := store.Append(p); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	metas, _ := store.List()
	if len(metas) != 3 {
		t.Fatalf("before Retain: got %d metas", len(metas))
	}
	// Retain last 2, within 48h
	if err := store.Retain(2, 48*time.Hour); err != nil {
		t.Fatalf("Retain: %v", err)
	}
	metas2, _ := store.List()
	if len(metas2) != 2 {
		t.Errorf("after Retain(2, 48h): got %d metas, want 2", len(metas2))
	}
}

func TestNewStore_CreatesDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	store, err := NewStore(sub)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	walDir := filepath.Join(sub, paths.ProjectDataDir, paths.WalDir)
	if _, err := os.Stat(walDir); os.IsNotExist(err) {
		t.Errorf("wal dir not created at %s", walDir)
	}
	// File is created on first Append
	if err := store.Append(&RollbackPoint{ID: "rb-1", Timestamp: time.Now().UTC(), ScopeType: ScopeTypeLifecycle, ScopeID: "s1", ObjectStates: []ObjectState{}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	walPath := filepath.Join(walDir, "rollback_points")
	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		t.Errorf("wal file not created at %s", walPath)
	}
}
