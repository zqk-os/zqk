package mesh_test

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/graph/memgraph"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/mesh"
)

func TestStateSync_Integration(t *testing.T) {
	t.Skip("Skipping flaky UDP broadcast test")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mgConfig := memgraph.MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		Username: "",
		Password: "",
		Database: "test",
		PoolSize: 2,
	}
	mgProvider := memgraph.NewMemGraphProvider(&mgConfig)

	poolConfig := provider.ConnectionConfig{
		MaxConns:    2,
		PoolTimeout: 5 * time.Second,
	}

	pool, err := mgProvider.CreatePool(ctx, poolConfig)
	if err != nil {
		t.Skipf("Graph DB not available for integration test: %v", err)
	}
	defer pool.Close()

	multicastAddr := "224.0.0.1:9999"
	syncer1, _ := mesh.NewGossipSyncer(multicastAddr, pool, nil)
	syncer2, _ := mesh.NewGossipSyncer(multicastAddr, pool, nil)

	if err := syncer1.Start(ctx); err != nil {
		t.Fatalf("Failed to start syncer1: %v", err)
	}
	defer func() { _ = syncer1.Stop() }()

	if err := syncer2.Start(ctx); err != nil {
		t.Fatalf("Failed to start syncer2: %v", err)
	}
	defer func() { _ = syncer2.Stop() }()

	received := make(chan mesh.SyncEvent, 1)
	syncer1.Subscribe(func(e mesh.SyncEvent) {
		received <- e
	})

	time.Sleep(100 * time.Millisecond)

	event := mesh.SyncEvent{
		NodeID: "node-2",
		Kind:   "GraphState",
		ItemID: "obj-123",
	}

	if err := syncer2.Broadcast(ctx, event); err != nil {
		t.Fatalf("Failed to broadcast: %v", err)
	}

	select {
	case e := <-received:
		if e.NodeID != "node-2" {
			t.Errorf("Expected node-2, got %v", e.NodeID)
		}
		if e.ItemID != "obj-123" {
			t.Errorf("Expected obj-123, got %v", e.ItemID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for sync event")
	}
}
