package docman

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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
		return nil, fileutil.ErrNotExist
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
	if err := fileutil.MkdirAll(filepath.Dir(docAbsPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(docAbsPath, []byte("# System Design\n\n## Overview\n\nHigh level architecture."), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	file := &MarkdownFile{
		Path:    docAbsPath,
		RelPath: docRelPath,
	}
	meta := &DocumentMetadata{
		Title:    "System Design",
		Summary:  "High level architecture.",
		Status:   objects.ObjectStatusActive,
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
	if err := fileutil.MkdirAll(filepath.Dir(docAbsPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(docAbsPath, []byte("# Guide Content"), 0644); err != nil {
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

func TestRegistry_RegisterAll_ContextCancellation(t *testing.T) {
	t.Parallel()
	mockStore := newMockDocStorageProvider()
	tmpDir := t.TempDir()

	for i := 1; i <= 5; i++ {
		docPath := filepath.Join(tmpDir, "docs", fmt.Sprintf("doc_%d.md", i))
		if err := fileutil.MkdirAll(filepath.Dir(docPath), 0755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := fileutil.WriteFile(docPath, []byte("# Title\nContent"), 0644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	reg := NewRegistry(mockStore, tmpDir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	created, _, err := reg.RegisterAll(ctx, "test", false)
	if err == nil {
		t.Fatalf("expected context.Canceled error, got nil")
	}
	if created != 0 {
		t.Errorf("expected 0 created on immediate cancel, got %d", created)
	}
}

func TestDocmanRegisterMeaningfulActivity(t *testing.T) {
	mockStore := newMockDocStorageProvider()
	tmpDir := t.TempDir()

	for i := 1; i <= 3; i++ {
		docPath := filepath.Join(tmpDir, "docs", fmt.Sprintf("doc_%d.md", i))
		if err := fileutil.MkdirAll(filepath.Dir(docPath), 0755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := fileutil.WriteFile(docPath, []byte(fmt.Sprintf("# Title %d\nContent", i)), 0644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	reg := NewRegistry(mockStore, tmpDir)
	created, skipped, err := reg.RegisterAll(context.Background(), "test", false)
	if err != nil {
		t.Fatalf("expected successful registration, got: %v", err)
	}
	if created != 3 {
		t.Errorf("expected 3 created, got %d", created)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Verify meaningful activity was touched recently
	lastActivity := process.GetLastMeaningfulActivity()
	if lastActivity.IsZero() {
		t.Errorf("expected non-zero meaningful activity timestamp")
	}
}

func TestRegistry_RegisterSubtrees(t *testing.T) {
	t.Parallel()
	mockStore := newMockDocStorageProvider()
	tmpDir := t.TempDir()

	// Create shipped docs and non-shipped docs
	files := map[string]string{
		"docs/architecture/arch.md":      "# Architecture\nDesign",
		"docs/best-practices/rules.md":   "# Best Practices\nRules",
		"docs/onboarding/guide.md":       "# Onboarding\nGuide",
		"docs/onboarding/archive/old.md": "# Old Guide\nDeprecated",
		"docs/launch/launch.md":          "# Launch Notes\nGTM",
	}

	for relPath, content := range files {
		fullPath := filepath.Join(tmpDir, relPath)
		if err := fileutil.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := fileutil.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	reg := NewRegistry(mockStore, tmpDir)
	created, skipped, err := reg.RegisterSubtrees(context.Background(), "test", ShippedInitDocSubtrees, false)
	if err != nil {
		t.Fatalf("RegisterSubtrees failed: %v", err)
	}

	if created != 3 {
		t.Fatalf("expected 3 created doc_entries, got %d", created)
	}
	if skipped != 0 {
		t.Fatalf("expected 0 skipped, got %d", skipped)
	}

	// Verify only the 3 shipped docs exist in store
	if len(mockStore.objects) != 3 {
		t.Fatalf("expected exactly 3 objects in storage, got %d", len(mockStore.objects))
	}

	registeredPaths := make(map[string]bool)
	for _, obj := range mockStore.objects {
		p := obj[objects.FieldKeyPath].(string)
		registeredPaths[paths.NormalizeDocEntryPathForKey(p)] = true
	}

	for _, expected := range []string{"docs/architecture/arch.md", "docs/best-practices/rules.md", "docs/onboarding/guide.md"} {
		if !registeredPaths[expected] {
			t.Errorf("expected %s to be registered", expected)
		}
	}
	if registeredPaths["docs/onboarding/archive/old.md"] {
		t.Errorf("archive doc should NOT be registered")
	}
	if registeredPaths["docs/launch/launch.md"] {
		t.Errorf("launch doc should NOT be registered")
	}
}

func TestRegisterShippedDocs_FailClosed(t *testing.T) {
	t.Parallel()

	// Empty project root should fail immediately
	_, _, err := RegisterShippedDocs(context.Background(), "", nil)
	if err == nil {
		t.Errorf("expected error on empty projectRoot, got nil")
	}
}
