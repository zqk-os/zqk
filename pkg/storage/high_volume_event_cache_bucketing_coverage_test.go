package storage_test

import (
	"context"
	"encoding/json"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// mockMeshTransport implements federation.Transport for mesh testing
type mockMeshTransport struct {
	payload json.RawMessage
	err     error
}

func (m *mockMeshTransport) SendHandshake(ctx context.Context, endpoint string, req federation.HandshakeRequest) (*federation.HandshakeResponse, error) {
	return &federation.HandshakeResponse{Accepted: true}, nil
}

func (m *mockMeshTransport) SendHeartbeat(ctx context.Context, endpoint string, kernelID string) error {
	return nil
}

func (m *mockMeshTransport) ExecuteTool(ctx context.Context, endpoint, tool string, args map[string]any) (json.RawMessage, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.payload != nil {
		return m.payload, nil
	}
	return json.RawMessage(`{"objects":[]}`), nil
}

// TestStorageExtended_Wave4_HighVolumeEventCache tests high_volume_event_cache_build.go
func TestStorageExtended_HighVolumeEventCache(t *testing.T) {
	tmpDir := t.TempDir()
	fos, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileObjectStorage: %v", err)
	}
	if cleanup := fos.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = fos.Shutdown(ctx) }()

	// Test BuildCache on HighVolumeEventCache
	cache := storagepkg.NewHighVolumeEventCache()
	err = cache.BuildCache(ctx, tmpDir, fos)
	if err != nil {
		t.Logf("BuildCache returned: %v", err)
	}

	// Test with canceled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = cache.BuildCache(cancCtx, tmpDir, fos)
	if err == nil {
		t.Errorf("expected error building cache with canceled context")
	}
}

// TestStorageExtended_BucketingStrategyStorage_Partitioning tests bucketing_strategy_storage.go
func TestStorageExtended_BucketingStrategyStorage_Partitioning(t *testing.T) {
	tmpDir := t.TempDir()
	fos, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileObjectStorage: %v", err)
	}
	if cleanup := fos.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = fos.Shutdown(ctx) }()

	// 1. FileBucketStrategyStorage
	fbss := storagepkg.NewFileBucketStrategyStorage(tmpDir)
	if fbss.GetBackendType() != "file" {
		t.Errorf("expected backend type 'file', got '%s'", fbss.GetBackendType())
	}

	// Load non-existent strategy
	_, err = fbss.LoadStrategy(ctx, "strategy-nonexistent")
	if err == nil {
		t.Errorf("expected error loading nonexistent strategy")
	}

	// Load all strategies
	strategies, err := fbss.LoadAllStrategies(ctx)
	if err != nil {
		t.Logf("LoadAllStrategies returned: %v", err)
	}
	_ = strategies

	// Load strategies for kind
	matched, err := fbss.LoadStrategiesForKind(ctx, objects.KindBacklogItem)
	if err != nil {
		t.Logf("LoadStrategiesForKind returned: %v", err)
	}
	_ = matched

	// Save strategy with missing ID
	badStrategy := map[string]any{
		"kind": objects.KindBucketingStrategy,
	}
	err = fbss.SaveStrategy(ctx, badStrategy)
	if err == nil {
		t.Errorf("expected error saving strategy with no ID")
	}

	// 2. ExistingStorageBucketStrategyProvider
	provider := storagepkg.NewExistingStorageBucketStrategyProvider(fos)
	if provider.GetBackendType() != "file" {
		t.Errorf("expected backend 'file', got '%s'", provider.GetBackendType())
	}

	_, _ = provider.LoadStrategy(ctx, "strategy-xyz")
	_, _ = provider.LoadAllStrategies(ctx)
	_, _ = provider.LoadStrategiesForKind(ctx, "backlog_item")

	// Save valid strategy
	stratObj := map[string]any{
		objects.FieldKeyID:        "strat-001",
		objects.FieldKeyKind:      objects.KindBucketingStrategy,
		objects.FieldKeyAppliesTo: []any{"backlog_item"},
		objects.FieldKeyEnabled:   true,
	}
	_ = provider.SaveStrategy(ctx, stratObj)
	_ = provider.DeleteStrategy(ctx, "strat-001")

	// 3. BucketStrategyStorageFactory
	factory, err := storagepkg.NewBucketStrategyStorageFactory(ctx, tmpDir)
	if err != nil {
		t.Logf("NewBucketStrategyStorageFactory returned: %v", err)
	} else {
		_ = factory.GetStorageProvider()
		_ = factory.GetBackendType()
	}
}

// TestStorageExtended_Wave4_MeshObjectStorage tests object_storage_mesh.go
func TestStorageExtended_MeshObjectStorage(t *testing.T) {
	tmpDir := t.TempDir()
	fos, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileObjectStorage: %v", err)
	}
	if cleanup := fos.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = fos.Shutdown(ctx) }()

	secCtx := &pkgctx.SecurityContext{
		AccountID:   "admin",
		Roles:       []string{"admin"},
		Permissions: []string{"read:*", "write:*"},
	}

	transport := &mockMeshTransport{
		payload: []byte(`{"objects":[{"id":"REMOTE-1","kind":"criteria","title":"Remote Criteria"}]}`),
	}

	mesh := storagepkg.NewMeshObjectStorage(fos, transport)
	if mesh.GetLocal() != fos {
		t.Errorf("expected GetLocal to return fos")
	}

	// Test methods delegating to local
	_ = mesh.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "PRI-099",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Priority 99",
		objects.FieldKeyStatus: "originated",
	})

	_, _ = mesh.Read(ctx, secCtx, "PRI-099")
	_, _ = mesh.Exists(ctx, secCtx, "PRI-099")
	_ = mesh.Update(ctx, secCtx, "PRI-099", map[string]any{"title": "Updated Priority 99"})

	// Count and List
	_, _ = mesh.Count(ctx, secCtx, storagepkg.ListFilter{Kind: objects.KindPriorityPlan})
	_, _ = mesh.List(ctx, secCtx, nil, storagepkg.ListFilter{Kind: objects.KindPriorityPlan})

	// Listing remote kernels (recurse guard)
	_, _ = mesh.List(ctx, secCtx, nil, storagepkg.ListFilter{Kind: objects.KindRemoteKernel})

	// Other delegated interface methods
	_, _ = mesh.Query(ctx, secCtx, nil, storagepkg.Query{})
	_, _ = mesh.Search(ctx, secCtx, nil, storagepkg.SearchQuery{})
	_, _ = mesh.BeginTransaction(ctx)
	_, _ = mesh.BulkCreate(ctx, secCtx, []map[string]any{})
	_, _ = mesh.BulkUpdate(ctx, secCtx, []storagepkg.BulkUpdateItem{})
	_, _ = mesh.BulkGet(ctx, secCtx, []string{"PRI-099"})
	_, _ = mesh.BulkDelete(ctx, secCtx, []string{"PRI-099"}, false)
	_, _ = mesh.Aggregate(ctx, secCtx, nil, storagepkg.ListFilter{Kind: objects.KindPriorityPlan}, nil)
	_, _ = mesh.GetRelated(ctx, secCtx, "PRI-099", "parent", 1)
	_, _ = mesh.GetPath(ctx, secCtx, "PRI-099", "BLI-001")
	_, _ = mesh.GetNeighbors(ctx, secCtx, "PRI-099", "outgoing")
	_ = mesh.Move(ctx, secCtx, "PRI-099", "workstream", false)
	_ = mesh.Rename(ctx, secCtx, "PRI-099", "PRI-100", false)
	_ = mesh.Delete(ctx, secCtx, "PRI-099", false)
	_ = mesh.Shutdown(ctx)
}
