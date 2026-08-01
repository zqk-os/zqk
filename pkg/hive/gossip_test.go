package hive

import (
	"context"
	"testing"
)

func TestGossipProtocol_Broadcast(t *testing.T) {
	t.Run("successful broadcast", func(t *testing.T) {
		gossip := NewGossipProtocol("node-1")
		err := gossip.Broadcast(context.Background(), []byte("test context"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
