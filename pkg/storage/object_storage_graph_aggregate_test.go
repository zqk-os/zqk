package storage

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// mockGraphConnectionForAggregate provides a mock graph connection for aggregate tests
type mockGraphConnectionForAggregate struct {
	queryResults map[string]*provider.QueryResult
	queryCalls   []provider.Query
	queryHandler func(ctx context.Context, query provider.Query) (*provider.QueryResult, error)
}

func newMockGraphConnectionForAggregate() *mockGraphConnectionForAggregate {
	return &mockGraphConnectionForAggregate{
		queryResults: make(map[string]*provider.QueryResult),
		queryCalls:   []provider.Query{},
	}
}

func (m *mockGraphConnectionForAggregate) CreateNode(ctx context.Context, node provider.Node) error {
	return nil
}

func (m *mockGraphConnectionForAggregate) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	return nil, nil
}

func (m *mockGraphConnectionForAggregate) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	return nil
}

func (m *mockGraphConnectionForAggregate) DeleteNode(ctx context.Context, id string, labels []string) error {
	return nil
}

func (m *mockGraphConnectionForAggregate) ListNodes(ctx context.Context, filter provider.NodeFilter) ([]*provider.Node, error) {
	return nil, nil
}

func (m *mockGraphConnectionForAggregate) CreateEdge(ctx context.Context, edge provider.Edge) error {
	return nil
}

func (m *mockGraphConnectionForAggregate) GetEdge(ctx context.Context, fromID, toID, edgeType string) (*provider.Edge, error) {
	return nil, nil
}

func (m *mockGraphConnectionForAggregate) UpdateEdge(ctx context.Context, fromID, toID, edgeType string, updates provider.EdgeUpdates) error {
	return nil
}

func (m *mockGraphConnectionForAggregate) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	return nil
}

func (m *mockGraphConnectionForAggregate) ListEdges(ctx context.Context, filter provider.EdgeFilter) ([]*provider.Edge, error) {
	return nil, nil
}

func (m *mockGraphConnectionForAggregate) executeQueryDefault(_ context.Context, query provider.Query) (*provider.QueryResult, error) {
	m.queryCalls = append(m.queryCalls, query)

	// Return predefined result based on query
	if result, ok := m.queryResults[query.Query]; ok {
		return result, nil
	}

	// Default empty result
	return &provider.QueryResult{
		Nodes: []*provider.Node{},
		Edges: []*provider.Edge{},
		Rows:  []map[string]any{},
		Meta:  make(map[string]any),
	}, nil
}

func (m *mockGraphConnectionForAggregate) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	if m.queryHandler != nil {
		return m.queryHandler(ctx, query)
	}
	return m.executeQueryDefault(ctx, query)
}

//nolint:gocritic // Interface requires value semantics for VectorQuery
func (m *mockGraphConnectionForAggregate) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	return nil, nil
}

//nolint:gocritic // Interface requires value semantics for TraversalQuery
func (m *mockGraphConnectionForAggregate) ExecuteTraversal(ctx context.Context, traversal provider.TraversalQuery) (*provider.QueryResult, error) {
	return nil, nil
}

func (m *mockGraphConnectionForAggregate) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	return nil, nil
}

func (m *mockGraphConnectionForAggregate) BeginNestedTransaction(ctx context.Context, parent provider.GraphTransaction) (provider.GraphTransaction, error) {
	return nil, nil
}

func (m *mockGraphConnectionForAggregate) HasOpenTransaction() bool {
	return false
}

func (m *mockGraphConnectionForAggregate) GetOpenTransaction() provider.GraphTransaction {
	return nil
}

func (m *mockGraphConnectionForAggregate) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockGraphConnectionForAggregate) Close() error {
	return nil
}

func (m *mockGraphConnectionForAggregate) ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	return nil, nil
}

func TestGraphAggregate_Count(t *testing.T) {
	mockConn := newMockGraphConnectionForAggregate()

	// Setup mock result for count query
	mockConn.queryResults["MATCH (n:BacklogItem:Entity) RETURN COUNT(n) AS count"] = &provider.QueryResult{
		Rows: []map[string]any{
			{"count": int64(3)},
		},
	}

	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	filter := ListFilter{
		Kind: "backlog_item",
	}
	aggregations := []Aggregation{
		{Function: AggregationCount, Alias: "count"},
	}

	result, err := graphStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if result.Aggregations["count"] != int64(3) {
		t.Errorf("Expected count = 3, got %v", result.Aggregations["count"])
	}
}

func TestGraphAggregate_GroupBy(t *testing.T) {
	mockConn := newMockGraphConnectionForAggregate()

	// Override query handler to handle grouped queries
	mockConn.queryHandler = func(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
		mockConn.queryCalls = append(mockConn.queryCalls, query)
		if strings.Contains(query.Query, "WITH") && strings.Contains(query.Query, "group_key") {
			return &provider.QueryResult{
				Rows: []map[string]any{
					{"group_key": "exploring", "count": int64(2)},
					{"group_key": "validated", "count": int64(1)},
				},
			}, nil
		}
		return mockConn.executeQueryDefault(ctx, query)
	}

	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	filter := ListFilter{
		Kind:    "backlog_item",
		GroupBy: "status",
	}
	aggregations := []Aggregation{
		{Function: AggregationCount, Alias: "count"},
	}

	result, err := graphStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if result.Groups == nil {
		t.Fatal("Expected groups to be populated")
	}

	exploringGroup, ok := result.Groups["exploring"]
	if !ok {
		t.Fatal("Expected 'exploring' group")
	}
	if exploringGroup.Aggregations["count"] != int64(2) {
		t.Errorf("Expected exploring count = 2, got %v", exploringGroup.Aggregations["count"])
	}

	validatedGroup, ok := result.Groups["validated"]
	if !ok {
		t.Fatal("Expected 'validated' group")
	}
	if validatedGroup.Aggregations["count"] != int64(1) {
		t.Errorf("Expected validated count = 1, got %v", validatedGroup.Aggregations["count"])
	}
}

func TestGraphAggregate_MultipleAggregations(t *testing.T) {
	mockConn := newMockGraphConnectionForAggregate()

	// Setup mock result for multiple aggregations
	mockConn.queryResults["MATCH (n:BacklogItem:Entity) RETURN COUNT(n) AS count, SUM(n.priority) AS sum_priority, AVG(n.priority) AS avg_priority"] = &provider.QueryResult{
		Rows: []map[string]any{
			{"count": int64(3), "sum_priority": 16.0, "avg_priority": 5.33},
		},
	}

	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	filter := ListFilter{
		Kind: "backlog_item",
	}
	aggregations := []Aggregation{
		{Function: AggregationCount, Alias: "count"},
		{Function: AggregationSum, Field: "priority", Alias: "sum_priority"},
		{Function: AggregationAvg, Field: "priority", Alias: "avg_priority"},
	}

	result, err := graphStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if result.Aggregations["count"] != int64(3) {
		t.Errorf("Expected count = 3, got %v", result.Aggregations["count"])
	}
	if result.Aggregations["sum_priority"] != 16.0 {
		t.Errorf("Expected sum_priority = 16.0, got %v", result.Aggregations["sum_priority"])
	}
}

func TestGraphAggregate_EmptyResult(t *testing.T) {
	mockConn := newMockGraphConnectionForAggregate()

	// Setup mock result for empty query
	mockConn.queryResults["MATCH (n:BacklogItem:Entity) WHERE n.status = $filter_status RETURN COUNT(n) AS count"] = &provider.QueryResult{
		Rows: []map[string]any{},
	}

	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	filter := ListFilter{
		Kind: "backlog_item",
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusNonexistent,
		},
	}
	aggregations := []Aggregation{
		{Function: AggregationCount, Alias: "count"},
	}

	result, err := graphStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}

	if result.Aggregations["count"] != 0 {
		t.Errorf("Expected count = 0 for empty result, got %v", result.Aggregations["count"])
	}
}

func TestGraphAggregate_InvalidAggregation(t *testing.T) {
	mockConn := newMockGraphConnectionForAggregate()
	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	filter := ListFilter{
		Kind: "backlog_item",
	}
	aggregations := []Aggregation{
		{Function: AggregationSum, Field: "", Alias: "sum"}, // Sum requires a field
	}

	_, err = graphStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err == nil {
		t.Fatal("Expected error for sum aggregation without field")
	}
}

func TestGraphAggregate_NoAggregations(t *testing.T) {
	mockConn := newMockGraphConnectionForAggregate()
	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	filter := ListFilter{
		Kind: "backlog_item",
	}
	aggregations := []Aggregation{} // Empty aggregations

	_, err = graphStorage.Aggregate(ctx, secCtx, storageCtx, filter, aggregations)
	if err == nil {
		t.Fatal("Expected error for empty aggregations")
	}
}
