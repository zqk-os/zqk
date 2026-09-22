// BLI-STARTER-COMMUNITY-066 / PRI-STARTER-COMMUNITY-066 coverage elevation
package rpcpool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/graph/provider"
)

func TestExtraUnixRoundtripAndMissPaths(t *testing.T) {
	ctx := context.Background()
	if _, err := ConnectClient(filepath.Join(t.TempDir(), "missing.sock")); err == nil {
		t.Fatal("expected dial fail")
	}

	gp := provider.NewMockGraphProvider()
	inner, err := gp.CreatePool(ctx, provider.ConnectionConfig{MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("zqk-rpcpool-066-%d.sock", os.Getpid()))
	_ = os.Remove(sock)
	t.Cleanup(func() { _ = os.Remove(sock) })
	if err := StartServer(sock, inner); err != nil {
		t.Fatal(err)
	}
	var pool provider.ConnectionPool
	deadline := time.Now().Add(2 * time.Second)
	for {
		pool, err = ConnectClient(sock)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() { _ = pool.Close() })

	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := provider.Node{ID: "n1", Labels: []string{"L"}, Properties: map[string]any{"k": "v"}}
	if err := conn.CreateNode(ctx, n); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.GetNode(ctx, "n1", []string{"L"}); err != nil {
		t.Fatal(err)
	}
	if err := conn.UpdateNode(ctx, "n1", provider.NodeUpdates{Properties: map[string]any{"k": "2"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ListNodes(ctx, provider.NodeFilter{Labels: []string{"L"}}); err != nil {
		t.Fatal(err)
	}
	e := provider.Edge{FromID: "n1", ToID: "n1", Type: "SELF", Properties: map[string]any{"w": 1}}
	if err := conn.CreateEdge(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.GetEdge(ctx, "n1", "n1", "SELF"); err != nil {
		t.Fatal(err)
	}
	if err := conn.UpdateEdge(ctx, "n1", "n1", "SELF", provider.EdgeUpdates{Properties: map[string]any{"w": 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ListEdges(ctx, provider.EdgeFilter{Type: "SELF"}); err != nil {
		t.Fatal(err)
	}
	_, _ = conn.ExecuteQuery(ctx, provider.Query{Query: "MATCH (n:L) RETURN n"})
	_, _ = conn.ExecuteVectorQuery(ctx, provider.VectorQuery{})
	_, _ = conn.ExecuteTraversal(ctx, provider.TraversalQuery{})
	_, _ = conn.ExecuteBatch(ctx, []provider.Operation{{Type: "create_node", Data: provider.Node{ID: "n2", Labels: []string{"L"}}}})
	_ = conn.HealthCheck(ctx)
	_ = conn.HasOpenTransaction()
	_ = conn.GetOpenTransaction()

	tx, err := conn.BeginTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.GetOpenTransaction()
	_ = tx.SupportsNestedTransactions()
	_, _ = tx.BeginNestedTransaction(ctx)
	if rpctx, ok := tx.(*RPCTransaction); ok {
		_, _ = conn.BeginNestedTransaction(ctx, rpctx)
	}
	_ = tx.CreateNode(ctx, provider.Node{ID: "tx-n"})
	_, _ = tx.GetNode(ctx, "tx-n", nil)
	_ = tx.UpdateNode(ctx, "tx-n", provider.NodeUpdates{Properties: map[string]any{"a": 1}})
	_ = tx.CreateEdge(ctx, provider.Edge{FromID: "tx-n", ToID: "tx-n", Type: "T"})
	_, _ = tx.GetEdge(ctx, "tx-n", "tx-n", "T")
	_ = tx.UpdateEdge(ctx, "tx-n", "tx-n", "T", provider.EdgeUpdates{Properties: map[string]any{"b": 1}})
	_ = tx.DeleteEdge(ctx, "tx-n", "tx-n", "T")
	_ = tx.DeleteNode(ctx, "tx-n", nil)
	_, _ = tx.ExecuteQuery(ctx, provider.Query{})
	_ = tx.IsCommitted()
	_ = tx.IsRolledBack()
	_ = tx.GetParent()
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	tx2, err := conn.BeginTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx2.CreateNode(ctx, provider.Node{ID: "tx-c"})
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := conn.DeleteEdge(ctx, "n1", "n1", "SELF"); err != nil {
		t.Fatal(err)
	}
	if err := conn.DeleteNode(ctx, "n1", nil); err != nil {
		t.Fatal(err)
	}
	_ = pool.Stats()
	_ = pool.ReturnConnection(conn)
	_ = pool.(*RPCConnectionPool).ReturnConnection(nil)
	if err := pool.Execute(ctx, func(c provider.GraphConnection) error {
		return c.HealthCheck(ctx)
	}); err != nil {
		t.Fatal(err)
	}

	cli := pool.(*RPCConnectionPool)
	c2, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = c2.Close()
	_ = pool.ReturnConnection(c2)

	bad := &RPCConnection{client: cli.client, connID: 99}
	_ = bad.CreateNode(ctx, provider.Node{})
	_, _ = bad.GetNode(ctx, "x", nil)
	_ = bad.UpdateNode(ctx, "x", provider.NodeUpdates{})
	_ = bad.DeleteNode(ctx, "x", nil)
	_, _ = bad.ListNodes(ctx, provider.NodeFilter{})
	_ = bad.CreateEdge(ctx, provider.Edge{})
	_, _ = bad.GetEdge(ctx, "a", "b", "T")
	_ = bad.UpdateEdge(ctx, "a", "b", "T", provider.EdgeUpdates{})
	_ = bad.DeleteEdge(ctx, "a", "b", "T")
	_, _ = bad.ListEdges(ctx, provider.EdgeFilter{})
	_, _ = bad.ExecuteQuery(ctx, provider.Query{})
	_, _ = bad.ExecuteVectorQuery(ctx, provider.VectorQuery{})
	_, _ = bad.ExecuteTraversal(ctx, provider.TraversalQuery{})
	_, _ = bad.BeginTransaction(ctx)
	_, _ = bad.BeginNestedTransaction(ctx, &RPCTransaction{client: cli.client, txID: 1})
	_ = bad.HasOpenTransaction()
	_ = bad.GetOpenTransaction()
	_ = bad.HealthCheck(ctx)
	_ = bad.Close()
	_, _ = bad.ExecuteBatch(ctx, nil)

	badTx := &RPCTransaction{client: cli.client, txID: 99}
	_ = badTx.CreateNode(ctx, provider.Node{})
	_, _ = badTx.GetNode(ctx, "x", nil)
	_ = badTx.UpdateNode(ctx, "x", provider.NodeUpdates{})
	_ = badTx.DeleteNode(ctx, "x", nil)
	_ = badTx.CreateEdge(ctx, provider.Edge{})
	_, _ = badTx.GetEdge(ctx, "a", "b", "T")
	_ = badTx.UpdateEdge(ctx, "a", "b", "T", provider.EdgeUpdates{})
	_ = badTx.DeleteEdge(ctx, "a", "b", "T")
	_, _ = badTx.ExecuteQuery(ctx, provider.Query{})
	_, _ = badTx.BeginNestedTransaction(ctx)
	_ = badTx.SupportsNestedTransactions()
	_ = badTx.Commit(ctx)
	_ = badTx.Rollback(ctx)
	_ = badTx.IsCommitted()
	_ = badTx.IsRolledBack()
	_ = badTx.GetParent()

	rpc := NewGraphRPC(inner)
	_ = rpc.GetConnection(&GetConnectionArgs{}, &GetConnectionReply{})
	_ = rpc.Stats(&StatsArgs{}, &StatsReply{})
	_ = rpc.ConnCreateNode(&ConnCreateNodeArgs{TargetID: 99}, &ConnCreateNodeReply{})
	_ = rpc.ConnGetNode(&ConnGetNodeArgs{TargetID: 99}, &ConnGetNodeReply{})
	_ = rpc.ConnUpdateNode(&ConnUpdateNodeArgs{TargetID: 99}, &ConnUpdateNodeReply{})
	_ = rpc.ConnDeleteNode(&ConnDeleteNodeArgs{TargetID: 99}, &ConnDeleteNodeReply{})
	_ = rpc.ConnListNodes(&ConnListNodesArgs{TargetID: 99}, &ConnListNodesReply{})
	_ = rpc.ConnCreateEdge(&ConnCreateEdgeArgs{TargetID: 99}, &ConnCreateEdgeReply{})
	_ = rpc.ConnGetEdge(&ConnGetEdgeArgs{TargetID: 99}, &ConnGetEdgeReply{})
	_ = rpc.ConnUpdateEdge(&ConnUpdateEdgeArgs{TargetID: 99}, &ConnUpdateEdgeReply{})
	_ = rpc.ConnDeleteEdge(&ConnDeleteEdgeArgs{TargetID: 99}, &ConnDeleteEdgeReply{})
	_ = rpc.ConnListEdges(&ConnListEdgesArgs{TargetID: 99}, &ConnListEdgesReply{})
	_ = rpc.ConnExecuteQuery(&ConnExecuteQueryArgs{TargetID: 99}, &ConnExecuteQueryReply{})
	_ = rpc.ConnExecuteVectorQuery(&ConnExecuteVectorQueryArgs{TargetID: 99}, &ConnExecuteVectorQueryReply{})
	_ = rpc.ConnExecuteTraversal(&ConnExecuteTraversalArgs{TargetID: 99}, &ConnExecuteTraversalReply{})
	_ = rpc.ConnBeginTransaction(&ConnBeginTransactionArgs{TargetID: 99}, &ConnBeginTransactionReply{})
	_ = rpc.ConnBeginNestedTransaction(&ConnBeginNestedTransactionArgs{TargetID: 99}, &ConnBeginNestedTransactionReply{})
	_ = rpc.ConnHasOpenTransaction(&ConnHasOpenTransactionArgs{TargetID: 99}, &ConnHasOpenTransactionReply{})
	_ = rpc.ConnGetOpenTransaction(&ConnGetOpenTransactionArgs{TargetID: 99}, &ConnGetOpenTransactionReply{})
	_ = rpc.ConnHealthCheck(&ConnHealthCheckArgs{TargetID: 99}, &ConnHealthCheckReply{})
	_ = rpc.ConnClose(&ConnCloseArgs{TargetID: 99}, &ConnCloseReply{})
	_ = rpc.ConnExecuteBatch(&ConnExecuteBatchArgs{TargetID: 99}, &ConnExecuteBatchReply{})
	_ = rpc.TxCreateNode(&TxCreateNodeArgs{TargetID: 99}, &TxCreateNodeReply{})
	_ = rpc.TxGetNode(&TxGetNodeArgs{TargetID: 99}, &TxGetNodeReply{})
	_ = rpc.TxUpdateNode(&TxUpdateNodeArgs{TargetID: 99}, &TxUpdateNodeReply{})
	_ = rpc.TxDeleteNode(&TxDeleteNodeArgs{TargetID: 99}, &TxDeleteNodeReply{})
	_ = rpc.TxCreateEdge(&TxCreateEdgeArgs{TargetID: 99}, &TxCreateEdgeReply{})
	_ = rpc.TxGetEdge(&TxGetEdgeArgs{TargetID: 99}, &TxGetEdgeReply{})
	_ = rpc.TxUpdateEdge(&TxUpdateEdgeArgs{TargetID: 99}, &TxUpdateEdgeReply{})
	_ = rpc.TxDeleteEdge(&TxDeleteEdgeArgs{TargetID: 99}, &TxDeleteEdgeReply{})
	_ = rpc.TxExecuteQuery(&TxExecuteQueryArgs{TargetID: 99}, &TxExecuteQueryReply{})
	_ = rpc.TxBeginNestedTransaction(&TxBeginNestedTransactionArgs{TargetID: 99}, &TxBeginNestedTransactionReply{})
	_ = rpc.TxSupportsNestedTransactions(&TxSupportsNestedTransactionsArgs{TargetID: 99}, &TxSupportsNestedTransactionsReply{})
	_ = rpc.TxCommit(&TxCommitArgs{TargetID: 99}, &TxCommitReply{})
	_ = rpc.TxRollback(&TxRollbackArgs{TargetID: 99}, &TxRollbackReply{})
	_ = rpc.TxIsCommitted(&TxIsCommittedArgs{TargetID: 99}, &TxIsCommittedReply{})
	_ = rpc.TxIsRolledBack(&TxIsRolledBackArgs{TargetID: 99}, &TxIsRolledBackReply{})
	_ = rpc.TxGetParent(&TxGetParentArgs{TargetID: 99}, &TxGetParentReply{})
	_ = rpc.ReturnConnection(&ReturnConnectionArgs{ConnID: 99}, &ReturnConnectionReply{})

	failRPC := NewGraphRPC(extraFailPool{})
	_ = failRPC.GetConnection(&GetConnectionArgs{}, &GetConnectionReply{})
	_ = failRPC.ReturnConnection(&ReturnConnectionArgs{ConnID: 1}, &ReturnConnectionReply{})
	failRPC.conns[1] = extraFailConn{}
	_ = failRPC.ReturnConnection(&ReturnConnectionArgs{ConnID: 1}, &ReturnConnectionReply{})
}

type extraFailPool struct{}

func (extraFailPool) GetConnection(context.Context) (provider.GraphConnection, error) {
	return nil, fmt.Errorf("no conn")
}
func (extraFailPool) ReturnConnection(provider.GraphConnection) error { return fmt.Errorf("no ret") }
func (extraFailPool) Execute(context.Context, func(provider.GraphConnection) error) error {
	return fmt.Errorf("no exec")
}
func (extraFailPool) Stats() provider.PoolStats { return provider.PoolStats{} }
func (extraFailPool) Close() error              { return nil }

type extraFailConn struct{ provider.GraphConnection }

func (extraFailConn) Close() error { return fmt.Errorf("close") }
