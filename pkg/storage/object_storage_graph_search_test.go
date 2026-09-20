package storage

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestGraphSearch_Basic(t *testing.T) {
	mockConn := newMockGraphConnectionForSearch()

	// Setup mock result for search query
	mockConn.queryHandler = func(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
		mockConn.queryCalls = append(mockConn.queryCalls, query)
		if strings.Contains(query.Query, "CONTAINS") {
			// Return mock nodes
			return &provider.QueryResult{
				Nodes: []*provider.Node{
					{
						ID:     "BLI-901",
						Labels: []string{"BacklogItem", "Entity"},
						Properties: map[string]any{
							objects.FieldKeyID:     "BLI-901",
							objects.FieldKeyKind:   "backlog_item",
							objects.FieldKeyTitle:  "Implement search feature",
							objects.FieldKeyStatus: objects.ObjectStatusExploring,
						},
					},
					{
						ID:     "BLI-903",
						Labels: []string{"BacklogItem", "Entity"},
						Properties: map[string]any{
							objects.FieldKeyID:     "BLI-903",
							objects.FieldKeyKind:   "backlog_item",
							objects.FieldKeyTitle:  "Search implementation",
							objects.FieldKeyStatus: objects.ObjectStatusExploring,
						},
					},
				},
				Rows: []map[string]any{
					{objects.FieldKeyScore: 1.0},
					{objects.FieldKeyScore: 0.8},
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

	query := SearchQuery{
		Query: "search",
		Kinds: []string{"backlog_item"},
		Limit: 10,
	}

	result, err := graphStorage.Search(ctx, secCtx, storageCtx, query)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if result.TotalCount < 2 {
		t.Errorf("Expected at least 2 matches for 'search', got %d", result.TotalCount)
	}
}

func TestGraphSearch_EmptyResult(t *testing.T) {
	mockConn := newMockGraphConnectionForSearch()

	// Setup mock result for empty search
	mockConn.queryHandler = func(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
		mockConn.queryCalls = append(mockConn.queryCalls, query)
		return &provider.QueryResult{
			Nodes: []*provider.Node{},
			Rows:  []map[string]any{},
		}, nil
	}

	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	query := SearchQuery{
		Query: "nonexistent",
		Kinds: []string{"backlog_item"},
		Limit: 10,
	}

	result, err := graphStorage.Search(ctx, secCtx, storageCtx, query)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("Expected 0 matches for 'nonexistent', got %d", result.TotalCount)
	}
}

// mockGraphConnectionForSearch provides a mock graph connection for search tests
type mockGraphConnectionForSearch struct {
	queryResults map[string]*provider.QueryResult
	queryCalls   []provider.Query
	queryHandler func(ctx context.Context, query provider.Query) (*provider.QueryResult, error)
}

func newMockGraphConnectionForSearch() *mockGraphConnectionForSearch {
	return &mockGraphConnectionForSearch{
		queryResults: make(map[string]*provider.QueryResult),
		queryCalls:   []provider.Query{},
	}
}

func (m *mockGraphConnectionForSearch) CreateNode(ctx context.Context, node provider.Node) error {
	return nil
}

func (m *mockGraphConnectionForSearch) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	return nil, nil
}

func (m *mockGraphConnectionForSearch) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	return nil
}

func (m *mockGraphConnectionForSearch) DeleteNode(ctx context.Context, id string, labels []string) error {
	return nil
}

func (m *mockGraphConnectionForSearch) ListNodes(ctx context.Context, filter provider.NodeFilter) ([]*provider.Node, error) {
	return nil, nil
}

func (m *mockGraphConnectionForSearch) CreateEdge(ctx context.Context, edge provider.Edge) error {
	return nil
}

func (m *mockGraphConnectionForSearch) GetEdge(ctx context.Context, fromID, toID, edgeType string) (*provider.Edge, error) {
	return nil, nil
}

func (m *mockGraphConnectionForSearch) UpdateEdge(ctx context.Context, fromID, toID, edgeType string, updates provider.EdgeUpdates) error {
	return nil
}

func (m *mockGraphConnectionForSearch) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	return nil
}

func (m *mockGraphConnectionForSearch) ListEdges(ctx context.Context, filter provider.EdgeFilter) ([]*provider.Edge, error) {
	return nil, nil
}

func (m *mockGraphConnectionForSearch) executeQueryDefault(_ context.Context, query provider.Query) (*provider.QueryResult, error) {
	m.queryCalls = append(m.queryCalls, query)

	if result, ok := m.queryResults[query.Query]; ok {
		return result, nil
	}

	return &provider.QueryResult{
		Nodes: []*provider.Node{},
		Edges: []*provider.Edge{},
		Rows:  []map[string]any{},
		Meta:  make(map[string]any),
	}, nil
}

func (m *mockGraphConnectionForSearch) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	if m.queryHandler != nil {
		return m.queryHandler(ctx, query)
	}
	return m.executeQueryDefault(ctx, query)
}

//nolint:gocritic // Interface requires value semantics for VectorQuery
func (m *mockGraphConnectionForSearch) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	return nil, nil
}

//nolint:gocritic // Interface requires value semantics for TraversalQuery
func (m *mockGraphConnectionForSearch) ExecuteTraversal(ctx context.Context, traversal provider.TraversalQuery) (*provider.QueryResult, error) {
	return nil, nil
}

func (m *mockGraphConnectionForSearch) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	return nil, nil
}

func (m *mockGraphConnectionForSearch) BeginNestedTransaction(ctx context.Context, parent provider.GraphTransaction) (provider.GraphTransaction, error) {
	return nil, nil
}

func (m *mockGraphConnectionForSearch) HasOpenTransaction() bool {
	return false
}

func (m *mockGraphConnectionForSearch) GetOpenTransaction() provider.GraphTransaction {
	return nil
}

func (m *mockGraphConnectionForSearch) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockGraphConnectionForSearch) Close() error {
	return nil
}

func (m *mockGraphConnectionForSearch) ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	return nil, nil
}
