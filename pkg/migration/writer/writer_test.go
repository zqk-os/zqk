package writer

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// MockGraphConnection for testing
type mockGraphConnection struct {
	nodesCreated []provider.Node
	edgesCreated []provider.Edge
}

func (m *mockGraphConnection) CreateNode(ctx context.Context, node provider.Node) error {
	m.nodesCreated = append(m.nodesCreated, node)
	return nil
}

func (m *mockGraphConnection) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
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
	m.edgesCreated = append(m.edgesCreated, edge)
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
	return nil, nil
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
	return &provider.BatchResult{
		SuccessCount: len(operations),
		FailureCount: 0,
		Errors:       []error{},
		Results:      []any{},
	}, nil
}

func TestCreateDocumentNode(t *testing.T) {
	t.Parallel()
	mockConn := &mockGraphConnection{}
	writer := NewGraphWriter(mockConn)

	ctx := pkgctx.NewSystemContext()

	docID := "doc_backlog_item_BLI-001"
	filePath := filepath.Join(paths.ProcessBacklogDir, "BLI-001.yaml")
	content := "id: BLI-001\nkind: backlog_item\ntitle: Test"

	err := writer.CreateDocumentNode(ctx, docID, filePath, content)
	if err != nil {
		t.Fatalf("CreateDocumentNode failed: %v", err)
	}

	// Verify node was created
	if len(mockConn.nodesCreated) != 1 {
		t.Fatalf("Expected 1 node created, got %d", len(mockConn.nodesCreated))
	}

	node := mockConn.nodesCreated[0]
	if node.ID != docID {
		t.Errorf("Expected node ID '%s', got '%s'", docID, node.ID)
	}

	// Verify labels
	hasDocumentLabel := false
	for _, label := range node.Labels {
		if label == "Document" {
			hasDocumentLabel = true
			break
		}
	}
	if !hasDocumentLabel {
		t.Error("Expected node to have 'Document' label")
	}

	// Verify properties
	if node.Properties["source_path"] != filePath {
		t.Errorf("Expected source_path '%s', got '%v'", filePath, node.Properties["source_path"])
	}

	if node.Properties[objects.FieldKeyContent] != content {
		t.Error("Expected content to be stored in properties")
	}
}

func TestCreateEntityNode(t *testing.T) {
	t.Parallel()
	mockConn := &mockGraphConnection{}
	writer := NewGraphWriter(mockConn)

	ctx := pkgctx.NewSystemContext()

	entityID := "BLI-001"
	entityKind := "backlog_item"
	properties := map[string]any{
		objects.FieldKeyID:       entityID,
		objects.FieldKeyKind:     entityKind,
		objects.FieldKeyTitle:    "Test Backlog Item",
		objects.FieldKeyStatus:   "planned",
		objects.FieldKeyCategory: "System",
	}

	err := writer.CreateEntityNode(ctx, entityID, entityKind, properties, nil)
	if err != nil {
		t.Fatalf("CreateEntityNode failed: %v", err)
	}

	// Verify node was created
	if len(mockConn.nodesCreated) != 1 {
		t.Fatalf("Expected 1 node created, got %d", len(mockConn.nodesCreated))
	}

	node := mockConn.nodesCreated[0]
	if node.ID != entityID {
		t.Errorf("Expected node ID '%s', got '%s'", entityID, node.ID)
	}

	// Verify label matches entity kind
	hasEntityLabel := false
	for _, label := range node.Labels {
		if label == "BacklogItem" {
			hasEntityLabel = true
			break
		}
	}
	if !hasEntityLabel {
		t.Errorf("Expected node to have '%s' label", entityKind)
	}

	// Verify properties
	if node.Properties[objects.FieldKeyTitle] != "Test Backlog Item" {
		t.Errorf("Expected title 'Test Backlog Item', got '%v'", node.Properties[objects.FieldKeyTitle])
	}
}

func TestCreateRelationshipEdge(t *testing.T) {
	t.Parallel()
	mockConn := &mockGraphConnection{}
	writer := NewGraphWriter(mockConn)

	ctx := pkgctx.NewSystemContext()

	fromID := "BLI-001"
	toID := "MIL-001"
	edgeType := "IMPLEMENTS"

	err := writer.CreateRelationshipEdge(ctx, fromID, toID, edgeType, nil)
	if err != nil {
		t.Fatalf("CreateRelationshipEdge failed: %v", err)
	}

	// Verify edge was created
	if len(mockConn.edgesCreated) != 1 {
		t.Fatalf("Expected 1 edge created, got %d", len(mockConn.edgesCreated))
	}

	edge := mockConn.edgesCreated[0]
	if edge.FromID != fromID {
		t.Errorf("Expected FromID '%s', got '%s'", fromID, edge.FromID)
	}

	if edge.ToID != toID {
		t.Errorf("Expected ToID '%s', got '%s'", toID, edge.ToID)
	}

	if edge.Type != edgeType {
		t.Errorf("Expected Type '%s', got '%s'", edgeType, edge.Type)
	}
}

func TestCreateHASSOURCEEdge(t *testing.T) {
	t.Parallel()
	mockConn := &mockGraphConnection{}
	writer := NewGraphWriter(mockConn)

	ctx := pkgctx.NewSystemContext()

	entityID := "BLI-001"
	documentID := "doc_backlog_item_BLI-001"

	err := writer.CreateHASSOURCEEdge(ctx, entityID, documentID)
	if err != nil {
		t.Fatalf("CreateHASSOURCEEdge failed: %v", err)
	}

	// Verify edge was created
	if len(mockConn.edgesCreated) != 1 {
		t.Fatalf("Expected 1 edge created, got %d", len(mockConn.edgesCreated))
	}

	edge := mockConn.edgesCreated[0]
	if edge.FromID != entityID {
		t.Errorf("Expected FromID '%s', got '%s'", entityID, edge.FromID)
	}

	if edge.ToID != documentID {
		t.Errorf("Expected ToID '%s', got '%s'", documentID, edge.ToID)
	}

	if edge.Type != "HAS_SOURCE" {
		t.Errorf("Expected Type 'HAS_SOURCE', got '%s'", edge.Type)
	}
}
