package storage

import (
	"context"
	"errors"
	"maps"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
)

type inMemoryGraphConnWave13 struct {
	provider.GraphConnection
	mu             sync.Mutex
	nodes          map[string]*provider.Node
	getNodeErr     error
	createNodeErr  error
	updateNodeErr  error
}

func newInMemoryGraphConnWave13() *inMemoryGraphConnWave13 {
	return &inMemoryGraphConnWave13{
		nodes: make(map[string]*provider.Node),
	}
}

func (m *inMemoryGraphConnWave13) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getNodeErr != nil {
		return nil, m.getNodeErr
	}
	node, exists := m.nodes[id]
	if !exists {
		return nil, nil
	}
	// Return a copy
	cp := *node
	cp.Properties = make(map[string]any, len(node.Properties))
	maps.Copy(cp.Properties, node.Properties)
	return &cp, nil
}

func (m *inMemoryGraphConnWave13) CreateNode(ctx context.Context, node provider.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createNodeErr != nil {
		return m.createNodeErr
	}
	cp := node
	cp.Properties = make(map[string]any, len(node.Properties))
	maps.Copy(cp.Properties, node.Properties)
	m.nodes[node.ID] = &cp
	return nil
}

func (m *inMemoryGraphConnWave13) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateNodeErr != nil {
		return m.updateNodeErr
	}
	node, exists := m.nodes[id]
	if !exists {
		return errors.New("node not found")
	}
	for k, v := range updates.Properties {
		node.Properties[k] = v
	}
	return nil
}

type mockGraphTransactionWave13 struct {
	provider.GraphTransaction
}

func (t *mockGraphTransactionWave13) CreateNode(ctx context.Context, node provider.Node) error {
	return nil
}
func (t *mockGraphTransactionWave13) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	return nil, nil
}
func (t *mockGraphTransactionWave13) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	return nil
}
func (t *mockGraphTransactionWave13) DeleteNode(ctx context.Context, id string, labels []string) error {
	return nil
}
func (t *mockGraphTransactionWave13) Commit(ctx context.Context) error {
	return nil
}
func (t *mockGraphTransactionWave13) Rollback(ctx context.Context) error {
	return nil
}

func (m *inMemoryGraphConnWave13) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	return &mockGraphTransactionWave13{}, nil
}

func (m *inMemoryGraphConnWave13) CreateEdge(ctx context.Context, edge provider.Edge) error {
	return nil
}

func (m *inMemoryGraphConnWave13) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	return &provider.QueryResult{Rows: []map[string]any{}}, nil
}

func TestStorageExtended_Wave13_HashRegistryGraph(t *testing.T) {
	conn := newInMemoryGraphConnWave13()

	t.Run("NewGraphHashRegistry_and_CRUD_In_Memory", func(t *testing.T) {
		reg := NewGraphHashRegistry("test_kind", conn)
		require.NotNil(t, reg)
		assert.Equal(t, "HashRegistry:test_kind", reg.registryID)

		// Set and Get
		assert.False(t, reg.HasHash("OBJ-1"))
		assert.Equal(t, "", reg.GetHash("OBJ-1"))

		reg.SetHash("OBJ-1", "hash-val-1")
		assert.True(t, reg.HasHash("OBJ-1"))
		assert.Equal(t, "hash-val-1", reg.GetHash("OBJ-1"))

		all := reg.GetAllHashes()
		assert.Len(t, all, 1)
		assert.Equal(t, "hash-val-1", all["OBJ-1"])

		// Delete
		reg.DeleteHash("OBJ-1")
		assert.False(t, reg.HasHash("OBJ-1"))
	})

	t.Run("Save_and_Load_Roundtrip", func(t *testing.T) {
		reg1 := NewGraphHashRegistry("roundtrip_kind", conn)
		reg1.SetHash("ID-100", "hash-100")
		reg1.SetHash("ID-200", "hash-200")

		// First save (creates node)
		err := reg1.Save()
		require.NoError(t, err)

		// Second save (updates existing node)
		reg1.SetHash("ID-300", "hash-300")
		err = reg1.Save()
		require.NoError(t, err)

		// Load into fresh registry
		reg2 := NewGraphHashRegistry("roundtrip_kind", conn)
		err = reg2.Load()
		require.NoError(t, err)

		assert.Equal(t, "hash-100", reg2.GetHash("ID-100"))
		assert.Equal(t, "hash-200", reg2.GetHash("ID-200"))
		assert.Equal(t, "hash-300", reg2.GetHash("ID-300"))
		assert.Len(t, reg2.GetAllHashes(), 3)
	})

	t.Run("Load_Branches", func(t *testing.T) {
		// Node not found -> empty registry
		regEmpty := NewGraphHashRegistry("nonexistent_kind", conn)
		err := regEmpty.Load()
		require.NoError(t, err)
		assert.Len(t, regEmpty.GetAllHashes(), 0)

		// GetNode error
		connErr := newInMemoryGraphConnWave13()
		connErr.getNodeErr = errors.New("db query failed")
		regErr := NewGraphHashRegistry("error_kind", connErr)
		err = regErr.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get registry node")

		// Node exists but no hashes property
		connNoProp := newInMemoryGraphConnWave13()
		connNoProp.nodes["HashRegistry:no_prop_kind"] = &provider.Node{
			ID:         "HashRegistry:no_prop_kind",
			Properties: map[string]any{},
		}
		regNoProp := NewGraphHashRegistry("no_prop_kind", connNoProp)
		err = regNoProp.Load()
		require.NoError(t, err)
		assert.Len(t, regNoProp.GetAllHashes(), 0)

		// Node exists with invalid JSON in hashes
		connBadJSON := newInMemoryGraphConnWave13()
		connBadJSON.nodes["HashRegistry:bad_json_kind"] = &provider.Node{
			ID: "HashRegistry:bad_json_kind",
			Properties: map[string]any{
				"hashes": "{invalid-json",
			},
		}
		regBadJSON := NewGraphHashRegistry("bad_json_kind", connBadJSON)
		err = regBadJSON.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse registry hashes")
	})

	t.Run("Save_Branches", func(t *testing.T) {
		// GetNode error on check
		connErr := newInMemoryGraphConnWave13()
		connErr.getNodeErr = errors.New("check node failed")
		regErr := NewGraphHashRegistry("check_err_kind", connErr)
		err := regErr.Save()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check registry node")

		// CreateNode error
		connCreateErr := newInMemoryGraphConnWave13()
		connCreateErr.createNodeErr = errors.New("cannot create node")
		regCreateErr := NewGraphHashRegistry("create_err_kind", connCreateErr)
		err = regCreateErr.Save()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot create node")

		// UpdateNode error
		connUpdateErr := newInMemoryGraphConnWave13()
		connUpdateErr.nodes["HashRegistry:update_err_kind"] = &provider.Node{
			ID:         "HashRegistry:update_err_kind",
			Properties: map[string]any{},
		}
		connUpdateErr.updateNodeErr = errors.New("cannot update node")
		regUpdateErr := NewGraphHashRegistry("update_err_kind", connUpdateErr)
		err = regUpdateErr.Save()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot update node")
	})
}

type mockQueueShutdownHandlerWave13 struct {
	name         string
	pendingCount int64
	critical     bool
	drained      bool
	initShutdown bool
	initError    error
	drainError   error
}

func (m *mockQueueShutdownHandlerWave13) InitiateShutdown() error {
	m.initShutdown = true
	return m.initError
}

func (m *mockQueueShutdownHandlerWave13) Drain(ctx context.Context) error {
	m.drained = true
	return m.drainError
}

func (m *mockQueueShutdownHandlerWave13) IsDrained() bool {
	return m.drained
}

func (m *mockQueueShutdownHandlerWave13) GetPendingCount() int64 {
	return m.pendingCount
}

func (m *mockQueueShutdownHandlerWave13) GetName() string {
	return m.name
}

func (m *mockQueueShutdownHandlerWave13) IsCritical() bool {
	return m.critical
}

func TestStorageExtended_Wave13_CASFacadeExports(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("FileObjectStorage_CASUsesContentAddressableStorage_and_GenerateID", func(t *testing.T) {
		_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

		// CASUsesContentAddressableStorage (all kinds use CAS in this architecture)
		usesCAS := fileStorage.CASUsesContentAddressableStorage(objects.KindAuditAggregationMetric)
		assert.True(t, usesCAS)

		usesBacklog := fileStorage.CASUsesContentAddressableStorage("backlog_item")
		assert.True(t, usesBacklog)

		// CASGenerateID
		id, err := fileStorage.CASGenerateID(ctx, "backlog_item")
		require.NoError(t, err)
		assert.NotEmpty(t, id)
	})

	t.Run("QueueShutdownWrapper", func(t *testing.T) {
		handler := &mockQueueShutdownHandlerWave13{
			name:         "test_queue_handler",
			pendingCount: 5,
			critical:     true,
			drained:      false,
		}
		wrapper := &queueShutdownWrapper{h: handler}

		assert.Equal(t, "test_queue_handler", wrapper.GetName())
		assert.Equal(t, int64(5), wrapper.GetPendingCount())
		assert.True(t, wrapper.IsCritical())
		assert.False(t, wrapper.IsDrained())

		err := wrapper.InitiateShutdown()
		require.NoError(t, err)
		assert.True(t, handler.initShutdown)

		err = wrapper.Drain(ctx)
		require.NoError(t, err)
		assert.True(t, wrapper.IsDrained())
	})

	t.Run("ShutdownCoordinatorWrapper_and_Global", func(t *testing.T) {
		coord := caspkg.GlobalShutdownCoordinator
		require.NotNil(t, coord)

		isInit := coord.IsShutdownInitiated()
		assert.False(t, isInit)

		handler := &mockQueueShutdownHandlerWave13{name: "coord_test_queue"}
		coord.RegisterQueue(handler)
	})

	t.Run("ValidationRegistryWrapper_and_StrategyWrapper", func(t *testing.T) {
		reg := caspkg.GlobalValidationRegistry
		require.NotNil(t, reg)

		strategy := reg.GetStrategy(objects.KindAuditAggregationMetric)
		require.NotNil(t, strategy)

		valid, invalid, count := strategy.ValidateMappings("/tmp", map[string]string{"k": "v"}, map[string]string{"k": "b"})
		assert.NotNil(t, valid)
		assert.NotNil(t, invalid)
		_ = count
	})

	t.Run("CASAuditCreator_Callback", func(t *testing.T) {
		auditCreator := caspkg.GetAuditCreator()
		require.NotNil(t, auditCreator)

		tempDir, err := os.MkdirTemp("", "cas_audit_test_*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		fileStorage, err := NewFileObjectStorage(tempDir)
		require.NoError(t, err)

		// 1. Success path (failureCount == 0)
		auditCreator(ctx, secCtx, tempDir, fileStorage, 10, 10, 0, 50, nil)

		// 2. Failure path (failureCount > 0, failedFiles)
		auditCreator(ctx, secCtx, tempDir, fileStorage, 10, 8, 2, 75, []string{"file1.json", "file2.json"})

		// 3. Storage is not an ObjectStorageProvider
		auditCreator(ctx, secCtx, tempDir, nil, 10, 10, 0, 50, nil)
	})
}
