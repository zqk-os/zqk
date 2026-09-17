package docman

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

type mockDocStorageProvider struct {
	storage.ObjectStorageProvider
	mu      sync.Mutex
	objects map[string]map[string]any
}

func newMockDocStorageProvider() *mockDocStorageProvider {
	return &mockDocStorageProvider{
		objects: make(map[string]map[string]any),
	}
}

func (m *mockDocStorageProvider) List(ctx context.Context, sec *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var res []map[string]any
	for _, obj := range m.objects {
		if filter.Kind == "" || obj[objects.FieldKeyKind] == filter.Kind {
			res = append(res, obj)
		}
	}
	return &storage.QueryResult{Objects: res}, nil
}

func (m *mockDocStorageProvider) Create(ctx context.Context, sec *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id, _ := obj[objects.FieldKeyID].(string)
	m.objects[id] = obj
	return nil
}

func (m *mockDocStorageProvider) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	obj, ok := m.objects[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return obj, nil
}

func (m *mockDocStorageProvider) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.objects[id]
	if !ok {
		existing = make(map[string]any)
		m.objects[id] = existing
	}
	for k, v := range obj {
		existing[k] = v
	}
	return nil
}

func TestRegistry_GetExistingEntriesAndCreate(t *testing.T) {
	t.Parallel()
	mockStore := newMockDocStorageProvider()
	tmpDir := t.TempDir()

	reg := NewRegistry(mockStore, tmpDir)
	ctx := context.Background()

	// Initial check on empty storage
	existing, maxID, err := reg.GetExistingEntries(ctx, "test")
	if err != nil {
		t.Fatalf("GetExistingEntries failed: %v", err)
	}
	if len(existing) != 0 || maxID != 0 {
		t.Fatalf("expected empty existing and maxID=0, got %v, %d", existing, maxID)
	}

	// Create a dummy markdown file
	docRelPath := "docs/architecture/design.md"
	docAbsPath := filepath.Join(tmpDir, docRelPath)
	if err := os.MkdirAll(filepath.Dir(docAbsPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(docAbsPath, []byte("# System Design\n\n## Overview\n\nHigh level architecture."), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	file := &MarkdownFile{
		Path:    docAbsPath,
		RelPath: docRelPath,
	}
	meta := &DocumentMetadata{
		Title:    "System Design",
		Summary:  "High level architecture.",
		Status:   "active",
		Group:    "architecture",
		Category: "design",
	}

	err = reg.CreateDocEntry(ctx, "test", file, meta, "DOC-042")
	if err != nil {
		t.Fatalf("CreateDocEntry failed: %v", err)
	}

	// Re-check existing entries
	existing, maxID, err = reg.GetExistingEntries(ctx, "test")
	if err != nil {
		t.Fatalf("GetExistingEntries failed: %v", err)
	}
	if maxID != 42 {
		t.Errorf("expected maxID 42, got %d", maxID)
	}
	if len(existing) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(existing))
	}
}

func TestRegistry_UpdateExistingEntries(t *testing.T) {
	t.Parallel()
	mockStore := newMockDocStorageProvider()
	tmpDir := t.TempDir()

	docRelPath := "docs/guide.md"
	docAbsPath := filepath.Join(tmpDir, docRelPath)
	if err := os.MkdirAll(filepath.Dir(docAbsPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(docAbsPath, []byte("# Guide Content"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	// Seed incomplete entry
	mockStore.objects["DOC-001"] = map[string]any{
		objects.FieldKeyID:   "DOC-001",
		objects.FieldKeyKind: objects.KindDocEntry,
		objects.FieldKeyPath: docRelPath,
	}

	reg := NewRegistry(mockStore, tmpDir)
	ctx := context.Background()

	updated, err := reg.UpdateExistingEntries(ctx, "test")
	if err != nil {
		t.Fatalf("UpdateExistingEntries failed: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 entry updated, got %d", updated)
	}

	obj := mockStore.objects["DOC-001"]
	if _, ok := obj[objects.FieldKeyGoalRefs]; !ok {
		t.Errorf("expected goal_refs to be populated")
	}
	if _, ok := obj["content_hash"]; !ok {
		t.Errorf("expected content_hash to be populated")
	}
}
