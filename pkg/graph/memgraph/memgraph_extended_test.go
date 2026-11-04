package memgraph

import (
	"context"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/provider"
)

// TestMemGraph_SafeCypher_Coverage exercises all branching logic in safeCypher.
func TestMemGraph_SafeCypher_Coverage(t *testing.T) {
	// 1. Empty string
	res1 := safeCypher("MATCH (n%s) RETURN n", "")
	assert.Equal(t, "MATCH (n) RETURN n", res1)

	// 2. String with space
	res2 := safeCypher("MATCH (n) WHERE %s RETURN n", "n.prop = 'hello world'")
	assert.Equal(t, "MATCH (n) WHERE n.prop = 'hello world' RETURN n", res2)

	// 3. String with special characters and colon separated labels
	res3 := safeCypher("CREATE (n:%s) RETURN n", ":LabelA:LabelB")
	assert.Equal(t, "CREATE (n::LabelA:LabelB) RETURN n", res3)

	// 4. String with brackets/braces
	res4 := safeCypher("MATCH (n%s) RETURN n", "{id: 123}")
	assert.Equal(t, "MATCH (n{id: 123}) RETURN n", res4)

	// 5. Pure identifier without special characters
	res5 := safeCypher("MATCH (n:%s) RETURN n", "Goal")
	assert.Equal(t, "MATCH (n:Goal) RETURN n", res5)

	// 6. Non-string arguments (integers, bools)
	res6 := safeCypher("MATCH (n) RETURN n SKIP %d LIMIT %d", 10, 5)
	assert.Equal(t, "MATCH (n) RETURN n SKIP 10 LIMIT 5", res6)
}

// TestMemGraph_BoltClientAndHelpers_Coverage exercises bolt client creation, connectivity, and record parsing.
func TestMemGraph_BoltClientAndHelpers_Coverage(t *testing.T) {
	ctx := context.Background()

	// 1. newBoltClient with Port == 0
	cfgDefaultPort := &MemGraphConfig{
		Host:     "localhost",
		Port:     0,
		Username: "memgraph",
		Password: "password",
	}
	client0, err0 := newBoltClient(cfgDefaultPort)
	require.NoError(t, err0)
	require.NotNil(t, client0)

	// verifyConnectivity with timed out context
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	_ = client0.verifyConnectivity(timeoutCtx)

	// close driver
	_ = client0.close(ctx)

	// 2. newBoltClient with Port > 0
	cfgExplicitPort := &MemGraphConfig{
		Host:     "127.0.0.1",
		Port:     7687,
		Username: "memgraph",
		Password: "password",
	}
	client1, err1 := newBoltClient(cfgExplicitPort)
	require.NoError(t, err1)
	require.NotNil(t, client1)
	_ = client1.close(ctx)

	// 3. parseSingleNode edge cases
	// Empty records -> nil, nil
	nodeEmpty, errNodeEmpty := parseSingleNode([]*neo4j.Record{})
	assert.NoError(t, errNodeEmpty)
	assert.Nil(t, nodeEmpty)

	// Non-Node value format in record
	badRecord := &neo4j.Record{
		Values: []any{"not-a-node"},
	}
	_, errBadNode := parseSingleNode([]*neo4j.Record{badRecord})
	assert.Error(t, errBadNode)
	assert.Contains(t, errBadNode.Error(), "unexpected node format in response")

	// Valid Node without id in props (falls back to ElementId)
	validNodeNoID := neo4j.Node{
		ElementId: "elem-12345",
		Labels:    []string{"TestNode"},
		Props:     map[string]any{"name": "test"},
	}
	goodNodeRecord := &neo4j.Record{Values: []any{validNodeNoID}}
	parsedNode, errParsedNode := parseSingleNode([]*neo4j.Record{goodNodeRecord})
	require.NoError(t, errParsedNode)
	assert.Equal(t, "elem-12345", parsedNode.ID)
	assert.Equal(t, "test", parsedNode.Properties["name"])

	// 4. parseSingleEdge edge cases
	// Empty records -> nil, nil
	edgeEmpty, errEdgeEmpty := parseSingleEdge([]*neo4j.Record{}, "from", "to")
	assert.NoError(t, errEdgeEmpty)
	assert.Nil(t, edgeEmpty)

	// Non-Relationship record
	_, errBadEdge := parseSingleEdge([]*neo4j.Record{badRecord}, "from", "to")
	assert.Error(t, errBadEdge)
	assert.Contains(t, errBadEdge.Error(), "unexpected edge format in response")

	// Valid Relationship with override from/to values in Record
	rel := neo4j.Relationship{
		ElementId: "rel-999",
		Type:      "DEPENDS_ON",
		Props:     map[string]any{"weight": 1.5},
	}
	edgeRecord3Values := &neo4j.Record{
		Values: []any{rel, "override-from", "override-to"},
	}
	parsedEdge, errParsedEdge := parseSingleEdge([]*neo4j.Record{edgeRecord3Values}, "from", "to")
	require.NoError(t, errParsedEdge)
	assert.Equal(t, "override-from", parsedEdge.FromID)
	assert.Equal(t, "override-to", parsedEdge.ToID)
	assert.Equal(t, "DEPENDS_ON", parsedEdge.Type)
}

// TestMemGraph_Transaction_LifecycleAndGuards tests transaction state checks, error returns,
// and method delegation when explicitTx is nil or transaction is already finished.
func TestMemGraph_Transaction_LifecycleAndGuards(t *testing.T) {
	ctx := context.Background()
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
	}

	// 1. BeginTransaction when boltClient is nil
	_, errNoBolt := conn.BeginTransaction(ctx)
	require.Error(t, errNoBolt)
	assert.Contains(t, errNoBolt.Error(), "bolt client not initialized")

	// 2. BeginTransaction when already has open transaction
	txOpen := &memgraphTransaction{conn: conn}
	conn.SetOpenTransaction(txOpen)
	conn.boltClient = &boltClient{}
	_, errAlreadyOpen := conn.BeginTransaction(ctx)
	require.Error(t, errAlreadyOpen)
	assert.Contains(t, errAlreadyOpen.Error(), "connection already has an open transaction")
	conn.ClearTransaction()

	// 3. BeginNestedTransaction guards
	conn.boltClient = nil
	_, errNestedNoBolt := conn.BeginNestedTransaction(ctx, nil)
	require.Error(t, errNestedNoBolt)
	assert.Contains(t, errNestedNoBolt.Error(), "bolt client not initialized")

	conn.boltClient = &boltClient{}
	// Parent not a *memgraphTransaction
	_, errInvalidParent := conn.BeginNestedTransaction(ctx, &provider.MockTransaction{})
	require.Error(t, errInvalidParent)
	assert.Contains(t, errInvalidParent.Error(), "invalid parent transaction type")

	// Valid parent nesting simulation
	parentTx := &memgraphTransaction{conn: conn}
	nestedTx, errNested := conn.BeginNestedTransaction(ctx, parentTx)
	require.NoError(t, errNested)
	require.NotNil(t, nestedTx)
	assert.Equal(t, parentTx, nestedTx.(*memgraphTransaction).GetParent())

	// 4. inspectState when conn is nil
	orphanTx := &memgraphTransaction{conn: nil}
	_, _, _, errNilConn := orphanTx.inspectState("lock")
	require.Error(t, errNilConn)
	assert.Contains(t, errNilConn.Error(), "transaction connection is nil")

	// 5. Commit state guards
	// explicitTx is nil
	txUninit := &memgraphTransaction{conn: conn}
	errCommitUninit := txUninit.Commit(ctx)
	require.Error(t, errCommitUninit)
	assert.Contains(t, errCommitUninit.Error(), "transaction not initialized")

	// Already committed
	txCommitted := &memgraphTransaction{conn: conn, committed: true}
	errCommitAgain := txCommitted.Commit(ctx)
	require.Error(t, errCommitAgain)
	assert.Contains(t, errCommitAgain.Error(), "transaction already committed")

	// Already rolled back
	txRolledBack := &memgraphTransaction{conn: conn, rolledBack: true}
	errCommitRolledBack := txRolledBack.Commit(ctx)
	require.Error(t, errCommitRolledBack)
	assert.Contains(t, errCommitRolledBack.Error(), "transaction already rolled back")

	// 6. Rollback state guards
	errRollbackCommitted := txCommitted.Rollback(ctx)
	require.Error(t, errRollbackCommitted)
	assert.Contains(t, errRollbackCommitted.Error(), "transaction already committed")

	// Already rolled back -> returns nil
	errRollbackAgain := txRolledBack.Rollback(ctx)
	assert.NoError(t, errRollbackAgain)

	// explicitTx == nil -> returns nil
	assert.NoError(t, txUninit.Rollback(ctx))

	// 7. Nested transaction methods on memgraphTransaction
	assert.False(t, txUninit.SupportsNestedTransactions())
	_, errTxNested := txUninit.BeginNestedTransaction(ctx)
	require.Error(t, errTxNested)
	assert.Contains(t, errTxNested.Error(), "nested transactions not supported by MemGraph Bolt protocol")
	assert.Nil(t, txUninit.GetParent())

	// 8. CRUD and Query guards when explicitTx == nil
	dummyNode := provider.Node{ID: "n1", Labels: []string{"Goal"}}
	assert.EqualError(t, txUninit.CreateNode(ctx, dummyNode), "transaction not initialized")

	_, errGetNode := txUninit.GetNode(ctx, "n1", nil)
	assert.EqualError(t, errGetNode, "transaction not initialized")

	assert.EqualError(t, txUninit.UpdateNode(ctx, "n1", provider.NodeUpdates{}), "transaction not initialized")
	assert.EqualError(t, txUninit.DeleteNode(ctx, "n1", nil), "transaction not initialized")

	dummyEdge := provider.Edge{FromID: "n1", ToID: "n2", Type: "RELATES_TO"}
	assert.EqualError(t, txUninit.CreateEdge(ctx, dummyEdge), "transaction not initialized")

	_, errGetEdge := txUninit.GetEdge(ctx, "n1", "n2", "RELATES_TO")
	assert.EqualError(t, errGetEdge, "transaction not initialized")

	assert.EqualError(t, txUninit.UpdateEdge(ctx, "n1", "n2", "RELATES_TO", provider.EdgeUpdates{}), "transaction not initialized")
	assert.EqualError(t, txUninit.DeleteEdge(ctx, "n1", "n2", "RELATES_TO"), "transaction not initialized")

	_, errExecQuery := txUninit.ExecuteQuery(ctx, provider.Query{Query: "MATCH (n) RETURN n"})
	assert.EqualError(t, errExecQuery, "transaction not initialized")
}

// TestMemGraph_Connection_UninitializedAndBatchGuards tests fail-closed behavior on connection methods
// when boltClient is uninitialized, as well as batch execution boundaries.
func TestMemGraph_Connection_UninitializedAndBatchGuards(t *testing.T) {
	ctx := context.Background()
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     nil, // explicitly nil
	}

	dummyNode := provider.Node{ID: "node-1", Labels: []string{"Test"}}
	dummyEdge := provider.Edge{FromID: "node-1", ToID: "node-2", Type: "LINK"}

	// Verify fail-closed checks on uninitialized bolt client
	assert.Error(t, conn.CreateNode(ctx, dummyNode))
	_, errGetNode := conn.GetNode(ctx, "node-1", nil)
	assert.Error(t, errGetNode)
	assert.Error(t, conn.UpdateNode(ctx, "node-1", provider.NodeUpdates{}))
	assert.Error(t, conn.DeleteNode(ctx, "node-1", nil))
	_, errListNodes := conn.ListNodes(ctx, provider.NodeFilter{})
	assert.Error(t, errListNodes)

	assert.Error(t, conn.CreateEdge(ctx, dummyEdge))
	_, errGetEdge := conn.GetEdge(ctx, "node-1", "node-2", "LINK")
	assert.Error(t, errGetEdge)
	assert.Error(t, conn.UpdateEdge(ctx, "node-1", "node-2", "LINK", provider.EdgeUpdates{}))
	assert.Error(t, conn.DeleteEdge(ctx, "node-1", "node-2", "LINK"))
	_, errListEdges := conn.ListEdges(ctx, provider.EdgeFilter{})
	assert.Error(t, errListEdges)

	_, errQuery := conn.ExecuteQuery(ctx, provider.Query{Query: "RETURN 1"})
	assert.Error(t, errQuery)
	_, errVector := conn.ExecuteVectorQuery(ctx, provider.VectorQuery{})
	assert.Error(t, errVector)
	_, errTraversal := conn.ExecuteTraversal(ctx, provider.TraversalQuery{})
	assert.Error(t, errTraversal)
	assert.Error(t, conn.HealthCheck(ctx))

	// Batch execution when boltClient == nil
	_, errBatchNil := conn.ExecuteBatch(ctx, []provider.Operation{{Type: "create_node"}})
	assert.Error(t, errBatchNil)
	assert.Contains(t, errBatchNil.Error(), "bolt client not initialized")

	// Batch execution with empty operations slice returns empty BatchResult immediately
	conn.boltClient = &boltClient{}
	batchRes, errEmptyBatch := conn.ExecuteBatch(ctx, []provider.Operation{})
	require.NoError(t, errEmptyBatch)
	assert.Equal(t, 0, batchRes.SuccessCount)
	assert.Equal(t, 0, batchRes.FailureCount)
	assert.Empty(t, batchRes.Errors)

	// CloseInternal and Reset
	assert.NoError(t, conn.CloseInternal())
	conn.Reset()

	// Reset with open transaction rolls it back
	tx := &memgraphTransaction{conn: conn}
	conn.SetOpenTransaction(tx)
	conn.Reset()
	assert.False(t, conn.HasOpenTransaction())
}

// TestMemGraph_Pool_LifecycleAndWaiting tests connection pool concurrency, capacity waiting,
// return channels, and full-pool overflow handling using managed goroutines.
func TestMemGraph_Pool_LifecycleAndWaiting(t *testing.T) {
	ctx := context.Background()
	cfg := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		Username: "memgraph",
		Password: "password",
		PoolSize: 2,
	}

	pool, err := NewMemGraphConnectionPool(&cfg)
	require.NoError(t, err)
	require.NotNil(t, pool)

	// 1. Stats inspection
	stats := pool.Stats()
	assert.Equal(t, 2, stats.MaxSize)
	assert.Equal(t, 0, stats.Active)

	// 2. ReturnConnection invalid type guard
	errBadType := pool.ReturnConnection(&provider.MockConnection{})
	require.Error(t, errBadType)
	assert.Contains(t, errBadType.Error(), "invalid connection type")

	// 3. ReturnConnection with open transaction (rolls it back)
	dummyConn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
	}
	tx := &memgraphTransaction{conn: dummyConn}
	dummyConn.SetOpenTransaction(tx)
	errReturn := pool.ReturnConnection(dummyConn)
	assert.NoError(t, errReturn)
	assert.False(t, dummyConn.HasOpenTransaction())

	// 4. ReturnConnection when channel is full (overflow safety check)
	dummyConn2 := &memgraphConnection{BaseConnection: &provider.BaseConnection{}}
	dummyConn3 := &memgraphConnection{BaseConnection: &provider.BaseConnection{}}
	_ = pool.ReturnConnection(dummyConn2) // channel has 2 items now (full)
	// Third return triggers default overflow branch and calls CloseInternal
	errOverflow := pool.ReturnConnection(dummyConn3)
	assert.NoError(t, errOverflow)

	// 5. GetConnection waiting and timeout
	// Pool currently has 2 connections in channel
	c1, errC1 := pool.GetConnection(ctx)
	require.NoError(t, errC1)
	c2, errC2 := pool.GetConnection(ctx)
	require.NoError(t, errC2)

	// Active is 2 (at maxSize). Requesting a 3rd connection will block.
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, errTimeout := pool.GetConnection(timeoutCtx)
	require.Error(t, errTimeout)
	assert.Equal(t, context.DeadlineExceeded, errTimeout)

	// 6. GetConnection waiting succeeds when connection is returned concurrently
	// POL-CONCURRENCY: Spawn worker via goroutinelabels.NewGoroutine().StartSimple()
	waitSuccessCtx, cancelSuccess := context.WithTimeout(ctx, 2*time.Second)
	defer cancelSuccess()

	goroutinelabels.NewGoroutine("test_memgraph_pool_return_worker", "returns connection after brief sleep").
		StartSimple(func() {
			time.Sleep(20 * time.Millisecond)
			_ = pool.ReturnConnection(c1)
		})

	cAcquired, errAcquired := pool.GetConnection(waitSuccessCtx)
	require.NoError(t, errAcquired)
	require.NotNil(t, cAcquired)

	// Clean up
	_ = pool.ReturnConnection(c2)
	_ = pool.ReturnConnection(cAcquired)
	require.NoError(t, pool.Close())
}

// TestMemGraph_Client_PingCloseAndCoverage tests the top-level Client type.
func TestMemGraph_Client_PingCloseAndCoverage(t *testing.T) {
	ctx := context.Background()

	// 1. Client with nil driver
	nilClient := &Client{driver: nil}
	assert.NoError(t, nilClient.Close(ctx))
	errPingNil := nilClient.Ping(ctx)
	require.Error(t, errPingNil)
	assert.Contains(t, errPingNil.Error(), "driver is not initialized")

	// 2. NewClient instantiation
	client, err := NewClient("bolt://localhost:7687", "user", "pass")
	require.NoError(t, err)
	require.NotNil(t, client)

	// Ping with short timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	_ = client.Ping(timeoutCtx)

	assert.NoError(t, client.Close(ctx))
}

type mockManagedTxForTest struct {
	neo4j.ManagedTransaction
	runErr  error
	records []*neo4j.Record
}

func (m *mockManagedTxForTest) Run(ctx context.Context, cypher string, params map[string]any) (neo4j.ResultWithContext, error) {
	if m.runErr != nil {
		return nil, m.runErr
	}
	return &mockResultWithContext{records: m.records}, nil
}

// TestMemGraph_Connection_InTransactionDelegation verifies connection methods delegate to open transaction.
func TestMemGraph_Connection_InTransactionDelegation(t *testing.T) {
	ctx := context.Background()
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	tx := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockBoltTx{},
	}
	conn.SetOpenTransaction(tx)

	// CreateNode delegated
	errCreate := conn.CreateNode(ctx, provider.Node{ID: "delegated-1", Labels: []string{"Item"}})
	assert.NoError(t, errCreate)

	// UpdateNode delegated
	errUpdate := conn.UpdateNode(ctx, "delegated-1", provider.NodeUpdates{Properties: map[string]any{"foo": "bar"}})
	assert.NoError(t, errUpdate)

	// DeleteNode delegated
	errDelete := conn.DeleteNode(ctx, "delegated-1", []string{"Item"})
	assert.NoError(t, errDelete)

	// CreateEdge delegated
	errCreateEdge := conn.CreateEdge(ctx, provider.Edge{FromID: "d1", ToID: "d2", Type: "REL"})
	assert.NoError(t, errCreateEdge)

	// UpdateEdge delegated
	errUpdateEdge := conn.UpdateEdge(ctx, "d1", "d2", "REL", provider.EdgeUpdates{Properties: map[string]any{"w": 2}})
	assert.NoError(t, errUpdateEdge)

	// DeleteEdge delegated
	errDeleteEdge := conn.DeleteEdge(ctx, "d1", "d2", "REL")
	assert.NoError(t, errDeleteEdge)
}

// TestMemGraph_Connection_ExecuteBatch_Comprehensive tests chunking, fallbacks, and error paths in ExecuteBatch.
func TestMemGraph_Connection_ExecuteBatch_Comprehensive(t *testing.T) {
	ctx := context.Background()
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	tx := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockBoltTx{},
	}
	conn.SetOpenTransaction(tx)

	ops := []provider.Operation{
		// Chunked create_node
		{Type: "create_node", Data: provider.Node{ID: "c1", Labels: []string{"Node"}}},
		{Type: "create_node", Data: provider.Node{ID: "c2", Labels: []string{"Node"}}},
		// Invalid create_node payload
		{Type: "create_node", Data: "not-a-node"},
		// update_node
		{Type: "update_node", Data: map[string]any{"id": "c1", "properties": map[string]any{"p": 1}}},
		{Type: "update_node", Data: map[string]any{"id": "c2", "properties": map[string]any{"p": 2}}},
		{Type: "update_node", Data: "invalid-update"},
		// delete_node
		{Type: "delete_node", Data: map[string]any{"id": "c1"}},
		{Type: "delete_node", Data: "invalid-delete"},
		// create_edge
		{Type: "create_edge", Data: provider.Edge{FromID: "c1", ToID: "c2", Type: "CONNECTED"}},
		{Type: "create_edge", Data: "invalid-edge"},
		// update_edge
		{Type: "update_edge", Data: map[string]any{"from_id": "c1", "to_id": "c2", "type": "CONNECTED"}},
		{Type: "update_edge", Data: "invalid-update-edge"},
		// delete_edge
		{Type: "delete_edge", Data: map[string]any{"from_id": "c1", "to_id": "c2", "type": "CONNECTED"}},
		{Type: "delete_edge", Data: "invalid-delete-edge"},
		// query
		{Type: "query", Data: provider.Query{Query: "RETURN 1"}},
		{Type: "query", Data: "invalid-query"},
		// unknown operation
		{Type: "unknown_op_xyz", Data: nil},
	}

	result, err := conn.ExecuteBatch(ctx, ops)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Greater(t, result.SuccessCount, 0)
	assert.Greater(t, result.FailureCount, 0)
}

// TestMemGraph_Transaction_CollectCypherAndRunErrors tests error handling inside transaction Cypher runs.
func TestMemGraph_Transaction_CollectCypherAndRunErrors(t *testing.T) {
	ctx := context.Background()
	conn := &memgraphConnection{BaseConnection: &provider.BaseConnection{}}
	failingTx := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockBoltTx{runErr: assert.AnError},
	}

	// CreateNode run error
	errCreate := failingTx.CreateNode(ctx, provider.Node{ID: "err-1"})
	assert.Error(t, errCreate)

	// GetNode run error
	_, errGet := failingTx.GetNode(ctx, "err-1", nil)
	assert.Error(t, errGet)

	// ExecuteQuery run error
	_, errQuery := failingTx.ExecuteQuery(ctx, provider.Query{Query: "RETURN 1"})
	assert.Error(t, errQuery)

	// collectCypher run error
	_, errCollect := failingTx.collectCypher(ctx, "RETURN 1", nil)
	assert.Error(t, errCollect)
}

// TestMemGraph_BoltHelpers_ExecuteInTransaction exercises executeBoltQueryInTransaction.
func TestMemGraph_BoltHelpers_ExecuteInTransaction(t *testing.T) {
	ctx := context.Background()
	conn := &memgraphConnection{BaseConnection: &provider.BaseConnection{}}

	// Success case
	rec := &neo4j.Record{Values: []any{"ok"}}
	mockTxSuccess := &mockManagedTxForTest{records: []*neo4j.Record{rec}}
	records, errSuccess := conn.executeBoltQueryInTransaction(ctx, mockTxSuccess, "RETURN 1", nil)
	require.NoError(t, errSuccess)
	assert.Len(t, records, 1)

	// Run error case
	mockTxRunErr := &mockManagedTxForTest{runErr: assert.AnError}
	_, errRun := conn.executeBoltQueryInTransaction(ctx, mockTxRunErr, "RETURN 1", nil)
	assert.Error(t, errRun)
	assert.Contains(t, errRun.Error(), "query execution failed")
}

// TestMemGraph_Connection_OpenTx_ListTraversalAndHealth exercises queryRecords, ListNodes,
// ListEdges, ExecuteTraversal, and HealthCheck via open transaction execution.
func TestMemGraph_Connection_OpenTx_ListTraversalAndHealth(t *testing.T) {
	ctx := context.Background()
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}

	nodeVal := neo4j.Node{ElementId: "n-1", Labels: []string{"Item"}, Props: map[string]any{"id": "n-1"}}
	relVal := neo4j.Relationship{ElementId: "r-1", Type: "RELATES", Props: map[string]any{}}
	recordNode := &neo4j.Record{Values: []any{nodeVal}}
	recordEdge := &neo4j.Record{Values: []any{"n-1", "n-2", "RELATES", relVal}}

	mockTx := &mockBoltTx{
		records: []*neo4j.Record{recordNode, recordEdge},
	}
	conn.SetOpenTransaction(&memgraphTransaction{conn: conn, explicitTx: mockTx})

	// 1. ListNodes with all filter variations
	nodesAll, errListAll := conn.ListNodes(ctx, provider.NodeFilter{
		Labels:     []string{"Item"},
		Properties: map[string]any{"status": "active"},
		Limit:      10,
		Offset:     5,
	})
	require.NoError(t, errListAll)
	assert.NotEmpty(t, nodesAll)

	// 2. ListEdges with all filter variations
	edgesAll, errListEdges := conn.ListEdges(ctx, provider.EdgeFilter{
		Type:   "RELATES",
		FromID: "n-1",
		ToID:   "n-2",
		Limit:  10,
		Offset: 2,
	})
	require.NoError(t, errListEdges)
	assert.NotEmpty(t, edgesAll)

	// 3. ExecuteTraversal across directions and depths
	travOut, errTravOut := conn.ExecuteTraversal(ctx, provider.TraversalQuery{
		StartNodeID:  "n-1",
		Direction:    provider.DirectionOutgoing,
		Relationship: "RELATES",
		MaxDepth:     2,
		Filter:       provider.NodeFilter{Labels: []string{"Item"}},
	})
	require.NoError(t, errTravOut)
	assert.NotNil(t, travOut)

	travIn, errTravIn := conn.ExecuteTraversal(ctx, provider.TraversalQuery{
		StartNodeID: "n-2",
		Direction:   provider.DirectionIncoming,
	})
	require.NoError(t, errTravIn)
	assert.NotNil(t, travIn)

	travBoth, errTravBoth := conn.ExecuteTraversal(ctx, provider.TraversalQuery{
		StartNodeID: "n-1",
		Direction:   provider.DirectionBoth,
		MaxDepth:    1,
	})
	require.NoError(t, errTravBoth)
	assert.NotNil(t, travBoth)

	// 4. HealthCheck with real bolt client driver
	realBolt, errBolt := newBoltClient(&MemGraphConfig{Host: "127.0.0.1", Port: 7687})
	require.NoError(t, errBolt)
	t.Cleanup(func() { _ = realBolt.close(ctx) })

	connWithRealBolt := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     realBolt,
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	// Dial will fail quickly on localhost:7687 returning connectivity error
	_ = connWithRealBolt.HealthCheck(timeoutCtx)
}

// TestMemGraph_Pool_ExecuteContextCancellation exercises context cancellation in pool.Execute.
func TestMemGraph_Pool_ExecuteContextCancellation(t *testing.T) {
	cfg := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		Username: "memgraph",
		Password: "password",
		PoolSize: 2,
	}
	pool, err := NewMemGraphConnectionPool(&cfg)
	require.NoError(t, err)
	defer pool.Close()

	// 1. Execute with context canceled inside fn
	cancelCtx, cancel := context.WithCancel(context.Background())
	errCancel := pool.Execute(cancelCtx, func(conn provider.GraphConnection) error {
		cancel() // cancels context
		return assert.AnError
	})
	assert.Error(t, errCancel)

	// 2. Execute with GetConnection error (exhausted pool and short timeout)
	c1, err1 := pool.GetConnection(context.Background())
	require.NoError(t, err1)
	c2, err2 := pool.GetConnection(context.Background())
	require.NoError(t, err2)

	timeoutCtx, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelTimeout()
	errGetFail := pool.Execute(timeoutCtx, func(conn provider.GraphConnection) error {
		return nil
	})
	assert.Error(t, errGetFail)

	_ = pool.ReturnConnection(c1)
	_ = pool.ReturnConnection(c2)
}

type mockFailingBoltTx struct {
	neo4j.ExplicitTransaction
	commitErr   error
	rollbackErr error
}

func (m *mockFailingBoltTx) Commit(ctx context.Context) error {
	return m.commitErr
}

func (m *mockFailingBoltTx) Rollback(ctx context.Context) error {
	return m.rollbackErr
}

type mockSessionWithContext struct {
	neo4j.SessionWithContext
	closeErr error
}

func (s *mockSessionWithContext) Close(ctx context.Context) error {
	return s.closeErr
}

// TestMemGraph_Transaction_CommitRollbackErrors exercises failure branches of Commit and Rollback.
func TestMemGraph_Transaction_CommitRollbackErrors(t *testing.T) {
	ctx := context.Background()
	conn := &memgraphConnection{BaseConnection: &provider.BaseConnection{}}

	// Commit failure
	txCommitFail := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockFailingBoltTx{commitErr: assert.AnError},
	}
	errCommit := txCommitFail.Commit(ctx)
	assert.Error(t, errCommit)
	assert.Contains(t, errCommit.Error(), "failed to commit transaction")

	// Rollback failure
	txRollbackFail := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockFailingBoltTx{rollbackErr: assert.AnError},
	}
	errRollback := txRollbackFail.Rollback(ctx)
	assert.Error(t, errRollback)
	assert.Contains(t, errRollback.Error(), "failed to rollback transaction")

	// CloseInternal with session close and cleanup
	connWithSessionErr := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
		session:        &mockSessionWithContext{closeErr: assert.AnError},
	}
	assert.NoError(t, connWithSessionErr.CloseInternal())
	assert.Nil(t, connWithSessionErr.session)

	connWithSessionOK := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
		session:        &mockSessionWithContext{closeErr: nil},
	}
	assert.NoError(t, connWithSessionOK.CloseInternal())
	assert.Nil(t, connWithSessionOK.session)
}

// TestMemGraph_ExecuteTraversal_And_Reset_Coverage exercises ExecuteTraversal across all directions,
// label and property filters, path unpacking, and Reset session lifecycle.
func TestMemGraph_ExecuteTraversal_And_Reset_Coverage(t *testing.T) {
	ctx := context.Background()

	// 1. ExecuteTraversal when boltClient is uninitialized
	uninitConn := &memgraphConnection{BaseConnection: &provider.BaseConnection{}}
	_, errUninit := uninitConn.ExecuteTraversal(ctx, provider.TraversalQuery{})
	assert.Error(t, errUninit)
	assert.Contains(t, errUninit.Error(), memgraphErrBoltClientNotInitialized)

	// 2. Reset cleans up session
	sessConn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		session:        &mockSessionWithContext{},
	}
	sessConn.Reset()
	assert.Nil(t, sessConn.session)

	// 3. Prepare mock neo4j.Path record
	n1 := neo4j.Node{
		ElementId: "elem-1",
		Labels:    []string{"Task"},
		Props:     map[string]any{"id": "task-1", "name": "Write Test"},
	}
	n2 := neo4j.Node{
		ElementId: "elem-2",
		Labels:    []string{"Goal"},
		Props:     map[string]any{"id": "goal-1", "name": "Elevate Coverage"},
	}
	rel := neo4j.Relationship{
		ElementId: "elem-rel-1",
		Type:      "DEPENDS_ON",
		Props:     map[string]any{"weight": 2.0},
	}
	pathVal := neo4j.Path{
		Nodes:         []neo4j.Node{n1, n2},
		Relationships: []neo4j.Relationship{rel},
	}
	record := &neo4j.Record{
		Keys:   []string{"path", "start", "end"},
		Values: []any{pathVal, n1, n2},
	}

	mockTx := &mockBoltTx{
		records: []*neo4j.Record{record},
	}

	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	tx := &memgraphTransaction{
		conn:       conn,
		explicitTx: mockTx,
	}
	conn.SetOpenTransaction(tx)

	// 4. Traversal queries across all directions and filters
	directions := []provider.Direction{
		provider.DirectionOutgoing,
		provider.DirectionIncoming,
		provider.DirectionBoth,
		provider.Direction("unknown_dir"),
	}

	for _, dir := range directions {
		res, err := conn.ExecuteTraversal(ctx, provider.TraversalQuery{
			StartNodeID:  "task-1",
			Relationship: "DEPENDS_ON",
			Direction:    dir,
			MaxDepth:     3,
			Filter: provider.NodeFilter{
				Labels:     []string{"Task", "Urgent"},
				Properties: map[string]any{"status": "in_progress"},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Len(t, res.Nodes, 2)
		assert.Len(t, res.Edges, 1)
		assert.Equal(t, "task-1", res.Nodes[0].ID)
		assert.Equal(t, "goal-1", res.Nodes[1].ID)
		assert.Equal(t, "DEPENDS_ON", res.Edges[0].Type)
	}

	// 5. CreateNode delegation in transaction and uninitialized boltClient
	errCreateInTx := conn.CreateNode(ctx, provider.Node{
		ID:         "node-in-tx",
		Labels:     []string{"Task"},
		Properties: map[string]any{"key": "value"},
	})
	assert.NoError(t, errCreateInTx)

	errCreateUninit := uninitConn.CreateNode(ctx, provider.Node{})
	assert.Error(t, errCreateUninit)
	assert.Contains(t, errCreateUninit.Error(), memgraphErrBoltClientNotInitialized)
}
