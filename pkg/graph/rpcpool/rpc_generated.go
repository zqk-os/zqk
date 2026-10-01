package rpcpool

import (
	"context"
	"fmt"
	"net/rpc"
	"sync"

	"github.com/zqk-os/zqk/pkg/graph/provider"
)

// --- API ---

type ConnCreateNodeArgs struct {
	TargetID int64
	Node     provider.Node
}
type ConnCreateNodeReply struct {
	ErrStr string
}

type ConnGetNodeArgs struct {
	TargetID int64
	Id       string
	Labels   []string
}
type ConnGetNodeReply struct {
	Val    *provider.Node
	ErrStr string
}

type ConnUpdateNodeArgs struct {
	TargetID int64
	Id       string
	Updates  provider.NodeUpdates
}
type ConnUpdateNodeReply struct {
	ErrStr string
}

type ConnDeleteNodeArgs struct {
	TargetID int64
	Id       string
	Labels   []string
}
type ConnDeleteNodeReply struct {
	ErrStr string
}

type ConnListNodesArgs struct {
	TargetID int64
	Filter   provider.NodeFilter
}
type ConnListNodesReply struct {
	Val    []*provider.Node
	ErrStr string
}

type ConnCreateEdgeArgs struct {
	TargetID int64
	Edge     provider.Edge
}
type ConnCreateEdgeReply struct {
	ErrStr string
}

type ConnGetEdgeArgs struct {
	TargetID int64
	Fromid   string
	Toid     string
	Edgetype string
}
type ConnGetEdgeReply struct {
	Val    *provider.Edge
	ErrStr string
}

type ConnUpdateEdgeArgs struct {
	TargetID int64
	Fromid   string
	Toid     string
	Edgetype string
	Updates  provider.EdgeUpdates
}
type ConnUpdateEdgeReply struct {
	ErrStr string
}

type ConnDeleteEdgeArgs struct {
	TargetID int64
	Fromid   string
	Toid     string
	Edgetype string
}
type ConnDeleteEdgeReply struct {
	ErrStr string
}

type ConnListEdgesArgs struct {
	TargetID int64
	Filter   provider.EdgeFilter
}
type ConnListEdgesReply struct {
	Val    []*provider.Edge
	ErrStr string
}

type ConnExecuteQueryArgs struct {
	TargetID int64
	Query    provider.Query
}
type ConnExecuteQueryReply struct {
	Val    *provider.QueryResult
	ErrStr string
}

type ConnExecuteVectorQueryArgs struct {
	TargetID int64
	Query    provider.VectorQuery
}
type ConnExecuteVectorQueryReply struct {
	Val    *provider.QueryResult
	ErrStr string
}

type ConnExecuteTraversalArgs struct {
	TargetID  int64
	Traversal provider.TraversalQuery
}
type ConnExecuteTraversalReply struct {
	Val    *provider.QueryResult
	ErrStr string
}

type ConnBeginTransactionArgs struct {
	TargetID int64
}
type ConnBeginTransactionReply struct {
	Val    int64
	ErrStr string
}

type ConnBeginNestedTransactionArgs struct {
	TargetID int64
	Parent   int64
}
type ConnBeginNestedTransactionReply struct {
	Val    int64
	ErrStr string
}

type ConnHasOpenTransactionArgs struct {
	TargetID int64
}
type ConnHasOpenTransactionReply struct {
	Val    bool
	ErrStr string
}

type ConnGetOpenTransactionArgs struct {
	TargetID int64
}
type ConnGetOpenTransactionReply struct {
	Val    int64
	ErrStr string
}

type ConnHealthCheckArgs struct {
	TargetID int64
}
type ConnHealthCheckReply struct {
	ErrStr string
}

type ConnCloseArgs struct {
	TargetID int64
}
type ConnCloseReply struct {
	ErrStr string
}

type ConnExecuteBatchArgs struct {
	TargetID   int64
	Operations []provider.Operation
}
type ConnExecuteBatchReply struct {
	Val    *provider.BatchResult
	ErrStr string
}

type TxCreateNodeArgs struct {
	TargetID int64
	Node     provider.Node
}
type TxCreateNodeReply struct {
	ErrStr string
}

type TxGetNodeArgs struct {
	TargetID int64
	Id       string
	Labels   []string
}
type TxGetNodeReply struct {
	Val    *provider.Node
	ErrStr string
}

type TxUpdateNodeArgs struct {
	TargetID int64
	Id       string
	Updates  provider.NodeUpdates
}
type TxUpdateNodeReply struct {
	ErrStr string
}

type TxDeleteNodeArgs struct {
	TargetID int64
	Id       string
	Labels   []string
}
type TxDeleteNodeReply struct {
	ErrStr string
}

type TxCreateEdgeArgs struct {
	TargetID int64
	Edge     provider.Edge
}
type TxCreateEdgeReply struct {
	ErrStr string
}

type TxGetEdgeArgs struct {
	TargetID int64
	Fromid   string
	Toid     string
	Edgetype string
}
type TxGetEdgeReply struct {
	Val    *provider.Edge
	ErrStr string
}

type TxUpdateEdgeArgs struct {
	TargetID int64
	Fromid   string
	Toid     string
	Edgetype string
	Updates  provider.EdgeUpdates
}
type TxUpdateEdgeReply struct {
	ErrStr string
}

type TxDeleteEdgeArgs struct {
	TargetID int64
	Fromid   string
	Toid     string
	Edgetype string
}
type TxDeleteEdgeReply struct {
	ErrStr string
}

type TxExecuteQueryArgs struct {
	TargetID int64
	Query    provider.Query
}
type TxExecuteQueryReply struct {
	Val    *provider.QueryResult
	ErrStr string
}

type TxBeginNestedTransactionArgs struct {
	TargetID int64
}
type TxBeginNestedTransactionReply struct {
	Val    int64
	ErrStr string
}

type TxSupportsNestedTransactionsArgs struct {
	TargetID int64
}
type TxSupportsNestedTransactionsReply struct {
	Val    bool
	ErrStr string
}

type TxCommitArgs struct {
	TargetID int64
}
type TxCommitReply struct {
	ErrStr string
}

type TxRollbackArgs struct {
	TargetID int64
}
type TxRollbackReply struct {
	ErrStr string
}

type TxIsCommittedArgs struct {
	TargetID int64
}
type TxIsCommittedReply struct {
	Val    bool
	ErrStr string
}

type TxIsRolledBackArgs struct {
	TargetID int64
}
type TxIsRolledBackReply struct {
	Val    bool
	ErrStr string
}

type TxGetParentArgs struct {
	TargetID int64
}
type TxGetParentReply struct {
	Val    int64
	ErrStr string
}

// --- Client ---

type RPCConnectionPool struct {
	client *rpc.Client
}

func NewRPCConnectionPool(client *rpc.Client) *RPCConnectionPool {
	return &RPCConnectionPool{client: client}
}

type GetConnectionArgs struct{}
type GetConnectionReply struct {
	ConnID int64
	ErrStr string
}

// invokeRPC sends an RPC request and validates both network errors and application error strings.
func invokeRPC(client *rpc.Client, method string, args any, reply any, errStr *string) error {
	if err := client.Call(method, args, reply); err != nil {
		return err
	}
	if *errStr != "" {
		return fmt.Errorf("%s", *errStr)
	}
	return nil
}

func (p *RPCConnectionPool) GetConnection(ctx context.Context) (provider.GraphConnection, error) {
	var reply GetConnectionReply
	if err := invokeRPC(p.client, "GraphRPC.GetConnection", GetConnectionArgs{}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return &RPCConnection{client: p.client, connID: reply.ConnID}, nil
}

type ReturnConnectionArgs struct{ ConnID int64 }
type ReturnConnectionReply struct{ ErrStr string }

func (p *RPCConnectionPool) ReturnConnection(conn provider.GraphConnection) error {
	c, ok := conn.(*RPCConnection)
	if !ok {
		return nil
	}
	var reply ReturnConnectionReply
	return invokeRPC(p.client, "GraphRPC.ReturnConnection", ReturnConnectionArgs{ConnID: c.connID}, &reply, &reply.ErrStr)
}

func (p *RPCConnectionPool) Execute(ctx context.Context, fn func(conn provider.GraphConnection) error) error {
	conn, err := p.GetConnection(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if retErr := p.ReturnConnection(conn); retErr != nil {
			return
		}
	}()
	return fn(conn)
}

type StatsArgs struct{}
type StatsReply struct{ Stats provider.PoolStats }

func (p *RPCConnectionPool) Stats() provider.PoolStats {
	var reply StatsReply
	if err := p.client.Call("GraphRPC.Stats", StatsArgs{}, &reply); err != nil {
		return provider.PoolStats{}
	}
	return reply.Stats
}

func (p *RPCConnectionPool) Close() error {
	return p.client.Close()
}

type RPCConnection struct {
	client *rpc.Client
	connID int64
}

func (c *RPCConnection) CreateNode(ctx context.Context, node provider.Node) error {
	var reply ConnCreateNodeReply
	return invokeRPC(c.client, "GraphRPC.ConnCreateNode", ConnCreateNodeArgs{TargetID: c.connID, Node: node}, &reply, &reply.ErrStr)
}

func (c *RPCConnection) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	var reply ConnGetNodeReply
	if err := invokeRPC(c.client, "GraphRPC.ConnGetNode", ConnGetNodeArgs{TargetID: c.connID, Id: id, Labels: labels}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCConnection) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	var reply ConnUpdateNodeReply
	return invokeRPC(c.client, "GraphRPC.ConnUpdateNode", ConnUpdateNodeArgs{TargetID: c.connID, Id: id, Updates: updates}, &reply, &reply.ErrStr)
}

func (c *RPCConnection) DeleteNode(ctx context.Context, id string, labels []string) error {
	var reply ConnDeleteNodeReply
	return invokeRPC(c.client, "GraphRPC.ConnDeleteNode", ConnDeleteNodeArgs{TargetID: c.connID, Id: id, Labels: labels}, &reply, &reply.ErrStr)
}

func (c *RPCConnection) ListNodes(ctx context.Context, filter provider.NodeFilter) ([]*provider.Node, error) {
	var reply ConnListNodesReply
	if err := invokeRPC(c.client, "GraphRPC.ConnListNodes", ConnListNodesArgs{TargetID: c.connID, Filter: filter}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCConnection) CreateEdge(ctx context.Context, edge provider.Edge) error {
	var reply ConnCreateEdgeReply
	return invokeRPC(c.client, "GraphRPC.ConnCreateEdge", ConnCreateEdgeArgs{TargetID: c.connID, Edge: edge}, &reply, &reply.ErrStr)
}

func (c *RPCConnection) GetEdge(ctx context.Context, fromID string, toID string, edgeType string) (*provider.Edge, error) {
	var reply ConnGetEdgeReply
	if err := invokeRPC(c.client, "GraphRPC.ConnGetEdge", ConnGetEdgeArgs{TargetID: c.connID, Fromid: fromID, Toid: toID, Edgetype: edgeType}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCConnection) UpdateEdge(ctx context.Context, fromID string, toID string, edgeType string, updates provider.EdgeUpdates) error {
	var reply ConnUpdateEdgeReply
	return invokeRPC(c.client, "GraphRPC.ConnUpdateEdge", ConnUpdateEdgeArgs{TargetID: c.connID, Fromid: fromID, Toid: toID, Edgetype: edgeType, Updates: updates}, &reply, &reply.ErrStr)
}

func (c *RPCConnection) DeleteEdge(ctx context.Context, fromID string, toID string, edgeType string) error {
	var reply ConnDeleteEdgeReply
	return invokeRPC(c.client, "GraphRPC.ConnDeleteEdge", ConnDeleteEdgeArgs{TargetID: c.connID, Fromid: fromID, Toid: toID, Edgetype: edgeType}, &reply, &reply.ErrStr)
}

func (c *RPCConnection) ListEdges(ctx context.Context, filter provider.EdgeFilter) ([]*provider.Edge, error) {
	var reply ConnListEdgesReply
	if err := invokeRPC(c.client, "GraphRPC.ConnListEdges", ConnListEdgesArgs{TargetID: c.connID, Filter: filter}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCConnection) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	var reply ConnExecuteQueryReply
	if err := invokeRPC(c.client, "GraphRPC.ConnExecuteQuery", ConnExecuteQueryArgs{TargetID: c.connID, Query: query}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCConnection) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	var reply ConnExecuteVectorQueryReply
	if err := invokeRPC(c.client, "GraphRPC.ConnExecuteVectorQuery", ConnExecuteVectorQueryArgs{TargetID: c.connID, Query: query}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCConnection) ExecuteTraversal(ctx context.Context, traversal provider.TraversalQuery) (*provider.QueryResult, error) {
	var reply ConnExecuteTraversalReply
	if err := invokeRPC(c.client, "GraphRPC.ConnExecuteTraversal", ConnExecuteTraversalArgs{TargetID: c.connID, Traversal: traversal}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCConnection) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	var reply ConnBeginTransactionReply
	if err := invokeRPC(c.client, "GraphRPC.ConnBeginTransaction", ConnBeginTransactionArgs{TargetID: c.connID}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return &RPCTransaction{client: c.client, txID: reply.Val}, nil
}

func (c *RPCConnection) BeginNestedTransaction(ctx context.Context, parent provider.GraphTransaction) (provider.GraphTransaction, error) {
	var reply ConnBeginNestedTransactionReply
	if err := invokeRPC(c.client, "GraphRPC.ConnBeginNestedTransaction", ConnBeginNestedTransactionArgs{TargetID: c.connID, Parent: parent.(*RPCTransaction).txID}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return &RPCTransaction{client: c.client, txID: reply.Val}, nil
}

func (c *RPCConnection) HasOpenTransaction() bool {
	var reply ConnHasOpenTransactionReply
	err := c.client.Call("GraphRPC.ConnHasOpenTransaction", ConnHasOpenTransactionArgs{TargetID: c.connID}, &reply)
	if err != nil || reply.ErrStr != "" {
		return false
	}
	return reply.Val
}

func (c *RPCConnection) GetOpenTransaction() provider.GraphTransaction {
	var reply ConnGetOpenTransactionReply
	err := c.client.Call("GraphRPC.ConnGetOpenTransaction", ConnGetOpenTransactionArgs{TargetID: c.connID}, &reply)
	if err != nil || reply.ErrStr != "" {
		return nil
	}
	return &RPCTransaction{client: c.client, txID: reply.Val}
}

func (c *RPCConnection) HealthCheck(ctx context.Context) error {
	var reply ConnHealthCheckReply
	return invokeRPC(c.client, "GraphRPC.ConnHealthCheck", ConnHealthCheckArgs{TargetID: c.connID}, &reply, &reply.ErrStr)
}

func (c *RPCConnection) Close() error {
	var reply ConnCloseReply
	return invokeRPC(c.client, "GraphRPC.ConnClose", ConnCloseArgs{TargetID: c.connID}, &reply, &reply.ErrStr)
}

func (c *RPCConnection) ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	var reply ConnExecuteBatchReply
	if err := invokeRPC(c.client, "GraphRPC.ConnExecuteBatch", ConnExecuteBatchArgs{TargetID: c.connID, Operations: operations}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

type RPCTransaction struct {
	client *rpc.Client
	txID   int64
}

func (c *RPCTransaction) CreateNode(ctx context.Context, node provider.Node) error {
	var reply TxCreateNodeReply
	return invokeRPC(c.client, "GraphRPC.TxCreateNode", TxCreateNodeArgs{TargetID: c.txID, Node: node}, &reply, &reply.ErrStr)
}

func (c *RPCTransaction) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	var reply TxGetNodeReply
	if err := invokeRPC(c.client, "GraphRPC.TxGetNode", TxGetNodeArgs{TargetID: c.txID, Id: id, Labels: labels}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCTransaction) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	var reply TxUpdateNodeReply
	return invokeRPC(c.client, "GraphRPC.TxUpdateNode", TxUpdateNodeArgs{TargetID: c.txID, Id: id, Updates: updates}, &reply, &reply.ErrStr)
}

func (c *RPCTransaction) DeleteNode(ctx context.Context, id string, labels []string) error {
	var reply TxDeleteNodeReply
	return invokeRPC(c.client, "GraphRPC.TxDeleteNode", TxDeleteNodeArgs{TargetID: c.txID, Id: id, Labels: labels}, &reply, &reply.ErrStr)
}

func (c *RPCTransaction) CreateEdge(ctx context.Context, edge provider.Edge) error {
	var reply TxCreateEdgeReply
	return invokeRPC(c.client, "GraphRPC.TxCreateEdge", TxCreateEdgeArgs{TargetID: c.txID, Edge: edge}, &reply, &reply.ErrStr)
}

func (c *RPCTransaction) GetEdge(ctx context.Context, fromID string, toID string, edgeType string) (*provider.Edge, error) {
	var reply TxGetEdgeReply
	if err := invokeRPC(c.client, "GraphRPC.TxGetEdge", TxGetEdgeArgs{TargetID: c.txID, Fromid: fromID, Toid: toID, Edgetype: edgeType}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCTransaction) UpdateEdge(ctx context.Context, fromID string, toID string, edgeType string, updates provider.EdgeUpdates) error {
	var reply TxUpdateEdgeReply
	return invokeRPC(c.client, "GraphRPC.TxUpdateEdge", TxUpdateEdgeArgs{TargetID: c.txID, Fromid: fromID, Toid: toID, Edgetype: edgeType, Updates: updates}, &reply, &reply.ErrStr)
}

func (c *RPCTransaction) DeleteEdge(ctx context.Context, fromID string, toID string, edgeType string) error {
	var reply TxDeleteEdgeReply
	return invokeRPC(c.client, "GraphRPC.TxDeleteEdge", TxDeleteEdgeArgs{TargetID: c.txID, Fromid: fromID, Toid: toID, Edgetype: edgeType}, &reply, &reply.ErrStr)
}

func (c *RPCTransaction) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	var reply TxExecuteQueryReply
	if err := invokeRPC(c.client, "GraphRPC.TxExecuteQuery", TxExecuteQueryArgs{TargetID: c.txID, Query: query}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return reply.Val, nil
}

func (c *RPCTransaction) BeginNestedTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	var reply TxBeginNestedTransactionReply
	if err := invokeRPC(c.client, "GraphRPC.TxBeginNestedTransaction", TxBeginNestedTransactionArgs{TargetID: c.txID}, &reply, &reply.ErrStr); err != nil {
		return nil, err
	}
	return &RPCTransaction{client: c.client, txID: reply.Val}, nil
}

func (c *RPCTransaction) SupportsNestedTransactions() bool {
	var reply TxSupportsNestedTransactionsReply
	err := c.client.Call("GraphRPC.TxSupportsNestedTransactions", TxSupportsNestedTransactionsArgs{TargetID: c.txID}, &reply)
	if err != nil || reply.ErrStr != "" {
		return false
	}
	return reply.Val
}

func (c *RPCTransaction) Commit(ctx context.Context) error {
	var reply TxCommitReply
	return invokeRPC(c.client, "GraphRPC.TxCommit", TxCommitArgs{TargetID: c.txID}, &reply, &reply.ErrStr)
}

func (c *RPCTransaction) Rollback(ctx context.Context) error {
	var reply TxRollbackReply
	return invokeRPC(c.client, "GraphRPC.TxRollback", TxRollbackArgs{TargetID: c.txID}, &reply, &reply.ErrStr)
}

func (c *RPCTransaction) IsCommitted() bool {
	var reply TxIsCommittedReply
	err := c.client.Call("GraphRPC.TxIsCommitted", TxIsCommittedArgs{TargetID: c.txID}, &reply)
	if err != nil || reply.ErrStr != "" {
		return false
	}
	return reply.Val
}

func (c *RPCTransaction) IsRolledBack() bool {
	var reply TxIsRolledBackReply
	err := c.client.Call("GraphRPC.TxIsRolledBack", TxIsRolledBackArgs{TargetID: c.txID}, &reply)
	if err != nil || reply.ErrStr != "" {
		return false
	}
	return reply.Val
}

func (c *RPCTransaction) GetParent() provider.GraphTransaction {
	var reply TxGetParentReply
	err := c.client.Call("GraphRPC.TxGetParent", TxGetParentArgs{TargetID: c.txID}, &reply)
	if err != nil || reply.ErrStr != "" {
		return nil
	}
	return &RPCTransaction{client: c.client, txID: reply.Val}
}

// --- Server ---

type GraphRPC struct {
	pool   provider.ConnectionPool
	conns  map[int64]provider.GraphConnection
	txs    map[int64]provider.GraphTransaction
	mu     sync.Mutex
	nextID int64
}

func NewGraphRPC(pool provider.ConnectionPool) *GraphRPC {
	return &GraphRPC{
		pool:  pool,
		conns: make(map[int64]provider.GraphConnection),
		txs:   make(map[int64]provider.GraphTransaction),
	}
}

func (s *GraphRPC) GetConnection(args *GetConnectionArgs, reply *GetConnectionReply) error {
	conn, err := s.pool.GetConnection(context.Background())
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.ConnID = s.storeConn(conn)
	return nil
}

func (s *GraphRPC) ReturnConnection(args *ReturnConnectionArgs, reply *ReturnConnectionReply) error {
	s.mu.Lock()
	conn := s.conns[args.ConnID]
	delete(s.conns, args.ConnID)
	s.mu.Unlock()
	if conn != nil {
		if err := s.pool.ReturnConnection(conn); err != nil {
			reply.ErrStr = err.Error()
		}
	}
	return nil
}

func (s *GraphRPC) Stats(args *StatsArgs, reply *StatsReply) error {
	reply.Stats = s.pool.Stats()
	return nil
}

func (s *GraphRPC) registerSlot(registerFn func(id int64)) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	registerFn(s.nextID)
	return s.nextID
}

func (s *GraphRPC) storeConn(conn provider.GraphConnection) int64 {
	return s.registerSlot(func(id int64) { s.conns[id] = conn })
}

func (s *GraphRPC) storeTx(tx provider.GraphTransaction) int64 {
	return s.registerSlot(func(id int64) { s.txs[id] = tx })
}

func (s *GraphRPC) recordTx(replyVal *int64, tx provider.GraphTransaction, err error) error {
	if err != nil {
		return err
	}
	*replyVal = s.storeTx(tx)
	return nil
}

func (s *GraphRPC) deleteTx(targetID int64) {
	s.mu.Lock()
	delete(s.txs, targetID)
	s.mu.Unlock()
}

func (s *GraphRPC) getConn(targetID int64) (provider.GraphConnection, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target := s.conns[targetID]
	if target == nil {
		return nil, "target not found"
	}
	return target, ""
}

func (s *GraphRPC) getTx(targetID int64) (provider.GraphTransaction, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target := s.txs[targetID]
	if target == nil {
		return nil, "target not found"
	}
	return target, ""
}

func (s *GraphRPC) execConn(targetID int64, replyErr *string, fn func(conn provider.GraphConnection) error) error {
	conn, errStr := s.getConn(targetID)
	if errStr != "" {
		*replyErr = errStr
		return nil
	}
	if err := fn(conn); err != nil {
		*replyErr = err.Error()
	}
	return nil
}

func (s *GraphRPC) execTx(targetID int64, replyErr *string, fn func(tx provider.GraphTransaction) error) error {
	tx, errStr := s.getTx(targetID)
	if errStr != "" {
		*replyErr = errStr
		return nil
	}
	if err := fn(tx); err != nil {
		*replyErr = err.Error()
	}
	return nil
}

func (s *GraphRPC) closeTx(targetID int64, replyErr *string, fn func(tx provider.GraphTransaction) error) error {
	return s.execTx(targetID, replyErr, func(tx provider.GraphTransaction) error {
		if err := fn(tx); err != nil {
			return err
		}
		s.deleteTx(targetID)
		return nil
	})
}

func (s *GraphRPC) ConnCreateNode(args *ConnCreateNodeArgs, reply *ConnCreateNodeReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		return c.CreateNode(context.Background(), args.Node)
	})
}

func (s *GraphRPC) ConnGetNode(args *ConnGetNodeArgs, reply *ConnGetNodeReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) (err error) {
		reply.Val, err = c.GetNode(context.Background(), args.Id, args.Labels)
		return err
	})
}

func (s *GraphRPC) ConnUpdateNode(args *ConnUpdateNodeArgs, reply *ConnUpdateNodeReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		return c.UpdateNode(context.Background(), args.Id, args.Updates)
	})
}

func (s *GraphRPC) ConnDeleteNode(args *ConnDeleteNodeArgs, reply *ConnDeleteNodeReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		return c.DeleteNode(context.Background(), args.Id, args.Labels)
	})
}

func (s *GraphRPC) ConnListNodes(args *ConnListNodesArgs, reply *ConnListNodesReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) (err error) {
		reply.Val, err = c.ListNodes(context.Background(), args.Filter)
		return err
	})
}

func (s *GraphRPC) ConnCreateEdge(args *ConnCreateEdgeArgs, reply *ConnCreateEdgeReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		return c.CreateEdge(context.Background(), args.Edge)
	})
}

func (s *GraphRPC) ConnGetEdge(args *ConnGetEdgeArgs, reply *ConnGetEdgeReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) (err error) {
		reply.Val, err = c.GetEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype)
		return err
	})
}

func (s *GraphRPC) ConnUpdateEdge(args *ConnUpdateEdgeArgs, reply *ConnUpdateEdgeReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		return c.UpdateEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype, args.Updates)
	})
}

func (s *GraphRPC) ConnDeleteEdge(args *ConnDeleteEdgeArgs, reply *ConnDeleteEdgeReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		return c.DeleteEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype)
	})
}

func (s *GraphRPC) ConnListEdges(args *ConnListEdgesArgs, reply *ConnListEdgesReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) (err error) {
		reply.Val, err = c.ListEdges(context.Background(), args.Filter)
		return err
	})
}

func (s *GraphRPC) ConnExecuteQuery(args *ConnExecuteQueryArgs, reply *ConnExecuteQueryReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) (err error) {
		reply.Val, err = c.ExecuteQuery(context.Background(), args.Query)
		return err
	})
}

func (s *GraphRPC) ConnExecuteVectorQuery(args *ConnExecuteVectorQueryArgs, reply *ConnExecuteVectorQueryReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) (err error) {
		reply.Val, err = c.ExecuteVectorQuery(context.Background(), args.Query)
		return err
	})
}

func (s *GraphRPC) ConnExecuteTraversal(args *ConnExecuteTraversalArgs, reply *ConnExecuteTraversalReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) (err error) {
		reply.Val, err = c.ExecuteTraversal(context.Background(), args.Traversal)
		return err
	})
}

func (s *GraphRPC) ConnBeginTransaction(args *ConnBeginTransactionArgs, reply *ConnBeginTransactionReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		tx, err := c.BeginTransaction(context.Background())
		return s.recordTx(&reply.Val, tx, err)
	})
}

func (s *GraphRPC) ConnBeginNestedTransaction(args *ConnBeginNestedTransactionArgs, reply *ConnBeginNestedTransactionReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		s.mu.Lock()
		argTx := s.txs[args.Parent]
		s.mu.Unlock()
		tx, err := c.BeginNestedTransaction(context.Background(), argTx)
		return s.recordTx(&reply.Val, tx, err)
	})
}

func (s *GraphRPC) ConnHasOpenTransaction(args *ConnHasOpenTransactionArgs, reply *ConnHasOpenTransactionReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		reply.Val = c.HasOpenTransaction()
		return nil
	})
}

func (s *GraphRPC) ConnGetOpenTransaction(args *ConnGetOpenTransactionArgs, reply *ConnGetOpenTransactionReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		reply.Val = s.storeTx(c.GetOpenTransaction())
		return nil
	})
}

func (s *GraphRPC) ConnHealthCheck(args *ConnHealthCheckArgs, reply *ConnHealthCheckReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		return c.HealthCheck(context.Background())
	})
}

func (s *GraphRPC) ConnClose(args *ConnCloseArgs, reply *ConnCloseReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) error {
		return c.Close()
	})
}

func (s *GraphRPC) ConnExecuteBatch(args *ConnExecuteBatchArgs, reply *ConnExecuteBatchReply) error {
	return s.execConn(args.TargetID, &reply.ErrStr, func(c provider.GraphConnection) (err error) {
		reply.Val, err = c.ExecuteBatch(context.Background(), args.Operations)
		return err
	})
}

func (s *GraphRPC) TxCreateNode(args *TxCreateNodeArgs, reply *TxCreateNodeReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		return t.CreateNode(context.Background(), args.Node)
	})
}

func (s *GraphRPC) TxGetNode(args *TxGetNodeArgs, reply *TxGetNodeReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) (err error) {
		reply.Val, err = t.GetNode(context.Background(), args.Id, args.Labels)
		return err
	})
}

func (s *GraphRPC) TxUpdateNode(args *TxUpdateNodeArgs, reply *TxUpdateNodeReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		return t.UpdateNode(context.Background(), args.Id, args.Updates)
	})
}

func (s *GraphRPC) TxDeleteNode(args *TxDeleteNodeArgs, reply *TxDeleteNodeReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		return t.DeleteNode(context.Background(), args.Id, args.Labels)
	})
}

func (s *GraphRPC) TxCreateEdge(args *TxCreateEdgeArgs, reply *TxCreateEdgeReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		return t.CreateEdge(context.Background(), args.Edge)
	})
}

func (s *GraphRPC) TxGetEdge(args *TxGetEdgeArgs, reply *TxGetEdgeReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) (err error) {
		reply.Val, err = t.GetEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype)
		return err
	})
}

func (s *GraphRPC) TxUpdateEdge(args *TxUpdateEdgeArgs, reply *TxUpdateEdgeReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		return t.UpdateEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype, args.Updates)
	})
}

func (s *GraphRPC) TxDeleteEdge(args *TxDeleteEdgeArgs, reply *TxDeleteEdgeReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		return t.DeleteEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype)
	})
}

func (s *GraphRPC) TxExecuteQuery(args *TxExecuteQueryArgs, reply *TxExecuteQueryReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) (err error) {
		reply.Val, err = t.ExecuteQuery(context.Background(), args.Query)
		return err
	})
}

func (s *GraphRPC) TxBeginNestedTransaction(args *TxBeginNestedTransactionArgs, reply *TxBeginNestedTransactionReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		tx, err := t.BeginNestedTransaction(context.Background())
		return s.recordTx(&reply.Val, tx, err)
	})
}

func (s *GraphRPC) TxSupportsNestedTransactions(args *TxSupportsNestedTransactionsArgs, reply *TxSupportsNestedTransactionsReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		reply.Val = t.SupportsNestedTransactions()
		return nil
	})
}

func (s *GraphRPC) TxCommit(args *TxCommitArgs, reply *TxCommitReply) error {
	return s.closeTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		return t.Commit(context.Background())
	})
}

func (s *GraphRPC) TxRollback(args *TxRollbackArgs, reply *TxRollbackReply) error {
	return s.closeTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		return t.Rollback(context.Background())
	})
}

func (s *GraphRPC) TxIsCommitted(args *TxIsCommittedArgs, reply *TxIsCommittedReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		reply.Val = t.IsCommitted()
		return nil
	})
}

func (s *GraphRPC) TxIsRolledBack(args *TxIsRolledBackArgs, reply *TxIsRolledBackReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		reply.Val = t.IsRolledBack()
		return nil
	})
}

func (s *GraphRPC) TxGetParent(args *TxGetParentArgs, reply *TxGetParentReply) error {
	return s.execTx(args.TargetID, &reply.ErrStr, func(t provider.GraphTransaction) error {
		reply.Val = s.storeTx(t.GetParent())
		return nil
	})
}

