package memgraph

import (
	"context"
	"encoding/json"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/httpheaders"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestQueryHelpers_FormattingAndSanitization(t *testing.T) {
	// 1. buildLabelFilter
	if f := buildLabelFilter(nil); f != "" {
		t.Fatalf("expected empty label filter, got %q", f)
	}
	if f := buildLabelFilter([]string{"Node", "", "Active"}); f != ":Node:Active" {
		t.Fatalf("unexpected label filter: %q", f)
	}

	// 2. matchNodeByID with and without namespace
	ctx := context.Background()
	q1, p1 := matchNodeByID(ctx, "ID1", []string{"Node"})
	if !strings.Contains(q1, "MATCH (n:Node {id: $id})") || p1["id"] != "ID1" {
		t.Fatalf("unexpected matchNodeByID without ns: %s, %v", q1, p1)
	}
	nsCtx := pkgctx.WithSecurityContext(ctx, &pkgctx.SecurityContext{NamespaceID: "ns-123"})
	q2, p2 := matchNodeByID(nsCtx, "ID1", []string{"Node"})
	if !strings.Contains(q2, "namespace_id") || p2["namespace_id"] != "ns-123" {
		t.Fatalf("unexpected matchNodeByID with ns: %s, %v", q2, p2)
	}

	// 3. matchEdge
	qEdge, pEdge := matchEdge("code_entity:A", "source_file:B", "CALLS")
	if !strings.Contains(qEdge, ":CodeEntity") || !strings.Contains(qEdge, ":SourceFile") || pEdge["fromID"] != "code_entity:A" {
		t.Fatalf("unexpected matchEdge: %s, %v", qEdge, pEdge)
	}

	// 4. applyLimitOffset & appendCypherWhere
	qLimit := applyLimitOffset("MATCH (n)", 10, 5)
	if !strings.Contains(qLimit, "LIMIT 10") || !strings.Contains(qLimit, "SKIP 5") {
		t.Fatalf("unexpected applyLimitOffset: %s", qLimit)
	}
	if appendCypherWhere("MATCH (n)", nil) != "MATCH (n)" {
		t.Fatal("expected unchanged query for nil where")
	}
	qWhere := appendCypherWhere("MATCH (n)", []string{"n.x = 1", "n.y = 2"})
	if !strings.Contains(qWhere, " WHERE n.x = 1 AND n.y = 2") {
		t.Fatalf("unexpected appendCypherWhere: %s", qWhere)
	}
}

func TestNodeLabelAndParamKeyHelpers(t *testing.T) {
	// nodeLabelFromID
	if l := nodeLabelFromID("code_entity:foo"); l != ":CodeEntity" {
		t.Errorf("unexpected nodeLabelFromID: %s", l)
	}
	if l := nodeLabelFromID("source_file:bar"); l != ":SourceFile" {
		t.Errorf("unexpected nodeLabelFromID: %s", l)
	}
	if l := nodeLabelFromID("package:baz"); l != ":Package" {
		t.Errorf("unexpected nodeLabelFromID: %s", l)
	}
	if l := nodeLabelFromID("other:xyz"); l != "" {
		t.Errorf("unexpected nodeLabelFromID: %s", l)
	}

	// sanitizeParamKey & makeParamKey
	if s := sanitizeParamKey("user.name-key_1"); s != "user_name_key_1" {
		t.Errorf("unexpected sanitizeParamKey: %s", s)
	}
	if m := makeParamKey("prop.title"); m != "prop_prop_title" {
		t.Errorf("unexpected makeParamKey: %s", m)
	}

	// safeCypher
	if out := safeCypher("CREATE (n:%s)", "ValidLabel"); out != "CREATE (n:ValidLabel)" {
		t.Errorf("unexpected safeCypher: %s", out)
	}
}

func TestNeo4jConversions_AndRecordParsing(t *testing.T) {
	// 1. convertNeo4jNode with ID in props
	n1 := neo4j.Node{
		ElementId: "elem-1",
		Labels:    []string{"User"},
		Props: map[string]any{
			"id":   "user-100",
			"name": "Alice",
		},
	}
	node1 := convertNeo4jNode(n1)
	if node1.ID != "user-100" || node1.Properties["name"] != "Alice" {
		t.Fatalf("unexpected convertNeo4jNode: %+v", node1)
	}

	// 2. convertNeo4jNode fallback to ElementId
	n2 := neo4j.Node{
		ElementId: "elem-2",
		Labels:    []string{"Role"},
		Props:     map[string]any{},
	}
	node2 := convertNeo4jNode(n2)
	if node2.ID != "elem-2" {
		t.Fatalf("expected fallback ID elem-2, got %s", node2.ID)
	}

	// 3. convertNeo4jRelationship
	rel := neo4j.Relationship{
		Type:  "MEMBER_OF",
		Props: map[string]any{"weight": 1.5},
	}
	edge := convertNeo4jRelationship(rel, "u1", "r1")
	if edge.FromID != "u1" || edge.ToID != "r1" || edge.Type != "MEMBER_OF" || edge.Properties["weight"] != 1.5 {
		t.Fatalf("unexpected convertNeo4jRelationship: %+v", edge)
	}

	// 4. parseSingleNode
	if n, err := parseSingleNode(nil); n != nil || err != nil {
		t.Fatalf("expected nil for empty records, got %v, %v", n, err)
	}
	recInvalid := &neo4j.Record{Values: []any{"not-a-node"}}
	if _, err := parseSingleNode([]*neo4j.Record{recInvalid}); err == nil {
		t.Fatal("expected error for invalid node format")
	}

	// 5. parseSingleEdge
	if e, err := parseSingleEdge(nil, "a", "b"); e != nil || err != nil {
		t.Fatalf("expected nil for empty records, got %v, %v", e, err)
	}
	recRel := &neo4j.Record{Values: []any{rel, "from-rec", "to-rec"}}
	edgeRec, err := parseSingleEdge([]*neo4j.Record{recRel}, "fallbackA", "fallbackB")
	if err != nil || edgeRec.FromID != "from-rec" || edgeRec.ToID != "to-rec" {
		t.Fatalf("unexpected parseSingleEdge result: %+v, %v", edgeRec, err)
	}
}

type mockConnection struct {
	provider.GraphConnection
	called map[string]bool
}

func newMockConnection() *mockConnection {
	return &mockConnection{called: make(map[string]bool)}
}

func (m *mockConnection) CreateNode(ctx context.Context, node provider.Node) error {
	m.called["CreateNode"] = true
	return nil
}
func (m *mockConnection) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	m.called["GetNode"] = true
	return &provider.Node{ID: id}, nil
}
func (m *mockConnection) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	m.called["UpdateNode"] = true
	return nil
}
func (m *mockConnection) DeleteNode(ctx context.Context, id string, labels []string) error {
	m.called["DeleteNode"] = true
	return nil
}
func (m *mockConnection) ListNodes(ctx context.Context, filter provider.NodeFilter) ([]*provider.Node, error) {
	m.called["ListNodes"] = true
	return nil, nil
}
func (m *mockConnection) CreateEdge(ctx context.Context, edge provider.Edge) error {
	m.called["CreateEdge"] = true
	return nil
}
func (m *mockConnection) GetEdge(ctx context.Context, fromID, toID, edgeType string) (*provider.Edge, error) {
	m.called["GetEdge"] = true
	return &provider.Edge{FromID: fromID, ToID: toID}, nil
}
func (m *mockConnection) UpdateEdge(ctx context.Context, fromID, toID, edgeType string, updates provider.EdgeUpdates) error {
	m.called["UpdateEdge"] = true
	return nil
}
func (m *mockConnection) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	m.called["DeleteEdge"] = true
	return nil
}
func (m *mockConnection) ListEdges(ctx context.Context, filter provider.EdgeFilter) ([]*provider.Edge, error) {
	m.called["ListEdges"] = true
	return nil, nil
}
func (m *mockConnection) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	m.called["ExecuteQuery"] = true
	return &provider.QueryResult{}, nil
}
func (m *mockConnection) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	m.called["ExecuteVectorQuery"] = true
	return &provider.QueryResult{}, nil
}
func (m *mockConnection) ExecuteTraversal(ctx context.Context, query provider.TraversalQuery) (*provider.QueryResult, error) {
	m.called["ExecuteTraversal"] = true
	return &provider.QueryResult{}, nil
}
func (m *mockConnection) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	m.called["BeginTransaction"] = true
	return nil, nil
}
func (m *mockConnection) BeginNestedTransaction(ctx context.Context, parent provider.GraphTransaction) (provider.GraphTransaction, error) {
	m.called["BeginNestedTransaction"] = true
	return nil, nil
}
func (m *mockConnection) HasOpenTransaction() bool {
	m.called["HasOpenTransaction"] = true
	return false
}
func (m *mockConnection) GetOpenTransaction() provider.GraphTransaction {
	m.called["GetOpenTransaction"] = true
	return nil
}
func (m *mockConnection) HealthCheck(ctx context.Context) error {
	m.called["HealthCheck"] = true
	return nil
}
func (m *mockConnection) ExecuteBatch(ctx context.Context, ops []provider.Operation) (*provider.BatchResult, error) {
	m.called["ExecuteBatch"] = true
	return &provider.BatchResult{}, nil
}

type mockPool struct {
	provider.ConnectionPool
	returned bool
	closed   bool
}

func (p *mockPool) ReturnConnection(conn provider.GraphConnection) error {
	p.returned = true
	return nil
}
func (p *mockPool) Close() error {
	p.closed = true
	return nil
}

func TestSingleConnectionWrapper_DelegatesAllMethods(t *testing.T) {
	mock := newMockConnection()
	pool := &mockPool{}
	w := &singleConnectionWrapper{conn: mock, pool: pool}
	ctx := context.Background()

	_ = w.CreateNode(ctx, provider.Node{ID: "N1"})
	_, _ = w.GetNode(ctx, "N1", nil)
	_ = w.UpdateNode(ctx, "N1", provider.NodeUpdates{})
	_ = w.DeleteNode(ctx, "N1", nil)
	_, _ = w.ListNodes(ctx, provider.NodeFilter{})

	_ = w.CreateEdge(ctx, provider.Edge{FromID: "A", ToID: "B"})
	_, _ = w.GetEdge(ctx, "A", "B", "REL")
	_ = w.UpdateEdge(ctx, "A", "B", "REL", provider.EdgeUpdates{})
	_ = w.DeleteEdge(ctx, "A", "B", "REL")
	_, _ = w.ListEdges(ctx, provider.EdgeFilter{})

	_, _ = w.ExecuteQuery(ctx, provider.Query{})
	_, _ = w.ExecuteVectorQuery(ctx, provider.VectorQuery{})
	_, _ = w.ExecuteTraversal(ctx, provider.TraversalQuery{})
	_, _ = w.BeginTransaction(ctx)
	_, _ = w.BeginNestedTransaction(ctx, nil)
	_ = w.HasOpenTransaction()
	_ = w.GetOpenTransaction()
	_ = w.HealthCheck(ctx)
	_, _ = w.ExecuteBatch(ctx, nil)

	if err := w.Close(); err != nil || !pool.returned || !pool.closed {
		t.Fatalf("expected ReturnConnection and Close on pool, got err=%v", err)
	}

	expectedMethods := []string{
		"CreateNode", "GetNode", "UpdateNode", "DeleteNode", "ListNodes",
		"CreateEdge", "GetEdge", "UpdateEdge", "DeleteEdge", "ListEdges",
		"ExecuteQuery", "ExecuteVectorQuery", "ExecuteTraversal",
		"BeginTransaction", "BeginNestedTransaction", "HasOpenTransaction",
		"GetOpenTransaction", "HealthCheck", "ExecuteBatch",
	}
	for _, m := range expectedMethods {
		if !mock.called[m] {
			t.Errorf("method %s was not called on underlying connection", m)
		}
	}
}

func TestMemgraphTransaction_UninitializedAndLifecycle(t *testing.T) {
	tx := &memgraphTransaction{}
	ctx := context.Background()

	if err := tx.CreateNode(ctx, provider.Node{ID: "1"}); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}
	if _, err := tx.GetNode(ctx, "1", nil); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}
	if err := tx.UpdateNode(ctx, "1", provider.NodeUpdates{}); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}
	if err := tx.DeleteNode(ctx, "1", nil); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}
	if err := tx.CreateEdge(ctx, provider.Edge{}); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}
	if _, err := tx.GetEdge(ctx, "a", "b", "c"); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}
	if err := tx.UpdateEdge(ctx, "a", "b", "c", provider.EdgeUpdates{}); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}
	if err := tx.DeleteEdge(ctx, "a", "b", "c"); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}
	if _, err := tx.ExecuteQuery(ctx, provider.Query{}); err == nil {
		t.Fatal("expected error on uninitialized tx")
	}

	if tx.SupportsNestedTransactions() {
		t.Fatal("expected SupportsNestedTransactions=false")
	}
	if tx.GetParent() != nil {
		t.Fatal("expected nil parent")
	}
	if tx.IsCommitted() || tx.IsRolledBack() {
		t.Fatal("expected tx not committed or rolled back initially")
	}

	// Commit & Rollback state transitions
	conn := &memgraphConnection{BaseConnection: &provider.BaseConnection{}}
	tx.conn = conn

	// Commit when explicitTx is nil
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("expected error committing uninitialized explicitTx")
	}

	// Commit when already committed
	tx.committed = true
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("expected error committing already committed tx")
	}

	// Rollback when already committed
	if err := tx.Rollback(ctx); err == nil {
		t.Fatal("expected error rolling back committed tx")
	}

	// Rollback when already rolled back is idempotent (returns nil)
	tx.committed = false
	tx.rolledBack = true
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("expected nil on idempotent rollback, got %v", err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("expected error committing already rolled back tx")
	}
}

func TestMemgraphClient_HTTPResponses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cypher" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		q, _ := req["query"].(string)

		if strings.Contains(q, "FAIL_500") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
			return
		}
		if strings.Contains(q, "FAIL_ERRORS") {
			w.Header().Set(httpheaders.ContentType, "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []any{},
				"errors":  []any{map[string]any{"code": "SyntaxError"}},
			})
			return
		}
		w.Header().Set(httpheaders.ContentType, "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"health": 1}},
		})
	}))
	defer ts.Close()

	parts := strings.Split(strings.TrimPrefix(ts.URL, "http://"), ":")
	port, _ := strconv.Atoi(parts[1])

	client := newMemgraphClient(&MemGraphConfig{
		Host: parts[0],
		Port: port,
	})

	ctx := context.Background()
	// Success query
	if _, err := client.executeQuery(ctx, "RETURN 1", nil); err != nil {
		t.Fatalf("executeQuery failed: %v", err)
	}
	// Health check
	if err := client.healthCheck(ctx); err != nil {
		t.Fatalf("healthCheck failed: %v", err)
	}
	// 500 error
	if _, err := client.executeQuery(ctx, "FAIL_500", nil); err == nil {
		t.Fatal("expected error on 500")
	}
	// errors in body
	if _, err := client.executeQuery(ctx, "FAIL_ERRORS", nil); err == nil {
		t.Fatal("expected error on response errors")
	}
}

func TestMemgraphConnection_UninitializedClientErrors(t *testing.T) {
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
	}
	ctx := context.Background()

	// Operations should fail with uninitialized boltClient
	if err := conn.CreateNode(ctx, provider.Node{ID: "1"}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.GetNode(ctx, "1", nil); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if err := conn.UpdateNode(ctx, "1", provider.NodeUpdates{}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if err := conn.DeleteNode(ctx, "1", nil); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.ListNodes(ctx, provider.NodeFilter{}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if err := conn.CreateEdge(ctx, provider.Edge{}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.GetEdge(ctx, "a", "b", "c"); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if err := conn.UpdateEdge(ctx, "a", "b", "c", provider.EdgeUpdates{}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if err := conn.DeleteEdge(ctx, "a", "b", "c"); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.ListEdges(ctx, provider.EdgeFilter{}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.ExecuteQuery(ctx, provider.Query{}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.ExecuteVectorQuery(ctx, provider.VectorQuery{}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.ExecuteTraversal(ctx, provider.TraversalQuery{}); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.BeginTransaction(ctx); err == nil {
		t.Fatal("expected error on nil boltClient")
	}
	if _, err := conn.BeginNestedTransaction(ctx, nil); err == nil {
		t.Fatal("expected error on nil boltClient")
	}

	// ExecuteBatch with nil boltClient returns error
	if _, err := conn.ExecuteBatch(ctx, nil); err == nil {
		t.Fatal("expected error on ExecuteBatch with nil boltClient")
	}
	if conn.Close() != nil {
		t.Fatal("expected nil on Close with nil boltClient")
	}
}

func TestMemGraphProvider_FeaturesAndCapabilities(t *testing.T) {
	p := NewMemGraphProvider(&MemGraphConfig{})
	if !p.SupportsFeature(provider.FeatureCypherQuery) {
		t.Error("expected FeatureCypherQuery supported")
	}
	if !p.SupportsFeature(provider.FeatureVectorSearch) {
		t.Error("expected FeatureVectorSearch supported")
	}
	if !p.SupportsFeature(provider.FeatureTransactions) {
		t.Error("expected FeatureTransactions supported")
	}
	if p.SupportsFeature(provider.Feature("non_existent_feature")) {
		t.Error("expected unsupported feature to return false")
	}

	caps := p.GetCapabilities()
	if len(caps.Features) == 0 {
		t.Error("expected non-empty supported features")
	}

	info := p.GetProviderInfo()
	if info.Name != "MemGraph" {
		t.Errorf("unexpected provider info: %+v", info)
	}

	// CreatePool
	ctx := context.Background()
	pool, err := p.CreatePool(ctx, provider.ConnectionConfig{Host: "localhost", Port: 7687, MaxConns: 2})
	if err != nil || pool == nil {
		t.Fatalf("CreatePool failed: %v", err)
	}
	_ = pool.Close()
}

type mockResultWithContext struct {
	neo4j.ResultWithContext
	records []*neo4j.Record
	err     error
}

func (m *mockResultWithContext) Collect(ctx context.Context) ([]*neo4j.Record, error) {
	return m.records, m.err
}

type mockBoltTx struct {
	neo4j.ExplicitTransaction
	records []*neo4j.Record
	runErr  error
}

func (m *mockBoltTx) Run(ctx context.Context, cypher string, params map[string]any) (neo4j.ResultWithContext, error) {
	if m.runErr != nil {
		return nil, m.runErr
	}
	return &mockResultWithContext{records: m.records}, nil
}
func (m *mockBoltTx) Commit(ctx context.Context) error   { return nil }
func (m *mockBoltTx) Rollback(ctx context.Context) error { return nil }

func TestMemgraphTransaction_WithMockExplicitTx(t *testing.T) {
	conn := &memgraphConnection{BaseConnection: &provider.BaseConnection{}}
	tx := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockBoltTx{},
	}
	ctx := context.Background()

	// CreateNode
	if err := tx.CreateNode(ctx, provider.Node{ID: "node-1", Labels: []string{"Item"}, Properties: map[string]any{"k": "v"}}); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// UpdateNode
	updates := provider.NodeUpdates{
		Properties:       map[string]any{"updated": true},
		AddLabels:        []string{"New"},
		RemoveProperties: []string{"old"},
	}
	if err := tx.UpdateNode(ctx, "node-1", updates); err != nil {
		t.Fatalf("UpdateNode failed: %v", err)
	}

	// DeleteNode
	if err := tx.DeleteNode(ctx, "node-1", []string{"Item"}); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}

	// CreateEdge, UpdateEdge, DeleteEdge
	if err := tx.CreateEdge(ctx, provider.Edge{FromID: "n1", ToID: "n2", Type: "REL", Properties: map[string]any{"w": 1}}); err != nil {
		t.Fatalf("CreateEdge failed: %v", err)
	}
	if err := tx.UpdateEdge(ctx, "n1", "n2", "REL", provider.EdgeUpdates{Properties: map[string]any{"w": 2}}); err != nil {
		t.Fatalf("UpdateEdge failed: %v", err)
	}
	if err := tx.DeleteEdge(ctx, "n1", "n2", "REL"); err != nil {
		t.Fatalf("DeleteEdge failed: %v", err)
	}

	// Query
	if _, err := tx.ExecuteQuery(ctx, provider.Query{Query: "RETURN 1", Language: provider.QueryLanguageCypher}); err != nil {
		t.Fatalf("ExecuteQuery failed: %v", err)
	}

	// Commit success
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}

func TestMemgraphConnection_WithMockTxQueryExecution(t *testing.T) {
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	tx := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockBoltTx{},
	}
	conn.SetOpenTransaction(tx)
	ctx := context.Background()

	_ = conn.CreateNode(ctx, provider.Node{ID: "n1", Labels: []string{"L1"}})
	_, _ = conn.GetNode(ctx, "n1", []string{"L1"})
	_ = conn.UpdateNode(ctx, "n1", provider.NodeUpdates{Properties: map[string]any{"p": "v"}})
	_ = conn.DeleteNode(ctx, "n1", nil)
	_, _ = conn.ListNodes(ctx, provider.NodeFilter{Labels: []string{"L1"}, Limit: 5, Offset: 2})

	_ = conn.CreateEdge(ctx, provider.Edge{FromID: "n1", ToID: "n2", Type: "REL"})
	_, _ = conn.GetEdge(ctx, "n1", "n2", "REL")
	_ = conn.UpdateEdge(ctx, "n1", "n2", "REL", provider.EdgeUpdates{})
	_ = conn.DeleteEdge(ctx, "n1", "n2", "REL")
	_, _ = conn.ListEdges(ctx, provider.EdgeFilter{FromID: "n1", Limit: 10})

	_, _ = conn.ExecuteQuery(ctx, provider.Query{Query: "MATCH (n) RETURN n", Language: provider.QueryLanguageCypher})
	_, _ = conn.ExecuteVectorQuery(ctx, provider.VectorQuery{Query: "find sim", Limit: 3})
	_, _ = conn.ExecuteTraversal(ctx, provider.TraversalQuery{StartNodeID: "n1", MaxDepth: 2})
}

type batchMockTx struct {
	provider.GraphTransaction
}

func (b *batchMockTx) CreateNode(ctx context.Context, node provider.Node) error { return nil }
func (b *batchMockTx) CreateEdge(ctx context.Context, edge provider.Edge) error { return nil }
func (b *batchMockTx) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	return nil
}
func (b *batchMockTx) DeleteNode(ctx context.Context, id string, labels []string) error { return nil }
func (b *batchMockTx) UpdateEdge(ctx context.Context, fromID, toID, edgeType string, updates provider.EdgeUpdates) error {
	return nil
}
func (b *batchMockTx) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	return nil
}
func (b *batchMockTx) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	return &provider.QueryResult{}, nil
}
func (b *batchMockTx) Commit(ctx context.Context) error   { return nil }
func (b *batchMockTx) Rollback(ctx context.Context) error { return nil }

func TestMemgraphConnection_ExecuteBatchDetailed(t *testing.T) {
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	mtx := &batchMockTx{}
	conn.SetOpenTransaction(mtx)
	ctx := context.Background()

	ops := []provider.Operation{
		// Chunked create_node
		{Type: "create_node", Data: provider.Node{ID: "b-1", Labels: []string{"B"}}},
		{Type: "create_node", Data: provider.Node{ID: "b-2", Labels: []string{"B"}}},
		// Chunked create_edge
		{Type: "create_edge", Data: provider.Edge{FromID: "source_file:1", ToID: "package:2", Type: "REL"}},
		{Type: "create_edge", Data: provider.Edge{FromID: "source_file:3", ToID: "package:4", Type: "REL"}},
		// Chunked update_node
		{Type: "update_node", Data: map[string]any{"id": "source_file:1", "properties": map[string]any{"p": 1}}},
		{Type: "update_node", Data: map[string]any{"id": "source_file:2", "properties": map[string]any{"p": 2}}},
		// Fallbacks
		{Type: "delete_node", Data: map[string]any{"id": "b-1"}},
		{Type: "update_edge", Data: map[string]any{"from_id": "b-1", "to_id": "b-2", "type": "REL"}},
		{Type: "delete_edge", Data: map[string]any{"from_id": "b-1", "to_id": "b-2", "type": "REL"}},
		{Type: "execute_query", Data: provider.Query{Query: "RETURN 1"}},
		{Type: "unknown_op", Data: nil},
	}

	res, err := conn.ExecuteBatch(ctx, ops)
	if err != nil {
		t.Fatalf("ExecuteBatch failed: %v", err)
	}
	if res.FailureCount != 1 {
		t.Fatalf("expected 1 failure for unknown_op, got %d", res.FailureCount)
	}
}

func TestMemgraphClient_DirectCloseAndPing(t *testing.T) {
	cl := &Client{driver: nil}
	ctx := context.Background()

	if err := cl.Close(ctx); err != nil {
		t.Fatalf("Close on nil driver should succeed: %v", err)
	}
	if err := cl.Ping(ctx); err == nil {
		t.Fatal("expected error on Ping with nil driver")
	}
}

func TestMemgraphConnection_NestedTransactions(t *testing.T) {
	conn := &memgraphConnection{boltClient: &boltClient{}}
	ctx := context.Background()

	// Invalid parent
	if _, err := conn.BeginNestedTransaction(ctx, &batchMockTx{}); err == nil {
		t.Fatal("expected error with invalid parent type")
	}

	// Valid parent
	parent := &memgraphTransaction{conn: conn}
	child, err := conn.BeginNestedTransaction(ctx, parent)
	if err != nil || child == nil {
		t.Fatalf("BeginNestedTransaction failed: %v", err)
	}
	if child.GetParent() != parent {
		t.Fatal("expected GetParent to return parent")
	}
	if _, err := child.BeginNestedTransaction(ctx); err == nil {
		t.Fatal("expected error calling BeginNestedTransaction on child")
	}
}

func TestMemgraphTransaction_GetNodeAndEdgeWithRecords(t *testing.T) {
	nodeRec := &neo4j.Record{
		Values: []any{
			neo4j.Node{ElementId: "elem-1", Labels: []string{"Item"}, Props: map[string]any{"id": "n1"}},
		},
	}
	edgeRec := &neo4j.Record{
		Values: []any{
			neo4j.Relationship{Type: "REL", Props: map[string]any{"cost": 10}},
			"n1", "n2",
		},
	}

	tx := &memgraphTransaction{
		conn: &memgraphConnection{BaseConnection: &provider.BaseConnection{}},
		explicitTx: &mockBoltTx{
			records: []*neo4j.Record{nodeRec},
		},
	}
	ctx := context.Background()

	node, err := tx.GetNode(ctx, "n1", []string{"Item"})
	if err != nil || node == nil || node.ID != "n1" {
		t.Fatalf("GetNode failed: %v, %v", node, err)
	}

	tx.explicitTx = &mockBoltTx{records: []*neo4j.Record{edgeRec}}
	edge, err := tx.GetEdge(ctx, "n1", "n2", "REL")
	if err != nil || edge == nil || edge.FromID != "n1" {
		t.Fatalf("GetEdge failed: %v, %v", edge, err)
	}
}

func TestMemgraphConnection_QueryRecordsAndConversions(t *testing.T) {
	nodeRec := &neo4j.Record{
		Values: []any{
			neo4j.Node{ElementId: "elem-1", Labels: []string{"Item"}, Props: map[string]any{"id": "n1"}},
		},
	}
	edgeRec := &neo4j.Record{
		Values: []any{
			neo4j.Relationship{Type: "REL", Props: map[string]any{"cost": 10}},
			"n1", "n2",
		},
	}

	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	tx := &memgraphTransaction{
		conn: conn,
	}
	conn.SetOpenTransaction(tx)
	ctx := context.Background()

	// 1. Node query
	tx.explicitTx = &mockBoltTx{records: []*neo4j.Record{nodeRec}}
	node, err := conn.GetNode(ctx, "n1", []string{"Item"})
	if err != nil || node == nil || node.ID != "n1" {
		t.Fatalf("conn.GetNode failed: %v, %v", node, err)
	}

	// 2. Edge query
	tx.explicitTx = &mockBoltTx{records: []*neo4j.Record{edgeRec}}
	edge, err := conn.GetEdge(ctx, "n1", "n2", "REL")
	if err != nil || edge == nil || edge.FromID != "n1" {
		t.Fatalf("conn.GetEdge failed: %v, %v", edge, err)
	}

	// 3. List queries
	tx.explicitTx = &mockBoltTx{records: []*neo4j.Record{nodeRec}}
	nodes, err := conn.ListNodes(ctx, provider.NodeFilter{Limit: 2})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("conn.ListNodes failed: %v, count=%d", err, len(nodes))
	}

	listEdgeRec := &neo4j.Record{
		Values: []any{
			"n1", "n2", "REL", neo4j.Relationship{Type: "REL", Props: map[string]any{"cost": 10}},
		},
	}
	tx.explicitTx = &mockBoltTx{records: []*neo4j.Record{listEdgeRec}}
	edges, err := conn.ListEdges(ctx, provider.EdgeFilter{Limit: 2})
	if err != nil || len(edges) != 1 {
		t.Fatalf("conn.ListEdges failed: %v, count=%d", err, len(edges))
	}
}

func TestPool_ExecuteLifecycleAndCancel(t *testing.T) {
	pool, err := NewMemGraphConnectionPool(&MemGraphConfig{Host: "localhost", Port: 7687})
	if err != nil {
		t.Fatalf("NewMemGraphConnectionPool failed: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	_ = pool.Execute(ctx, func(conn provider.GraphConnection) error {
		return nil
	})

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_ = pool.Execute(canceledCtx, func(conn provider.GraphConnection) error {
		return nil
	})

	p := NewMemGraphProvider(&MemGraphConfig{})
	conn, err := p.Connect(ctx, provider.ConnectionConfig{Host: "localhost", Port: 7687})
	if err == nil && conn != nil {
		_ = conn.Close()
	}
}

func TestMemgraphConnection_ExecuteQueryMatrix(t *testing.T) {
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	ctx := context.Background()

	// 1. Unsupported language
	if _, err := conn.ExecuteQuery(ctx, provider.Query{Language: "sparql"}); err == nil {
		t.Fatal("expected error on unsupported language")
	}

	// 2. Successful record parsing
	node := neo4j.Node{ElementId: "e1", Labels: []string{"Node"}, Props: map[string]any{"id": "n1"}}
	rec := &neo4j.Record{
		Keys:   []string{"n"},
		Values: []any{node},
	}
	tx := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockBoltTx{records: []*neo4j.Record{rec}},
	}
	conn.SetOpenTransaction(tx)

	res, err := conn.ExecuteQuery(ctx, provider.Query{Language: provider.QueryLanguageCypher, Query: "MATCH (n) RETURN n"})
	if err != nil || len(res.Nodes) != 1 || res.Nodes[0].ID != "n1" {
		t.Fatalf("unexpected ExecuteQuery result: %v, %v", res, err)
	}
}

func TestMemgraphConnection_VectorAndTraversalMatrix(t *testing.T) {
	node := neo4j.Node{ElementId: "e1", Labels: []string{"VNode"}, Props: map[string]any{"id": "v1"}}
	rec := &neo4j.Record{
		Keys:   []string{"n", "similarity"},
		Values: []any{node, float64(0.95)},
	}
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	tx := &memgraphTransaction{
		conn:       conn,
		explicitTx: &mockBoltTx{records: []*neo4j.Record{rec}},
	}
	conn.SetOpenTransaction(tx)
	ctx := context.Background()

	// 1. Vector query with empty custom query (standard builder)
	vqStandard := provider.VectorQuery{
		Vector:    []float32{0.1, 0.2},
		Threshold: 0.8,
		Limit:     5,
		Filter: provider.NodeFilter{
			Labels:     []string{"VNode"},
			Properties: map[string]any{"active": true},
		},
	}
	resV1, err := conn.ExecuteVectorQuery(ctx, vqStandard)
	if err != nil || len(resV1.Nodes) != 1 {
		t.Fatalf("ExecuteVectorQuery standard failed: %v, %v", resV1, err)
	}

	// 2. Vector query with custom query
	vqCustom := provider.VectorQuery{
		Query:  "MATCH (n) RETURN n, 1.0 AS similarity",
		Filter: provider.NodeFilter{Properties: map[string]any{"foo": "bar"}},
	}
	if _, err := conn.ExecuteVectorQuery(ctx, vqCustom); err != nil {
		t.Fatalf("ExecuteVectorQuery custom failed: %v", err)
	}

	// 3. Traversal query directions
	dirs := []provider.Direction{
		provider.DirectionOutgoing,
		provider.DirectionIncoming,
		provider.DirectionBoth,
		provider.Direction("default"),
	}
	for _, dir := range dirs {
		tq := provider.TraversalQuery{
			StartNodeID:  "v1",
			Relationship: "CALLS",
			Direction:    dir,
			MaxDepth:     3,
			Filter: provider.NodeFilter{
				Labels:     []string{"Dest"},
				Properties: map[string]any{"status": "ok"},
			},
		}
		_, _ = conn.ExecuteTraversal(ctx, tq)
	}
}

func TestMemgraphConnection_CloseAndRollbackBranches(t *testing.T) {
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
	}
	conn.SetOpenTransaction(&batchMockTx{})
	if err := conn.CloseInternal(); err != nil {
		t.Fatalf("CloseInternal failed: %v", err)
	}

	conn2 := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
	}
	conn2.SetOpenTransaction(&batchMockTx{})
	if err := conn2.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
