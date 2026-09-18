package observer

import (
	"context"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// mockGraphConn records CreateNode/CreateEdge calls for testing.
type mockGraphConn struct {
	mu     sync.Mutex
	Nodes  []provider.Node
	Edges  []provider.Edge
	FailOp string // if set, fail the next CreateNode or CreateEdge with this op
}

func (m *mockGraphConn) CreateNode(ctx context.Context, node provider.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailOp == "create_node" {
		m.FailOp = ""
		return context.DeadlineExceeded
	}
	m.Nodes = append(m.Nodes, node)
	return nil
}

func (m *mockGraphConn) CreateEdge(ctx context.Context, edge provider.Edge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailOp == "create_edge" {
		m.FailOp = ""
		return context.DeadlineExceeded
	}
	m.Edges = append(m.Edges, edge)
	return nil
}

func TestEntityID(t *testing.T) {
	e := &Entity{File: "a.go", Line: 10, Kind: "function", Name: "Foo"}
	if got := EntityID(e); got != "code_entity:a.go:10:function:Foo" {
		t.Errorf("EntityID = %q, want code_entity:a.go:10:function:Foo", got)
	}
	e.Receiver = "Bar"
	e.Kind = "method"
	e.Name = "Do"
	if got := EntityID(e); got != "code_entity:a.go:10:method:Bar.Do" {
		t.Errorf("EntityID(method) = %q", got)
	}
}

func TestFileID(t *testing.T) {
	if got := FileID("pkg/foo.go"); got != "source_file:pkg/foo.go" {
		t.Errorf("FileID = %q", got)
	}
}

func TestEntityToNode(t *testing.T) {
	e := &Entity{
		Kind: "function", Name: "Foo", File: "a.go", Line: 5,
		Signature: "func Foo()", Language: "go",
		Metadata: map[string]string{"package": "pkg"},
	}
	n := EntityToNode(e)
	if n.ID != EntityID(e) {
		t.Errorf("node ID = %q", n.ID)
	}
	if len(n.Labels) < 2 || n.Labels[0] != LabelCodeEntity || n.Labels[1] != "function" {
		t.Errorf("labels = %v", n.Labels)
	}
	if n.Properties[objects.FieldKeyName] != "Foo" || n.Properties["line"].(int) != 5 {
		t.Errorf("properties = %v", n.Properties)
	}
}

func TestPopulate_RecordsNodesAndEdges(t *testing.T) {
	ctx := context.Background()
	result := &ExtractResult{
		Entities: []Entity{
			{Kind: "type", Name: "Bar", File: "a.go", Line: 3, Language: "go"},
			{Kind: "function", Name: "Foo", File: "a.go", Line: 5, Language: "go"},
			{Kind: "method", Name: "Do", File: "a.go", Line: 7, Receiver: "Bar", Language: "go"},
		},
	}
	mock := &mockGraphConn{}
	res, err := Populate(ctx, mock, result, "run1")
	if err != nil {
		t.Fatalf("Populate: %v", err)
	}
	if res.NodesCreated != 4 {
		t.Errorf("NodesCreated = %d, want 4 (1 file + 3 entities)", res.NodesCreated)
	}
	// 3 CONTAINS + 1 METHOD_OF (Do -> Bar)
	if res.EdgesCreated != 4 {
		t.Errorf("EdgesCreated = %d, want 4", res.EdgesCreated)
	}
	if len(mock.Nodes) != 4 {
		t.Errorf("mock nodes = %d", len(mock.Nodes))
	}
	if len(mock.Edges) != 4 {
		t.Errorf("mock edges = %d", len(mock.Edges))
	}
	var methodOf int
	for _, e := range mock.Edges {
		if e.Type == EdgeMethodOf {
			methodOf++
		}
	}
	if methodOf != 1 {
		t.Errorf("METHOD_OF edges = %d, want 1", methodOf)
	}
}

func TestPopulate_NilConn(t *testing.T) {
	_, err := Populate(context.Background(), nil, &ExtractResult{Entities: []Entity{{}}}, "")
	if err == nil {
		t.Error("expected error for nil conn")
	}
}
