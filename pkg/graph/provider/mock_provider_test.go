package provider

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMockGraphProvider(t *testing.T) {
	ctx := context.Background()
	provider := NewMockGraphProvider()
	config := ConnectionConfig{MaxConns: 5}
	pool, err := provider.CreatePool(ctx, config)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	t.Run("Node operations", func(t *testing.T) {
		conn, err := pool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("failed to get connection: %v", err)
		}
		defer func() { _ = pool.ReturnConnection(conn) }()

		node := Node{
			ID:     "n1",
			Labels: []string{"User"},
			Properties: map[string]any{
				"test_name": "Alice",
			},
		}

		if err := conn.CreateNode(ctx, node); err != nil {
			t.Fatalf("failed to create node: %v", err)
		}

		got, err := conn.GetNode(ctx, "n1", nil)
		if err != nil {
			t.Fatalf("failed to get node: %v", err)
		}
		if got.ID != "n1" || got.Properties["test_name"] != "Alice" {
			t.Errorf("unexpected node data: %+v", got)
		}

		updates := NodeUpdates{
			Properties: map[string]any{"age": 30},
			AddLabels:  []string{"Admin"},
		}
		if err := conn.UpdateNode(ctx, "n1", updates); err != nil {
			t.Fatalf("failed to update node: %v", err)
		}

		got, _ = conn.GetNode(ctx, "n1", nil)
		if got.Properties["age"] != 30 || len(got.Labels) != 2 {
			t.Errorf("update failed: %+v", got)
		}

		nodes, err := conn.ListNodes(ctx, NodeFilter{Labels: []string{"Admin"}})
		if err != nil {
			t.Fatalf("failed to list nodes: %v", err)
		}
		if len(nodes) != 1 {
			t.Errorf("expected 1 node, got %d", len(nodes))
		}

		if err := conn.DeleteNode(ctx, "n1", nil); err != nil {
			t.Fatalf("failed to delete node: %v", err)
		}
		_, err = conn.GetNode(ctx, "n1", nil)
		if err == nil {
			t.Error("expected error getting deleted node")
		}
	})

	t.Run("Edge operations", func(t *testing.T) {
		conn, err := pool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("failed to get connection: %v", err)
		}
		defer func() { _ = pool.ReturnConnection(conn) }()

		edge := Edge{
			FromID: "n1",
			ToID:   "n2",
			Type:   "FOLLOWS",
			Properties: map[string]any{
				"since": 2020,
			},
		}

		if err := conn.CreateEdge(ctx, edge); err != nil {
			t.Fatalf("failed to create edge: %v", err)
		}

		got, err := conn.GetEdge(ctx, "n1", "n2", "FOLLOWS")
		if err != nil {
			t.Fatalf("failed to get edge: %v", err)
		}
		if got.Properties["since"] != 2020 {
			t.Errorf("got unexpected edge: %+v", got)
		}

		edges, err := conn.ListEdges(ctx, EdgeFilter{Type: "FOLLOWS"})
		if err != nil {
			t.Fatalf("failed to list edges: %v", err)
		}
		if len(edges) != 1 {
			t.Errorf("expected 1 edge, got %d", len(edges))
		}
	})

	t.Run("Transactions", func(t *testing.T) {
		conn, err := pool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("failed to get connection: %v", err)
		}
		defer func() { _ = pool.ReturnConnection(conn) }()

		tx, err := conn.BeginTransaction(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}

		nodeID := fmt.Sprintf("t1-%d", time.Now().UnixNano())
		node := Node{ID: nodeID, Labels: []string{"Test"}}
		if err := tx.CreateNode(ctx, node); err != nil {
			t.Fatalf("failed to create node in tx: %v", err)
		}

		// Should not be in store yet
		n, err := conn.GetNode(ctx, nodeID, nil)
		if err == nil {
			t.Errorf("node should not be in store before commit, got: %+v", n)
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("failed to commit: %v", err)
		}

		// Should be in store now
		_, err = conn.GetNode(ctx, nodeID, nil)
		if err != nil {
			t.Errorf("node should be in store after commit: %v", err)
		}
	})
}
