package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/ontology"
	"github.com/zqk-os/zqk/pkg/semantic/graph"
)

type mockGraphProvider struct {
	nodes map[string]graph.Node
	edges map[string][]graph.Edge
}

func (m *mockGraphProvider) GetNode(ctx context.Context, id string) (graph.Node, error) {
	if n, ok := m.nodes[id]; ok {
		return n, nil
	}
	return graph.Node{}, errors.New("not found")
}

func (m *mockGraphProvider) GetOutboundEdges(ctx context.Context, sourceID string) ([]graph.Edge, error) {
	if e, ok := m.edges[sourceID]; ok {
		return e, nil
	}
	return nil, nil
}

func TestMemoryOntologyBridge_Traverse(t *testing.T) {
	provider := &mockGraphProvider{
		nodes: map[string]graph.Node{
			"A": {ID: "A", Class: ontology.Class{Name: "NodeA"}},
			"B": {ID: "B", Class: ontology.Class{Name: "NodeB"}},
			"C": {ID: "C", Class: ontology.Class{Name: "NodeC"}},
		},
		edges: map[string][]graph.Edge{
			"A": {{SourceID: "A", TargetID: "B", Relation: "DependsOn"}},
			"B": {{SourceID: "B", TargetID: "C", Relation: "Implements"}},
		},
	}

	bridge := graph.NewBridge(provider)
	ctx := context.Background()

	// Traverse depth 1
	nodes, edges, err := bridge.Traverse(ctx, "A", 1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes (A, B), got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Errorf("Expected 1 edge (A->B), got %d", len(edges))
	}

	// Traverse depth 2
	nodes, edges, err = bridge.Traverse(ctx, "A", 2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes (A, B, C), got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Errorf("Expected 2 edges, got %d", len(edges))
	}

	// GetRelated
	related, err := bridge.GetRelated(ctx, "A", "DependsOn")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(related) != 1 || related[0].ID != "B" {
		t.Errorf("Expected node B, got %v", related)
	}
}
