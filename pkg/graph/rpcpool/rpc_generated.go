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
	defer func() { _ = p.ReturnConnection(conn) }()
	return fn(conn)
}

type StatsArgs struct{}
type StatsReply struct{ Stats provider.PoolStats }

func (p *RPCConnectionPool) Stats() provider.PoolStats {
	var reply StatsReply
	_ = p.client.Call("GraphRPC.Stats", StatsArgs{}, &reply)
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
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.conns[id] = conn
	s.mu.Unlock()
	reply.ConnID = id
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

func (s *GraphRPC) ConnCreateNode(args *ConnCreateNodeArgs, reply *ConnCreateNodeReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.CreateNode(context.Background(), args.Node)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) ConnGetNode(args *ConnGetNodeArgs, reply *ConnGetNodeReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.GetNode(context.Background(), args.Id, args.Labels)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) ConnUpdateNode(args *ConnUpdateNodeArgs, reply *ConnUpdateNodeReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.UpdateNode(context.Background(), args.Id, args.Updates)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) ConnDeleteNode(args *ConnDeleteNodeArgs, reply *ConnDeleteNodeReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.DeleteNode(context.Background(), args.Id, args.Labels)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) ConnListNodes(args *ConnListNodesArgs, reply *ConnListNodesReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.ListNodes(context.Background(), args.Filter)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) ConnCreateEdge(args *ConnCreateEdgeArgs, reply *ConnCreateEdgeReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.CreateEdge(context.Background(), args.Edge)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) ConnGetEdge(args *ConnGetEdgeArgs, reply *ConnGetEdgeReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.GetEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) ConnUpdateEdge(args *ConnUpdateEdgeArgs, reply *ConnUpdateEdgeReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.UpdateEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype, args.Updates)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) ConnDeleteEdge(args *ConnDeleteEdgeArgs, reply *ConnDeleteEdgeReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.DeleteEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) ConnListEdges(args *ConnListEdgesArgs, reply *ConnListEdgesReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.ListEdges(context.Background(), args.Filter)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) ConnExecuteQuery(args *ConnExecuteQueryArgs, reply *ConnExecuteQueryReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.ExecuteQuery(context.Background(), args.Query)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) ConnExecuteVectorQuery(args *ConnExecuteVectorQueryArgs, reply *ConnExecuteVectorQueryReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.ExecuteVectorQuery(context.Background(), args.Query)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) ConnExecuteTraversal(args *ConnExecuteTraversalArgs, reply *ConnExecuteTraversalReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.ExecuteTraversal(context.Background(), args.Traversal)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) ConnBeginTransaction(args *ConnBeginTransactionArgs, reply *ConnBeginTransactionReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.BeginTransaction(context.Background())
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.txs[id] = val
	s.mu.Unlock()
	reply.Val = id
	return nil
}

func (s *GraphRPC) ConnBeginNestedTransaction(args *ConnBeginNestedTransactionArgs, reply *ConnBeginNestedTransactionReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	s.mu.Lock()
	argTx := s.txs[args.Parent]
	s.mu.Unlock()
	val, err := target.BeginNestedTransaction(context.Background(), argTx)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.txs[id] = val
	s.mu.Unlock()
	reply.Val = id
	return nil
}

func (s *GraphRPC) ConnHasOpenTransaction(args *ConnHasOpenTransactionArgs, reply *ConnHasOpenTransactionReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	reply.Val = target.HasOpenTransaction()
	return nil
}

func (s *GraphRPC) ConnGetOpenTransaction(args *ConnGetOpenTransactionArgs, reply *ConnGetOpenTransactionReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val := target.GetOpenTransaction()
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.txs[id] = val
	s.mu.Unlock()
	reply.Val = id
	return nil
}

func (s *GraphRPC) ConnHealthCheck(args *ConnHealthCheckArgs, reply *ConnHealthCheckReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.HealthCheck(context.Background())
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) ConnClose(args *ConnCloseArgs, reply *ConnCloseReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.Close()
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) ConnExecuteBatch(args *ConnExecuteBatchArgs, reply *ConnExecuteBatchReply) error {
	target, errStr := s.getConn(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.ExecuteBatch(context.Background(), args.Operations)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) TxCreateNode(args *TxCreateNodeArgs, reply *TxCreateNodeReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.CreateNode(context.Background(), args.Node)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) TxGetNode(args *TxGetNodeArgs, reply *TxGetNodeReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.GetNode(context.Background(), args.Id, args.Labels)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) TxUpdateNode(args *TxUpdateNodeArgs, reply *TxUpdateNodeReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.UpdateNode(context.Background(), args.Id, args.Updates)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) TxDeleteNode(args *TxDeleteNodeArgs, reply *TxDeleteNodeReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.DeleteNode(context.Background(), args.Id, args.Labels)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) TxCreateEdge(args *TxCreateEdgeArgs, reply *TxCreateEdgeReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.CreateEdge(context.Background(), args.Edge)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) TxGetEdge(args *TxGetEdgeArgs, reply *TxGetEdgeReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.GetEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) TxUpdateEdge(args *TxUpdateEdgeArgs, reply *TxUpdateEdgeReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.UpdateEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype, args.Updates)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) TxDeleteEdge(args *TxDeleteEdgeArgs, reply *TxDeleteEdgeReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.DeleteEdge(context.Background(), args.Fromid, args.Toid, args.Edgetype)
	if err != nil {
		reply.ErrStr = err.Error()
	}
	return nil
}

func (s *GraphRPC) TxExecuteQuery(args *TxExecuteQueryArgs, reply *TxExecuteQueryReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.ExecuteQuery(context.Background(), args.Query)
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	reply.Val = val
	return nil
}

func (s *GraphRPC) TxBeginNestedTransaction(args *TxBeginNestedTransactionArgs, reply *TxBeginNestedTransactionReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val, err := target.BeginNestedTransaction(context.Background())
	if err != nil {
		reply.ErrStr = err.Error()
		return nil
	}
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.txs[id] = val
	s.mu.Unlock()
	reply.Val = id
	return nil
}

func (s *GraphRPC) TxSupportsNestedTransactions(args *TxSupportsNestedTransactionsArgs, reply *TxSupportsNestedTransactionsReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	reply.Val = target.SupportsNestedTransactions()
	return nil
}

func (s *GraphRPC) TxCommit(args *TxCommitArgs, reply *TxCommitReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.Commit(context.Background())
	if err != nil {
		reply.ErrStr = err.Error()
	} else {
		s.mu.Lock()
		delete(s.txs, args.TargetID)
		s.mu.Unlock()
	}
	return nil
}

func (s *GraphRPC) TxRollback(args *TxRollbackArgs, reply *TxRollbackReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	err := target.Rollback(context.Background())
	if err != nil {
		reply.ErrStr = err.Error()
	} else {
		s.mu.Lock()
		delete(s.txs, args.TargetID)
		s.mu.Unlock()
	}
	return nil
}

func (s *GraphRPC) TxIsCommitted(args *TxIsCommittedArgs, reply *TxIsCommittedReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	reply.Val = target.IsCommitted()
	return nil
}

func (s *GraphRPC) TxIsRolledBack(args *TxIsRolledBackArgs, reply *TxIsRolledBackReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	reply.Val = target.IsRolledBack()
	return nil
}

func (s *GraphRPC) TxGetParent(args *TxGetParentArgs, reply *TxGetParentReply) error {
	target, errStr := s.getTx(args.TargetID)
	if errStr != "" {
		reply.ErrStr = errStr
		return nil
	}
	val := target.GetParent()
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.txs[id] = val
	s.mu.Unlock()
	reply.Val = id
	return nil
}
