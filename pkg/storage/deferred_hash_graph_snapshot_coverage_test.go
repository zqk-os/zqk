package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStorageExtended_DeferredHashManager(t *testing.T) {
	ctx := context.Background()
	testRoot, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	dhm := NewDeferredHashManager(fos)
	require.NotNil(t, dhm)

	reg, proc := dhm.GetDeferredHashStats()
	assert.Equal(t, int64(0), reg)
	assert.Equal(t, int64(0), proc)

	// Create a real file on disk for hash calculations
	testFilePath := filepath.Join(testRoot, "test_deferred_obj.json")
	require.NoError(t, fileutil.WriteFile(testFilePath, []byte(`{"id":"BLI-DEF-1","kind":"backlog_item","title":"Deferred Test"}`), paths.FilePerm600))

	// 1. RegisterOperation
	dhm.RegisterOperation("BLI-DEF-1", "backlog_item", testFilePath, "op-1")
	// Duplicate op should be a no-op
	dhm.RegisterOperation("BLI-DEF-1", "backlog_item", testFilePath, "op-1")
	dhm.RegisterOperation("BLI-DEF-1", "backlog_item", testFilePath, "op-2")

	assert.Equal(t, 2, dhm.GetPendingOperations("BLI-DEF-1"))
	assert.False(t, dhm.IsObjectReady("BLI-DEF-1"))
	assert.Equal(t, 0, dhm.GetPendingOperations("NON-EXISTENT"))
	assert.True(t, dhm.IsObjectReady("NON-EXISTENT"))

	// 2. calculateHash
	hash1 := dhm.calculateHash([]byte("hello world"))
	assert.NotEmpty(t, hash1)

	// Fallback calculation with nil/non-file storage
	dhmNonFile := NewDeferredHashManager(nil)
	hash2 := dhmNonFile.calculateHash([]byte("hello world"))
	assert.Equal(t, hash1, hash2)

	// 3. updateHashAndCache edge cases
	err := dhmNonFile.updateHashAndCache(ctx, "OBJ-1", "kind", "")
	assert.Error(t, err) // Non-file storage cannot infer path

	err = dhm.updateHashAndCache(ctx, "NON-EXISTENT", "kind", "/invalid/path/that/does/not/exist.json")
	assert.Error(t, err) // File read fails

	// 4. CompleteOperation
	// Completing op-1 leaves op-2 pending (no update yet)
	err = dhm.CompleteOperation("BLI-DEF-1", "op-1")
	assert.NoError(t, err)
	assert.Equal(t, 1, dhm.GetPendingOperations("BLI-DEF-1"))

	// Completing op-2 triggers updateHashAndCache
	err = dhm.CompleteOperation("BLI-DEF-1", "op-2")
	assert.NoError(t, err)
	assert.Equal(t, 0, dhm.GetPendingOperations("BLI-DEF-1"))

	// Completing unknown op for non-existent object invokes updateHashAndCache with empty path
	_ = dhm.CompleteOperation("NON-EXISTENT-ID", "op-none")

	// 5. processReadyObjects and cleanupOldOperations
	dhm.processReadyObjects(ctx)
	dhm.cleanupOldOperations()

	// 6. Background processor
	bgCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	dhm.StartBackgroundProcessor(bgCtx)

	// 7. Singleton getter
	globalDHM := GetDeferredHashManager(fos)
	assert.NotNil(t, globalDHM)
}

func TestStorageExtended_ChangeJournalReconstruction(t *testing.T) {
	ctx := context.Background()
	_, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	svc := NewChangeJournalReconstructionService(fos)
	require.NotNil(t, svc)

	runs, reversed := svc.GetReconstructionStats()
	assert.Equal(t, int64(0), runs)
	assert.Equal(t, int64(0), reversed)

	// 1. ExtractObjectRef
	kind, id, err := ExtractObjectRef("backlog_item:BLI-123")
	assert.NoError(t, err)
	assert.Equal(t, "backlog_item", kind)
	assert.Equal(t, "BLI-123", id)

	_, _, err = ExtractObjectRef("invalid_ref_without_colon")
	assert.Error(t, err)

	// 2. parseTimestamp
	nowTime := time.Now().UTC()
	assert.Equal(t, nowTime, parseTimestamp(nowTime))
	assert.False(t, parseTimestamp(nowTime.Format(time.RFC3339Nano)).IsZero())
	assert.False(t, parseTimestamp(nowTime.Format(time.RFC3339)).IsZero())
	assert.False(t, parseTimestamp("2026-09-23T01:02:03Z").IsZero())
	assert.False(t, parseTimestamp("2026-09-23T01:02:03+00:00").IsZero())
	assert.True(t, parseTimestamp(nil).IsZero())
	assert.True(t, parseTimestamp(12345).IsZero())
	assert.True(t, parseTimestamp("not-a-timestamp").IsZero())

	// 3. deepCopyObject and deepCopySlice
	assert.Nil(t, deepCopyObject(nil))
	assert.Nil(t, deepCopySlice(nil))

	origObj := map[string]any{
		"title": "Orig",
		"nested": map[string]any{
			"key": "val",
		},
		"list": []any{"item1", map[string]any{"sub": "subval"}},
	}
	copied := deepCopyObject(origObj)
	assert.Equal(t, origObj, copied)

	// 4. getString
	assert.Equal(t, "Orig", getString(origObj, "title"))
	assert.Empty(t, getString(origObj, "missing"))
	assert.Empty(t, getString(nil, "title"))

	// 5. QueryChangeJournalEntries
	entries, err := QueryChangeJournalEntries(ctx, fos, "BLI-999", "backlog_item", nowTime.Add(-1*time.Hour), nowTime)
	assert.NoError(t, err)
	assert.Empty(t, entries)

	// Query with zero time bounds
	entriesZero, err := QueryChangeJournalEntries(ctx, fos, "BLI-999", "backlog_item", time.Time{}, time.Time{})
	assert.NoError(t, err)
	assert.Empty(t, entriesZero)

	// 6. ReconstructStateAtTimestamp when no entries exist
	logger := logging.GetLoggerFromProfile("test")
	currentObj := map[string]any{"id": "BLI-999", "title": "Current"}
	reconstructed, err := svc.ReconstructStateAtTimestamp(ctx, currentObj, "BLI-999", "backlog_item", nowTime.Add(-1*time.Hour), logger)
	assert.NoError(t, err)
	assert.Equal(t, currentObj, reconstructed)

	// Direct call to ReconstructStateAtTimestamp helper
	reconstructed2, err := ReconstructStateAtTimestamp(ctx, fos, currentObj, "BLI-999", "backlog_item", nowTime.Add(-1*time.Hour), logger)
	assert.NoError(t, err)
	assert.Equal(t, currentObj, reconstructed2)

	runsAfter, _ := svc.GetReconstructionStats()
	assert.Equal(t, int64(1), runsAfter)
}

type mockGraphConnWave23 struct {
	provider.GraphConnection
	mu    sync.Mutex
	nodes map[string]*provider.Node
}

func newMockGraphConnWave23() *mockGraphConnWave23 {
	return &mockGraphConnWave23{
		nodes: make(map[string]*provider.Node),
	}
}

func (m *mockGraphConnWave23) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[id]
	if !ok {
		return nil, nil
	}
	cp := *n
	return &cp, nil
}

func (m *mockGraphConnWave23) CreateNode(ctx context.Context, node provider.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := node
	m.nodes[node.ID] = &cp
	return nil
}

func (m *mockGraphConnWave23) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[id]
	if !ok {
		return nil
	}
	for k, v := range updates.Properties {
		n.Properties[k] = v
	}
	return nil
}

func (m *mockGraphConnWave23) DeleteNode(ctx context.Context, id string, labels []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nodes, id)
	return nil
}

func (m *mockGraphConnWave23) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var nodes []*provider.Node
	for _, n := range m.nodes {
		nodes = append(nodes, n)
	}
	return &provider.QueryResult{
		Nodes: nodes,
		Rows:  []map[string]any{},
	}, nil
}

func (m *mockGraphConnWave23) GetPool(ctx context.Context) (provider.ConnectionPool, error) {
	return &mockConnectionPoolWave20{conn: m}, nil
}

func TestStorageExtended_GraphStorage_Delete_and_Snapshot(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	conn := newMockGraphConnWave23()
	tmpDir := t.TempDir()
	graphStore, err := NewGraphObjectStorage(conn, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, graphStore)

	// 1. Delete without CLI authorization
	err = graphStore.Delete(ctx, secCtx, "BLI-TEST-1", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete operations must be performed through CLI")

	// 2. ExportSnapshot when graph is empty
	snapPath := filepath.Join(tmpDir, "snapshot.json.gz")
	err = graphStore.ExportSnapshot(ctx, secCtx, snapPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no objects found to snapshot")

	// 3. Populate a node in the graph for snapshot testing
	node := provider.Node{
		ID:     "BLI-SNAP-1",
		Labels: []string{"backlog_item", "Entity"},
		Properties: map[string]any{
			objects.FieldKeyID:     "BLI-SNAP-1",
			objects.FieldKeyKind:   "backlog_item",
			objects.FieldKeyTitle:  "Snapshot Item",
			objects.FieldKeyStatus: "planned",
		},
	}
	err = conn.CreateNode(ctx, node)
	require.NoError(t, err)

	// 4. ExportSnapshot with node present
	err = graphStore.ExportSnapshot(ctx, secCtx, snapPath)
	assert.NoError(t, err)
	assert.FileExists(t, snapPath)

	// 5. ImportSnapshot
	err = graphStore.ImportSnapshot(ctx, secCtx, snapPath)
	assert.NoError(t, err)

	// 6. findDependents
	deps, err := graphStore.findDependents(ctx, "BLI-SNAP-1", "backlog_item")
	assert.NoError(t, err)
	assert.Empty(t, deps)

	// 7. cascadeDelete
	cliCtx := WithCLIOperation(ctx)
	err = graphStore.cascadeDelete(cliCtx, secCtx, "BLI-SNAP-1", "backlog_item")
	assert.NoError(t, err)
}
