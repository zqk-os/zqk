package storage

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/goleak"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

type mockTraversalConn struct {
	mockGraphConnection
	traversalCalled bool
	traversalNodes  []*provider.Node
}

func (m *mockTraversalConn) ExecuteTraversal(ctx context.Context, traversal provider.TraversalQuery) (*provider.QueryResult, error) {
	m.traversalCalled = true
	if len(m.traversalNodes) > 0 {
		return &provider.QueryResult{
			Nodes: m.traversalNodes,
		}, nil
	}
	return nil, nil
}

// TestGraphObjectStorage_GetNeighbors_BulkTraversal verifies BLI-CEF-PERF-001:
// ExecuteTraversal is invoked to fetch neighbors in a single bulk query.
func TestGraphObjectStorage_GetNeighbors_BulkTraversal(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	startNode := provider.Node{
		ID:     "OBJ-start",
		Labels: []string{"Item"},
		Properties: map[string]any{
			objects.FieldKeyKind: "item",
			objects.FieldKeyID:   "OBJ-start",
		},
	}
	neighborNode := provider.Node{
		ID:     "OBJ-neighbor-1",
		Labels: []string{"Item"},
		Properties: map[string]any{
			objects.FieldKeyKind: "item",
			objects.FieldKeyID:   "OBJ-neighbor-1",
		},
	}

	mockConn := &mockTraversalConn{
		mockGraphConnection: mockGraphConnection{
			nodes: map[string]provider.Node{
				"OBJ-start":      startNode,
				"OBJ-neighbor-1": neighborNode,
			},
		},
		traversalNodes: []*provider.Node{&neighborNode},
	}

	storage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("NewGraphObjectStorage failed: %v", err)
	}

	neighbors, err := storage.GetNeighbors(ctx, secCtx, "OBJ-start", "outgoing")
	if err != nil {
		t.Fatalf("GetNeighbors failed: %v", err)
	}

	if !mockConn.traversalCalled {
		t.Errorf("expected ExecuteTraversal to be called for bulk lookup")
	}

	if len(neighbors) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(neighbors))
	}
	if neighbors[0][objects.FieldKeyID] != "OBJ-neighbor-1" {
		t.Errorf("expected OBJ-neighbor-1, got %v", neighbors[0][objects.FieldKeyID])
	}
}

// TestGraphObjectStorage_FindPathBFS_DepthBound verifies BLI-CEF-PERF-003:
// BFS search stops at maxBFSDepth without runaway queue expansion.
func TestGraphObjectStorage_FindPathBFS_DepthBound(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Construct a long chain exceeding maxBFSDepth (35 nodes)
	const chainLen = 35
	nodes := make(map[string]provider.Node)
	for i := 0; i < chainLen; i++ {
		id := fmt.Sprintf("CHAIN-NODE-%d", i)
		nodes[id] = provider.Node{
			ID:     id,
			Labels: []string{"Item"},
			Properties: map[string]any{
				objects.FieldKeyKind: "item",
				objects.FieldKeyID:   id,
			},
		}
	}

	mockConn := &mockChainConn{
		mockGraphConnection: mockGraphConnection{
			nodes: nodes,
		},
		chainLen: chainLen,
	}

	storage, err := NewGraphObjectStorage(mockConn, "")
	if err != nil {
		t.Fatalf("NewGraphObjectStorage failed: %v", err)
	}

	// Searching from 0 to 34 should not find path because chainLen > maxBFSDepth (32)
	path, err := storage.findPathBFS(ctx, secCtx, "CHAIN-NODE-0", fmt.Sprintf("CHAIN-NODE-%d", chainLen-1))
	if err != nil {
		t.Fatalf("findPathBFS returned unexpected error: %v", err)
	}
	if len(path) != 0 {
		t.Errorf("expected empty path due to maxBFSDepth cutoff, got %d nodes", len(path))
	}
}

type mockChainConn struct {
	mockGraphConnection
	chainLen int
}

func (m *mockChainConn) ExecuteTraversal(ctx context.Context, traversal provider.TraversalQuery) (*provider.QueryResult, error) {
	var idx int
	_, _ = fmt.Sscanf(traversal.StartNodeID, "CHAIN-NODE-%d", &idx)
	nextIdx := idx + 1
	if nextIdx < m.chainLen {
		nextID := fmt.Sprintf("CHAIN-NODE-%d", nextIdx)
		if node, ok := m.nodes[nextID]; ok {
			return &provider.QueryResult{
				Nodes: []*provider.Node{&node},
			}, nil
		}
	}
	return nil, nil
}
