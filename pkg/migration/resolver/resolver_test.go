package resolver

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// MockGraphConnection for testing
type mockGraphConnection struct {
	queries []provider.Query
}

func (m *mockGraphConnection) CreateNode(ctx context.Context, node provider.Node) error {
	return nil
}

func (m *mockGraphConnection) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	// Return mock node if ID matches
	if id == "MIL-001" || id == "GOAL-001" {
		return &provider.Node{
			ID:     id,
			Labels: []string{"Milestone", "Entity"},
			Properties: map[string]any{
				objects.FieldKeyID: id,
			},
		}, nil
	}
	return nil, nil
}

func (m *mockGraphConnection) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	return nil
}

func (m *mockGraphConnection) DeleteNode(ctx context.Context, id string, labels []string) error {
	return nil
}

func (m *mockGraphConnection) ListNodes(ctx context.Context, filter provider.NodeFilter) ([]*provider.Node, error) {
	return nil, nil
}

func (m *mockGraphConnection) CreateEdge(ctx context.Context, edge provider.Edge) error {
	return nil
}

func (m *mockGraphConnection) GetEdge(ctx context.Context, fromID, toID, edgeType string) (*provider.Edge, error) {
	return nil, nil
}

func (m *mockGraphConnection) UpdateEdge(ctx context.Context, fromID, toID, edgeType string, updates provider.EdgeUpdates) error {
	return nil
}

func (m *mockGraphConnection) DeleteEdge(ctx context.Context, fromID, toID, edgeType string) error {
	return nil
}

func (m *mockGraphConnection) ListEdges(ctx context.Context, filter provider.EdgeFilter) ([]*provider.Edge, error) {
	return nil, nil
}

func (m *mockGraphConnection) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	m.queries = append(m.queries, query)

	// Return mock results based on query
	if query.Query == "MATCH (n {id: $id}) RETURN n" {
		id := query.Params[objects.FieldKeyID].(string)
		if id == "MIL-001" || id == "GOAL-001" {
			return &provider.QueryResult{
				Rows: []map[string]any{
					{"n": map[string]any{objects.FieldKeyID: id}},
				},
			}, nil
		}
	}

	return &provider.QueryResult{Rows: []map[string]any{}}, nil
}

//nolint:gocritic // Interface requires value semantics for VectorQuery
func (m *mockGraphConnection) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	return nil, nil
}

//nolint:gocritic // Interface requires value semantics for TraversalQuery
func (m *mockGraphConnection) ExecuteTraversal(ctx context.Context, traversal provider.TraversalQuery) (*provider.QueryResult, error) {
	return nil, nil
}

func (m *mockGraphConnection) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
	return nil, nil
}

func (m *mockGraphConnection) BeginNestedTransaction(ctx context.Context, parent provider.GraphTransaction) (provider.GraphTransaction, error) {
	return nil, nil
}

func (m *mockGraphConnection) HasOpenTransaction() bool {
	return false
}

func (m *mockGraphConnection) GetOpenTransaction() provider.GraphTransaction {
	return nil
}

func (m *mockGraphConnection) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockGraphConnection) Close() error {
	return nil
}

func (m *mockGraphConnection) ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	return nil, nil
}

func TestResolveReference(t *testing.T) {
	t.Parallel()
	mockConn := &mockGraphConnection{}
	resolver := NewReferenceResolver(mockConn)

	ctx := pkgctx.NewSystemContext()

	// Test resolving existing reference
	exists, err := resolver.ResolveReference(ctx, "MIL-001")
	if err != nil {
		t.Fatalf("ResolveReference failed: %v", err)
	}

	if !exists {
		t.Error("Expected MIL-001 to exist, got false")
	}

	// Test resolving non-existent reference
	exists, err = resolver.ResolveReference(ctx, "MIL-999")
	if err != nil {
		t.Fatalf("ResolveReference failed: %v", err)
	}

	if exists {
		t.Error("Expected MIL-999 to not exist, got true")
	}
}

func TestResolveReferences(t *testing.T) {
	t.Parallel()
	mockConn := &mockGraphConnection{}
	resolver := NewReferenceResolver(mockConn)

	ctx := pkgctx.NewSystemContext()

	references := []string{"MIL-001", "GOAL-001", "MIL-999"}

	results := resolver.ResolveReferences(ctx, references)

	// Should have 3 results
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// First two should exist
	if !results["MIL-001"] {
		t.Error("Expected MIL-001 to exist")
	}

	if !results["GOAL-001"] {
		t.Error("Expected GOAL-001 to exist")
	}

	// Last one should not exist
	if results["MIL-999"] {
		t.Error("Expected MIL-999 to not exist")
	}
}

func TestResolveReferenceField(t *testing.T) {
	t.Parallel()
	mockConn := &mockGraphConnection{}
	resolver := NewReferenceResolver(mockConn)

	ctx := pkgctx.NewSystemContext()

	// Test single reference
	fieldValue := "MIL-001"
	unresolved := resolver.ResolveReferenceField(ctx, "milestone_ref", fieldValue)

	if len(unresolved) != 0 {
		t.Errorf("Expected 0 unresolved refs, got %d", len(unresolved))
	}

	// Test non-existent reference
	fieldValue = "MIL-999"
	unresolved = resolver.ResolveReferenceField(ctx, "milestone_ref", fieldValue)

	if len(unresolved) != 1 {
		t.Errorf("Expected 1 unresolved ref, got %d", len(unresolved))
	}

	if unresolved[0] != "MIL-999" {
		t.Errorf("Expected unresolved ref 'MIL-999', got '%s'", unresolved[0])
	}
}

func TestResolveReferenceField_Array(t *testing.T) {
	t.Parallel()
	mockConn := &mockGraphConnection{}
	resolver := NewReferenceResolver(mockConn)

	ctx := pkgctx.NewSystemContext()

	// Test array of references
	fieldValue := []any{"MIL-001", "GOAL-001", "MIL-999"}
	unresolved := resolver.ResolveReferenceField(ctx, objects.FieldKeyMilestoneRefs, fieldValue)

	// Should have 1 unresolved (MIL-999)
	if len(unresolved) != 1 {
		t.Errorf("Expected 1 unresolved ref, got %d", len(unresolved))
	}

	if unresolved[0] != "MIL-999" {
		t.Errorf("Expected unresolved ref 'MIL-999', got '%s'", unresolved[0])
	}
}
