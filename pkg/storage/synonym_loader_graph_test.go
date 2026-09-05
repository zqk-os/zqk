package storage

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// mockGraphConnection is a simple mock for testing
type mockGraphConnection struct {
	nodes map[string]provider.Node
}

func (m *mockGraphConnection) CreateNode(ctx context.Context, node provider.Node) error {
	if m.nodes == nil {
		m.nodes = make(map[string]provider.Node)
	}
	m.nodes[node.ID] = node
	return nil
}

func (m *mockGraphConnection) GetNode(ctx context.Context, id string, labels []string) (*provider.Node, error) {
	if m.nodes == nil {
		return nil, nil
	}
	node, exists := m.nodes[id]
	if !exists {
		return nil, nil
	}
	return &node, nil
}

func (m *mockGraphConnection) UpdateNode(ctx context.Context, id string, updates provider.NodeUpdates) error {
	if m.nodes == nil {
		return nil
	}
	node, exists := m.nodes[id]
	if !exists {
		return nil
	}
	for k, v := range updates.Properties {
		node.Properties[k] = v
	}
	m.nodes[id] = node
	return nil
}

func (m *mockGraphConnection) DeleteNode(ctx context.Context, id string, labels []string) error {
	if m.nodes == nil {
		return nil
	}
	delete(m.nodes, id)
	return nil
}

func (m *mockGraphConnection) ListNodes(ctx context.Context, filter provider.NodeFilter) ([]*provider.Node, error) {
	var nodes []*provider.Node
	for _, node := range m.nodes {
		// Simple label matching
		if len(filter.Labels) > 0 {
			matches := false
			for _, filterLabel := range filter.Labels {
				for _, nodeLabel := range node.Labels {
					if nodeLabel == filterLabel {
						matches = true
						break
					}
				}
				if matches {
					break
				}
			}
			if !matches {
				continue
			}
		}
		nodeCopy := node
		nodes = append(nodes, &nodeCopy)
	}
	return nodes, nil
}

func (m *mockGraphConnection) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	result := &provider.QueryResult{
		Nodes: []*provider.Node{},
		Edges: []*provider.Edge{},
		Rows:  []map[string]any{},
	}

	// Simple query execution for MATCH queries
	// For kind_synonym queries, return matching nodes
	if query.Query != emptyValue {
		for _, node := range m.nodes {
			// Check if node has "KindSynonym" label (PascalCase from toLabel("kind_synonym"))
			hasKindSynonymLabel := false
			for _, label := range node.Labels {
				if label == "KindSynonym" || label == "Entity" {
					hasKindSynonymLabel = true
					break
				}
			}
			if hasKindSynonymLabel {
				nodeCopy := node
				result.Nodes = append(result.Nodes, &nodeCopy)
				// Also add to rows for compatibility
				result.Rows = append(result.Rows, map[string]any{
					"n": &nodeCopy,
				})
			}
		}
	}

	return result, nil
}

func (m *mockGraphConnection) Close() error {
	return nil
}

// Implement remaining GraphConnection interface methods (stubs for testing)
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

func (m *mockGraphConnection) ExecuteBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	res := &provider.BatchResult{
		Results: make([]any, 0, len(operations)),
		Errors:  make([]error, 0),
	}
	for _, op := range operations {
		switch op.Type {
		case "create_node":
			node, ok := op.Data.(provider.Node)
			if !ok {
				res.FailureCount++
				res.Errors = append(res.Errors, errfmt.Errorf("invalid node data for create_node"))
				continue
			}
			if err := m.CreateNode(ctx, node); err != nil {
				res.FailureCount++
				res.Errors = append(res.Errors, err)
				continue
			}
			res.SuccessCount++
			res.Results = append(res.Results, node)
		case "update_node":
			updateData, ok := op.Data.(map[string]any)
			if !ok {
				res.FailureCount++
				res.Errors = append(res.Errors, errfmt.Errorf("invalid update data for update_node"))
				continue
			}
			id, _ := updateData[objects.FieldKeyID].(string)
			updates := provider.NodeUpdates{}
			if props, ok := updateData["properties"].(map[string]any); ok {
				updates.Properties = props
			}
			if err := m.UpdateNode(ctx, id, updates); err != nil {
				res.FailureCount++
				res.Errors = append(res.Errors, err)
				continue
			}
			res.SuccessCount++
			res.Results = append(res.Results, updateData)
		default:
			res.FailureCount++
			res.Errors = append(res.Errors, errfmt.Errorf("unsupported batch op %q in mock", op.Type))
		}
	}
	return res, nil
}

func TestStorageSynonymLoader_WithGraphStorage(t *testing.T) {
	// Create mock graph connection
	mockConn := &mockGraphConnection{
		nodes: make(map[string]provider.Node),
	}

	// Create graph storage
	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	// Create synonym objects in the graph
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	synonymObjects := []map[string]any{
		{
			objects.FieldKeyID:         "SYN-001",
			objects.FieldKeyKind:       "kind_synonym",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeySynonym:    "bi",
			objects.FieldKeyPriority:   10,
		},
		{
			objects.FieldKeyID:         "SYN-002",
			objects.FieldKeyKind:       "kind_synonym",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeySynonym:    "bli",
			objects.FieldKeyPriority:   5,
		},
		{
			objects.FieldKeyID:         "SYN-003",
			objects.FieldKeyKind:       "kind_synonym",
			objects.FieldKeyTargetKind: "priority_plan",
			objects.FieldKeySynonym:    "pp",
			objects.FieldKeyPriority:   10,
		},
	}

	// Create synonym objects in graph storage
	for _, obj := range synonymObjects {
		if err := graphStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create synonym object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Create synonym loader with graph storage
	loader := NewStorageSynonymLoader(graphStorage)

	// Load synonyms
	synonyms, err := loader.LoadSynonyms()
	if err != nil {
		t.Fatalf("LoadSynonyms() failed: %v", err)
	}

	if len(synonyms) != len(synonymObjects) {
		t.Errorf("LoadSynonyms() returned %d synonyms, want %d", len(synonyms), len(synonymObjects))
	}

	// Verify synonyms
	synonymMap := make(map[string]objects.SynonymData)
	for _, s := range synonyms {
		key := s.Kind + ":" + s.Synonym
		synonymMap[key] = s
	}

	expected := []struct {
		targetKind string
		synonym    string
		priority   int
	}{
		{"backlog_item", "bi", 10},
		{"backlog_item", "bli", 5},
		{"priority_plan", "pp", 10},
	}

	for _, exp := range expected {
		key := exp.targetKind + ":" + exp.synonym
		syn, exists := synonymMap[key]
		if !exists {
			t.Errorf("Synonym %q for %q not found", exp.synonym, exp.targetKind)
			continue
		}
		if syn.Kind != exp.targetKind {
			t.Errorf("Synonym %q has kind %q, want %q", exp.synonym, syn.Kind, exp.targetKind)
		}
		if syn.Synonym != exp.synonym {
			t.Errorf("Synonym has value %q, want %q", syn.Synonym, exp.synonym)
		}
		if syn.Priority != exp.priority {
			t.Errorf("Synonym %q has priority %d, want %d", exp.synonym, syn.Priority, exp.priority)
		}
	}
}

func TestStorageSynonymLoader_GraphStorage_Deduplication(t *testing.T) {
	// Create mock graph connection
	mockConn := &mockGraphConnection{
		nodes: make(map[string]provider.Node),
	}

	// Create graph storage
	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	// Create duplicate synonym objects (same target_kind and synonym, different priorities)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	synonymObjects := []map[string]any{
		{
			objects.FieldKeyID:         "SYN-001",
			objects.FieldKeyKind:       "kind_synonym",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeySynonym:    "bi",
			objects.FieldKeyPriority:   5, // Lower priority
		},
		{
			objects.FieldKeyID:         "SYN-002",
			objects.FieldKeyKind:       "kind_synonym",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeySynonym:    "bi",
			objects.FieldKeyPriority:   10, // Higher priority - should win
		},
	}

	// Create synonym objects in graph storage
	for _, obj := range synonymObjects {
		if err := graphStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create synonym object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Create synonym loader with graph storage
	loader := NewStorageSynonymLoader(graphStorage)

	// Load synonyms
	synonyms, err := loader.LoadSynonyms()
	if err != nil {
		t.Fatalf("LoadSynonyms() failed: %v", err)
	}

	// Should have both synonyms loaded (deduplication happens in resolver, not loader)
	if len(synonyms) != 2 {
		t.Errorf("LoadSynonyms() returned %d synonyms, want 2 (both should be loaded, deduplication in resolver)", len(synonyms))
	}

	// Verify both are present (resolver will handle deduplication)
	biCount := 0
	for _, s := range synonyms {
		if s.Kind == "backlog_item" && s.Synonym == "bi" {
			biCount++
		}
	}

	if biCount != 2 {
		t.Errorf("Expected 2 'bi' synonyms for backlog_item, got %d", biCount)
	}

	// Now test with resolver to verify deduplication
	resolver := objects.NewKindSynonymResolver()
	resolver.SetSynonymLoader(loader)

	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Resolver Initialize() failed: %v", err)
	}

	// Should resolve correctly (highest priority wins)
	result := resolver.ResolveKind("bi")
	if result != "backlog_item" {
		t.Errorf("ResolveKind('bi') = %q, want 'backlog_item'", result)
	}

	// Should only have one "bi" in synonyms list (deduplicated)
	synonymList := resolver.GetSynonyms("backlog_item")
	biInList := 0
	for _, s := range synonymList {
		if s == "bi" {
			biInList++
		}
	}

	if biInList != 1 {
		t.Errorf("GetSynonyms('backlog_item') has %d 'bi' entries, want 1 (deduplicated)", biInList)
	}
}

func TestStorageSynonymLoader_GraphStorage_EmptyResult(t *testing.T) {
	// Create mock graph connection with no nodes
	mockConn := &mockGraphConnection{
		nodes: make(map[string]provider.Node),
	}

	// Create graph storage
	graphStorage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("Failed to create graph storage: %v", err)
	}

	// Create synonym loader with graph storage
	loader := NewStorageSynonymLoader(graphStorage)

	// Load synonyms (should return empty list, not error)
	synonyms, err := loader.LoadSynonyms()
	if err != nil {
		t.Fatalf("LoadSynonyms() failed: %v", err)
	}

	if len(synonyms) != 0 {
		t.Errorf("LoadSynonyms() returned %d synonyms, want 0", len(synonyms))
	}
}
