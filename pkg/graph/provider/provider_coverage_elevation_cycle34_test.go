package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type errGetConnPool struct{}

func (e *errGetConnPool) GetConnection(ctx context.Context) (GraphConnection, error) {
	return nil, errors.New("simulated get connection failure")
}
func (e *errGetConnPool) ReturnConnection(conn GraphConnection) error { return nil }
func (e *errGetConnPool) Execute(ctx context.Context, fn func(conn GraphConnection) error) error {
	return errors.New("execute fail")
}
func (e *errGetConnPool) Close() error     { return nil }
func (e *errGetConnPool) Stats() PoolStats { return PoolStats{} }

type nonWrapperConn struct {
	GraphConnection
}

// TestProvider_QueryAndInterfaceHelpers exercises ToLabel, ExtractBooleanField, and ConnectViaPool.
func TestProvider_QueryAndInterfaceHelpers(t *testing.T) {
	ctx := context.Background()

	// 1. ToLabel
	assert.Equal(t, "BacklogItem", ToLabel("backlog_item"))
	assert.Equal(t, "Goal", ToLabel("goal"))
	assert.Equal(t, "TestCaseV2", ToLabel("test_case_v2"))
	assert.Equal(t, "MultiWordIdentifier", ToLabel("_multi__word__identifier_"))
	assert.Equal(t, "", ToLabel(""))
	assert.Equal(t, "", ToLabel("___"))
	assert.Equal(t, "", ToLabel("unsafe;injection"))
	assert.Equal(t, "", ToLabel("drop-table"))

	// 2. ExtractBooleanField
	// Nil QueryResult
	var nilQR *QueryResult
	_, errNilQR := nilQR.ExtractBooleanField("is_valid")
	assert.Error(t, errNilQR)
	assert.Contains(t, errNilQR.Error(), "no rows returned")

	// Empty rows
	emptyQR := &QueryResult{Rows: []map[string]any{}}
	_, errEmptyRows := emptyQR.ExtractBooleanField("is_valid")
	assert.Error(t, errEmptyRows)

	// Field not boolean
	strQR := &QueryResult{Rows: []map[string]any{{"is_valid": "true"}}}
	_, errNotBool := strQR.ExtractBooleanField("is_valid")
	assert.Error(t, errNotBool)
	assert.Contains(t, errNotBool.Error(), "field is not a boolean")

	// Valid boolean
	validQR := &QueryResult{Rows: []map[string]any{{"is_valid": true}}}
	val, errValid := validQR.ExtractBooleanField("is_valid")
	require.NoError(t, errValid)
	assert.True(t, val)

	// 3. ConnectViaPool
	// CreatePool error
	_, _, errPoolFail := ConnectViaPool(ctx, ConnectionConfig{}, func(ctx context.Context, cfg ConnectionConfig) (ConnectionPool, error) {
		return nil, errors.New("pool creation failed")
	})
	assert.Error(t, errPoolFail)

	// GetConnection error
	_, _, errGetConnFail := ConnectViaPool(ctx, ConnectionConfig{PoolTimeout: time.Millisecond}, func(ctx context.Context, cfg ConnectionConfig) (ConnectionPool, error) {
		return &errGetConnPool{}, nil
	})
	assert.Error(t, errGetConnFail)

	// ConnectViaPool success with MockGraphProvider
	mockProv := NewMockGraphProvider()
	conn, pool, errSuccess := ConnectViaPool(ctx, ConnectionConfig{}, mockProv.CreatePool)
	require.NoError(t, errSuccess)
	require.NotNil(t, conn)
	require.NotNil(t, pool)
	assert.NoError(t, pool.Close())
}

// TestProvider_BasePool_LifecycleAndConcurrency exercises BasePool creation, timeouts,
// concurrency, prepopulation, and transaction safety on return.
func TestProvider_BasePool_LifecycleAndConcurrency(t *testing.T) {
	ctx := context.Background()

	// 1. NewBasePool with maxSize <= 0 defaults to 10
	pDefault := NewBasePool(&ConnectionConfig{}, 0)
	require.NotNil(t, pDefault)
	assert.Equal(t, 10, pDefault.maxSize)
	_ = pDefault.Close()

	// 2. BasePool with small capacity
	cfg := ConnectionConfig{
		PoolTimeout: 50 * time.Millisecond,
	}
	pool := NewBasePool(&cfg, 2)
	require.NotNil(t, pool)

	createConn := func() (ConnectionWrapper, error) {
		return &MockConnection{
			BaseConnection: BaseConnection{},
			store:          NewMockStore(),
		}, nil
	}

	// 3. PrePopulate
	pool.PrePopulate(ctx, createConn, 0) // <= 0 defaults to half
	pool.PrePopulate(ctx, func() (ConnectionWrapper, error) {
		return nil, errors.New("conn creation failed") // error handled gracefully
	}, 1)
	pool.PrePopulate(ctx, createConn, 5) // clamped to maxSize (2)
	assert.Equal(t, 2, pool.Stats().Idle)

	// 4. Acquire existing from pool
	c1, errC1 := pool.GetConnection(ctx, createConn)
	require.NoError(t, errC1)
	assert.Equal(t, 1, pool.Stats().Active)

	c2, errC2 := pool.GetConnection(ctx, createConn)
	require.NoError(t, errC2)
	assert.Equal(t, 2, pool.Stats().Active)

	// 5. Pool is at capacity: acquiring 3rd times out via PoolTimeout
	_, errTimeout := pool.GetConnection(ctx, createConn)
	require.Error(t, errTimeout)
	graphErr, ok := errTimeout.(*GraphError)
	require.True(t, ok)
	assert.Equal(t, ErrorCodeTimeout, graphErr.Code)

	// 6. Return connection with open transaction (safety rollback)
	mockC1 := c1.(*MockConnection)
	tx := &MockTransaction{conn: mockC1}
	mockC1.SetOpenTransaction(tx)
	assert.True(t, c1.HasOpenTransaction())
	require.NoError(t, pool.ReturnConnection(c1))
	assert.False(t, c1.HasOpenTransaction())

	// 7. GetConnection blocks then succeeds when connection returned concurrently
	// POL-CONCURRENCY: Managed background goroutine
	goroutinelabels.NewGoroutine("test_base_pool_return_worker", "returns connection to pool after sleep").
		StartSimple(func() {
			time.Sleep(20 * time.Millisecond)
			_ = pool.ReturnConnection(c2)
		})

	c3, errC3 := pool.GetConnection(context.Background(), createConn)
	require.NoError(t, errC3)
	require.NotNil(t, c3)

	// 8. ReturnConnection overflow (channel full) triggers default branch
	extraConn := &MockConnection{BaseConnection: BaseConnection{}, store: NewMockStore()}
	_ = pool.ReturnConnection(c3)        // fills channel (item 1)
	_ = pool.ReturnConnection(extraConn) // fills channel (item 2)
	overflowConn := &MockConnection{BaseConnection: BaseConnection{}, store: NewMockStore()}
	// 3rd return overflows channel -> calls CloseInternal
	errOverflow := pool.ReturnConnection(overflowConn)
	assert.NoError(t, errOverflow)

	// 9. Execute with error and success
	errExec := pool.Execute(ctx, createConn, func(conn GraphConnection) error {
		return errors.New("exec error")
	})
	assert.EqualError(t, errExec, "exec error")

	errExecSuccess := pool.Execute(ctx, createConn, func(conn GraphConnection) error {
		return nil
	})
	assert.NoError(t, errExecSuccess)

	// MockConnectionPool ReturnConnection non-wrapper error
	mockProv := NewMockGraphProvider()
	mPool, errMP := mockProv.CreatePool(ctx, ConnectionConfig{})
	require.NoError(t, errMP)
	defer mPool.Close()
	errNonWrap := mPool.ReturnConnection(&nonWrapperConn{})
	assert.NoError(t, errNonWrap)

	// 10. Close pool
	require.NoError(t, pool.Close())
}

// TestProvider_BoundEdgeTraverser_FullCoverage exercises edge indexing, removal,
// direction options, and Cypher subset parsing.
func TestProvider_BoundEdgeTraverser_FullCoverage(t *testing.T) {
	ctx := context.Background()

	// 1. NewBoundEdgeTraverser defaults
	travDefault := NewBoundEdgeTraverser(nil, 0, 0)
	require.NotNil(t, travDefault)
	assert.Equal(t, DefaultMaxTraversalBound, travDefault.defaultBound)
	assert.Equal(t, DefaultTraversalTimeout, travDefault.defaultTimeout)

	// 2. IndexNode and RemoveNode edge cases
	travDefault.IndexNode(nil)
	travDefault.IndexNode(&Node{ID: ""})
	travDefault.RemoveNode("")
	travDefault.IndexEdge(nil)
	travDefault.IndexEdge(&Edge{FromID: ""})
	travDefault.RemoveEdge("", "", "")
	travDefault.RemoveEdge("missing-1", "missing-2", "rel")

	// Index valid nodes and edges
	n1 := &Node{ID: "node-1", Labels: []string{"Task"}}
	n2 := &Node{ID: "node-2", Labels: []string{"Goal"}}
	n3 := &Node{ID: "node-3", Labels: []string{"Goal"}}
	travDefault.IndexNode(n1)
	travDefault.IndexNode(n2)
	travDefault.IndexNode(n3)

	e1 := &Edge{FromID: "node-1", ToID: "node-2", Type: "DEPENDS_ON"}
	e2 := &Edge{FromID: "node-3", ToID: "node-1", Type: "BLOCKS"}
	travDefault.IndexEdge(e1)
	travDefault.IndexEdge(e2)

	// 3. Traverse1Hop guards and directions
	// Cancelled context
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, errCancel := travDefault.Traverse1Hop(cancelCtx, TraversalQuery{})
	assert.Error(t, errCancel)

	// Empty StartNodeID
	_, errEmptyStart := travDefault.Traverse1Hop(ctx, TraversalQuery{StartNodeID: "   "})
	assert.Error(t, errEmptyStart)
	assert.Contains(t, errEmptyStart.Error(), "start node ID cannot be empty")

	// Direction Incoming
	resIn, errIn := travDefault.Traverse1Hop(ctx, TraversalQuery{
		StartNodeID:  "node-1",
		Direction:    DirectionIncoming,
		Relationship: "BLOCKS",
	})
	require.NoError(t, errIn)
	assert.Len(t, resIn.Nodes, 1)
	assert.Equal(t, "node-3", resIn.Nodes[0].ID)

	// Direction Outgoing
	resOut, errOut := travDefault.Traverse1Hop(ctx, TraversalQuery{
		StartNodeID: "node-1",
		Direction:   DirectionOutgoing,
	})
	require.NoError(t, errOut)
	assert.Len(t, resOut.Nodes, 1)
	assert.Equal(t, "node-2", resOut.Nodes[0].ID)

	// Direction Both without relationship filter
	resBoth, errBoth := travDefault.Traverse1Hop(ctx, TraversalQuery{
		StartNodeID: "node-1",
		Direction:   DirectionBoth,
	})
	require.NoError(t, errBoth)
	assert.Len(t, resBoth.Nodes, 2)

	// Label filtering in traversal
	resFiltered, errFiltered := travDefault.Traverse1Hop(ctx, TraversalQuery{
		StartNodeID: "node-1",
		Direction:   DirectionBoth,
		Filter:      NodeFilter{Labels: []string{"Task"}}, // target nodes are Goal, not Task
	})
	require.NoError(t, errFiltered)
	assert.Empty(t, resFiltered.Nodes)

	// Property filtering in traversal (mismatch)
	resPropMismatch, errPropMismatch := travDefault.Traverse1Hop(ctx, TraversalQuery{
		StartNodeID: "node-1",
		Direction:   DirectionBoth,
		Filter:      NodeFilter{Properties: map[string]any{"color": "blue"}},
	})
	require.NoError(t, errPropMismatch)
	assert.Empty(t, resPropMismatch.Nodes)

	// Limit clamping traversal results
	resLimit, errLimit := travDefault.Traverse1Hop(ctx, TraversalQuery{
		StartNodeID: "node-1",
		Direction:   DirectionBoth,
		Filter:      NodeFilter{Limit: 1},
	})
	require.NoError(t, errLimit)
	assert.Len(t, resLimit.Nodes, 1)

	// 4. ExecuteCypherSubset coverage
	// Cancelled context
	_, errCypherCancel := travDefault.ExecuteCypherSubset(cancelCtx, "MATCH (n) RETURN n", nil)
	assert.Error(t, errCypherCancel)

	// Empty cypher
	_, errCypherEmpty := travDefault.ExecuteCypherSubset(ctx, "   ", nil)
	assert.Error(t, errCypherEmpty)
	assert.Contains(t, errCypherEmpty.Error(), "empty Cypher query")

	// Pattern 1: MATCH (n:Goal) RETURN n LIMIT 10
	p1Res, errP1 := travDefault.ExecuteCypherSubset(ctx, "MATCH (n:Goal) RETURN n LIMIT 10", nil)
	require.NoError(t, errP1)
	assert.Len(t, p1Res.Nodes, 2)

	// Pattern 2: MATCH (a:Task)-[r:DEPENDS_ON]->(b:Goal) WHERE a.id = $id RETURN b
	// Missing start node parameter
	_, errP2NoParam := travDefault.ExecuteCypherSubset(ctx, "MATCH (a:Task)-[r:DEPENDS_ON]->(b:Goal) WHERE a.id = $id RETURN b", nil)
	assert.Error(t, errP2NoParam)
	assert.Contains(t, errP2NoParam.Error(), "missing start node ID")

	// Valid Pattern 2
	p2Res, errP2 := travDefault.ExecuteCypherSubset(ctx, "MATCH (a:Task)-[r:DEPENDS_ON]->(b:Goal) WHERE a.id = $id RETURN b", map[string]any{"id": "node-1"})
	require.NoError(t, errP2)
	assert.Len(t, p2Res.Nodes, 1)
	assert.Equal(t, "node-2", p2Res.Nodes[0].ID)

	// Unsupported Cypher
	_, errUnsupported := travDefault.ExecuteCypherSubset(ctx, "DROP DATABASE graph", nil)
	assert.Error(t, errUnsupported)
	assert.Contains(t, errUnsupported.Error(), "unsupported Cypher query")

	// Stats and counts
	traversals, fullScans := travDefault.Stats()
	assert.Greater(t, traversals, int64(0))
	assert.Equal(t, int64(0), fullScans)
	assert.Equal(t, int64(0), travDefault.FullScanCount())

	// Remove edge and node
	travDefault.RemoveEdge(e1.FromID, e1.ToID, e1.Type)
	travDefault.RemoveNode("node-1")
	assert.Equal(t, 2, travDefault.labelIndex.TotalIndexedNodes())
}

// TestProvider_Retry_EdgeCases exercises all retry conditions, backoffs, and context cancellations.
func TestProvider_Retry_EdgeCases(t *testing.T) {
	ctx := context.Background()

	// 1. Context already cancelled before retry
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	errCancelled := Retry(cancelCtx, nil, func() error {
		return nil
	})
	assert.Error(t, errCancelled)
	assert.Equal(t, ErrorCodeDeadlineExceeded, errCancelled.(*GraphError).Code)

	// 2. Non-GraphError returns immediately without retrying
	attempts := 0
	errNonGraph := Retry(ctx, &RetryConfig{MaxAttempts: 5}, func() error {
		attempts++
		return errors.New("plain non-retryable error")
	})
	assert.Equal(t, 1, attempts)
	assert.EqualError(t, errNonGraph, "plain non-retryable error")

	// 3. GraphError with non-retryable code
	nonRetryableErr := &GraphError{
		Code:    ErrorCodeNodeNotFound,
		Message: "node does not exist",
	}
	attempts2 := 0
	errGraphNonRetry := Retry(ctx, &RetryConfig{MaxAttempts: 5}, func() error {
		attempts2++
		return nonRetryableErr
	})
	assert.Equal(t, 1, attempts2)
	assert.Equal(t, nonRetryableErr, errGraphNonRetry)

	// 4. GraphError configured explicitly in RetryableErrors list
	customRetryableErr := &GraphError{
		Code:    ErrorCodeTransactionFailed,
		Message: "temporary lock conflict",
	}
	attempts3 := 0
	cfgCustom := &RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    time.Millisecond,
		MaxDelay:        5 * time.Millisecond,
		BackoffFactor:   2.0,
		RetryableErrors: []ErrorCode{ErrorCodeTransactionFailed},
	}
	errCustomExhausted := Retry(ctx, cfgCustom, func() error {
		attempts3++
		return customRetryableErr
	})
	assert.Equal(t, 3, attempts3)
	assert.Equal(t, ErrorCodeRetryExhausted, errCustomExhausted.(*GraphError).Code)

	// 5. Successful retry on 2nd attempt
	attempts4 := 0
	transientErr := &GraphError{
		Code:    ErrorCodeConnectionFailed,
		Message: "connection dropped",
	}
	errSuccessOnRetry := Retry(ctx, cfgCustom, func() error {
		attempts4++
		if attempts4 == 1 {
			return transientErr
		}
		return nil
	})
	assert.NoError(t, errSuccessOnRetry)
	assert.Equal(t, 2, attempts4)

	// 6. WithTimeout helper
	ctxWithDeadline, cancelD := context.WithTimeout(ctx, time.Second)
	defer cancelD()
	c1, cancel1 := WithTimeout(ctxWithDeadline, 5*time.Second)
	defer cancel1()
	assert.Equal(t, ctxWithDeadline, c1)

	c2, cancel2 := WithTimeout(ctx, 50*time.Millisecond)
	defer cancel2()
	deadline, hasDeadline := c2.Deadline()
	assert.True(t, hasDeadline)
	assert.WithinDuration(t, time.Now().Add(50*time.Millisecond), deadline, 10*time.Millisecond)
}

// TestProvider_Health_Recommendations_FullCoverage exercises all threshold and recommendation paths.
func TestProvider_Health_Recommendations_FullCoverage(t *testing.T) {
	thresholds := HealthThresholds{
		ErrorRateThreshold:          5.0,
		UnhealthyErrorRateThreshold: 20.0,
		RetryRateThreshold:          10.0,
		PoolUtilizationThreshold:    80.0,
		MaxWaitTimeThreshold:        100 * time.Millisecond,
		SlowQueryThreshold:          50 * time.Millisecond,
		ConsecutiveFailureThreshold: 3,
	}

	// 1. Healthy state
	healthySnapshot := MetricsSnapshot{
		TotalOperations: 100,
		Operations: map[string]OperationMetrics{
			"get_node": {Count: 100, FailureCount: 1, TotalDuration: 100 * time.Millisecond},
		},
		Pool: PoolMetrics{UtilizationRate: 50.0, MaxWaitTime: 10 * time.Millisecond},
	}
	indicatorHealthy := DiagnoseHealthWithThresholds(&healthySnapshot, thresholds)
	assert.Equal(t, HealthStateHealthy, indicatorHealthy.Status)
	assert.Empty(t, indicatorHealthy.Recommendations)

	// 2. Unhealthy state due to error rate and consecutive failures
	unhealthySnapshot := MetricsSnapshot{
		TotalOperations: 100,
		Operations: map[string]OperationMetrics{
			"create_node": {Count: 100, FailureCount: 25, TotalDuration: 500 * time.Millisecond},
		},
		Health: HealthMetrics{ConsecutiveFailures: 4},
		Transactions: TransactionMetrics{
			TotalStarted:      20,
			TransactionErrors: 5,
		},
		Queries: map[string]QueryMetrics{
			"slow_q": {Query: "MATCH (n) RETURN n", AvgDuration: 200 * time.Millisecond},
		},
	}
	indicatorUnhealthy := DiagnoseHealthWithThresholds(&unhealthySnapshot, thresholds)
	assert.Equal(t, HealthStateUnhealthy, indicatorUnhealthy.Status)
	assert.NotEmpty(t, indicatorUnhealthy.Recommendations)

	// 3. Degraded state due to elevated error rate, retries, pool utilization, and wait time
	degradedSnapshot := MetricsSnapshot{
		TotalOperations: 100,
		TotalRetries:    15,
		Operations: map[string]OperationMetrics{
			"update_node": {Count: 100, FailureCount: 8, TotalDuration: 200 * time.Millisecond},
		},
		Pool: PoolMetrics{
			UtilizationRate: 85.0,
			MaxWaitTime:     150 * time.Millisecond,
		},
	}
	indicatorDegraded := DiagnoseHealthWithThresholds(&degradedSnapshot, thresholds)
	assert.Equal(t, HealthStateDegraded, indicatorDegraded.Status)
	assert.NotEmpty(t, indicatorDegraded.Recommendations)

	// DiagnoseHealth with default thresholds
	_ = DiagnoseHealth(healthySnapshot)
}

// TestProvider_MockStore_And_Transaction_Errors tests MockTransaction failures and MockStore edge cases.
func TestProvider_MockStore_And_Transaction_Errors(t *testing.T) {
	ctx := context.Background()
	provider := NewMockGraphProvider()
	conn, err := provider.Connect(ctx, ConnectionConfig{})
	require.NoError(t, err)
	defer conn.Close()

	// 1. Transaction error on commit
	tx, errTx := conn.BeginTransaction(ctx)
	require.NoError(t, errTx)
	require.NotNil(t, tx)

	// Operation within transaction on non-existent node
	errUp := tx.UpdateNode(ctx, "non_existent_node_xyz", NodeUpdates{Properties: map[string]any{"a": 1}})
	require.NoError(t, errUp) // queued operation

	// Commit executes the queued operation, which fails
	errCommitFail := tx.Commit(ctx)
	assert.Error(t, errCommitFail)
	_ = tx.Rollback(ctx) // clear failed transaction from connection

	// 2. Success commit sets finished state
	txSuccess, errTxSuccess := conn.BeginTransaction(ctx)
	require.NoError(t, errTxSuccess)
	require.NoError(t, txSuccess.Commit(ctx))
	assert.EqualError(t, txSuccess.Commit(ctx), "transaction already finished")
	assert.EqualError(t, txSuccess.Rollback(ctx), "transaction already finished")

	// 3. Nested transaction unsupported
	assert.False(t, tx.SupportsNestedTransactions())
	_, errNested := tx.BeginNestedTransaction(ctx)
	assert.Error(t, errNested)
	assert.Nil(t, tx.GetParent())

	// 4. ListEdges filter variations
	_ = conn.CreateNode(ctx, Node{ID: "n-a", Labels: []string{"A"}, Properties: map[string]any{"env": "test"}})
	_ = conn.CreateNode(ctx, Node{ID: "n-b", Labels: []string{"B"}, Properties: map[string]any{"env": "test"}})
	_ = conn.CreateEdge(ctx, Edge{FromID: "n-a", ToID: "n-b", Type: "LINK", Properties: map[string]any{"weight": 10}})

	edgesFiltered, errList := conn.ListEdges(ctx, EdgeFilter{
		FromID:     "n-a",
		ToID:       "n-b",
		Type:       "LINK",
		Properties: map[string]any{"weight": 10},
		Limit:      5,
		Offset:     0,
	})
	require.NoError(t, errList)
	assert.Len(t, edgesFiltered, 1)

	// ListNodes filter variations
	nodesFiltered, errListN := conn.ListNodes(ctx, NodeFilter{
		Labels:     []string{"A"},
		Properties: map[string]any{"env": "test"},
		Limit:      5,
		Offset:     0,
	})
	require.NoError(t, errListN)
	assert.Len(t, nodesFiltered, 1)

	// UpdateNode with missing node returns ErrorCodeNodeNotFound
	errMissingNode := conn.UpdateNode(ctx, "missing-xyz", NodeUpdates{})
	assert.Error(t, errMissingNode)

	// GetNode with missing node returns error
	_, errMissingGetNode := conn.GetNode(ctx, "missing-xyz", nil)
	assert.Error(t, errMissingGetNode)

	// UpdateEdge with missing edge returns error
	errMissingUpdateEdge := conn.UpdateEdge(ctx, "missing-1", "missing-2", "LINK", EdgeUpdates{})
	assert.Error(t, errMissingUpdateEdge)

	// GetEdge with missing edge returns error
	_, errMissingGetEdge := conn.GetEdge(ctx, "missing-1", "missing-2", "LINK")
	assert.Error(t, errMissingGetEdge)

	// Clean up created nodes and edges
	assert.NoError(t, conn.DeleteEdge(ctx, "n-a", "n-b", "LINK"))
	assert.NoError(t, conn.DeleteNode(ctx, "n-a", nil))
	assert.NoError(t, conn.DeleteNode(ctx, "n-b", nil))
}
