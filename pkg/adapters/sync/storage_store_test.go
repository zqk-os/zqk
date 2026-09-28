package sync

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockStorageProvider struct {
	storage.ObjectStorageProvider
	items map[string]map[string]any
}

func newMockStorageProvider() *mockStorageProvider {
	return &mockStorageProvider{
		items: make(map[string]map[string]any),
	}
}

func (m *mockStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	item, ok := m.items[id]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	return item, nil
}

func (m *mockStorageProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj["id"].(string)
	m.items[id] = obj
	return nil
}

func (m *mockStorageProvider) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	item, ok := m.items[id]
	if !ok {
		return storage.ErrObjectNotFound
	}
	for k, v := range updates {
		item[k] = v
	}
	return nil
}

func (m *mockStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var objects []map[string]any
	for _, obj := range m.items {
		if filter.Kind != "" && obj["kind"] != filter.Kind {
			continue
		}
		objects = append(objects, obj)
	}
	return &storage.QueryResult{Objects: objects}, nil
}

func TestStorageKernelStore_CRUD(t *testing.T) {
	sp := newMockStorageProvider()
	store := NewKernelSyncStore(sp, nil)
	ctx := context.Background()

	item := &BacklogItemSyncData{
		ID:               "BLI-GH-42",
		Title:            "Fix memory leak",
		Description:      "Identified memory leak in worker",
		ProblemStatement: "Workers leak goroutines",
		Status:           StatusPlanned,
		Priority:         PriorityHigh,
		PriorityTier:     TierP1,
		ExternalSource:   SourceGitHub,
		ExternalID:       "42",
		UpdatedAt:        time.Now(),
	}

	// 1. Upsert (Create)
	if err := store.UpsertBacklogItem(ctx, item); err != nil {
		t.Fatalf("UpsertBacklogItem create failed: %v", err)
	}

	// 2. Get by External ID
	found, err := store.GetBacklogItemByExternalID(ctx, SourceGitHub, "42")
	if err != nil {
		t.Fatalf("GetBacklogItemByExternalID failed: %v", err)
	}
	if found == nil {
		t.Fatalf("expected item to be found, got nil")
	}
	if found.Title != "Fix memory leak" {
		t.Errorf("expected title 'Fix memory leak', got %s", found.Title)
	}

	// 3. Upsert (Update)
	item.Title = "Fix memory leak in worker pool"
	if err := store.UpsertBacklogItem(ctx, item); err != nil {
		t.Fatalf("UpsertBacklogItem update failed: %v", err)
	}

	foundUpdated, err := store.GetBacklogItemByExternalID(ctx, SourceGitHub, "42")
	if err != nil || foundUpdated == nil {
		t.Fatalf("failed to retrieve updated item: %v", err)
	}
	if foundUpdated.Title != "Fix memory leak in worker pool" {
		t.Errorf("expected updated title, got %s", foundUpdated.Title)
	}

	// 4. List
	items, err := store.ListBacklogItems(ctx)
	if err != nil {
		t.Fatalf("ListBacklogItems failed: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}
