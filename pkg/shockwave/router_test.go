package shockwave

import (
	"testing"
	"time"
)

func TestNewShockwaveRouter(t *testing.T) {
	router := NewShockwaveRouter("node-1")

	if router.NodeID != "node-1" {
		t.Errorf("Expected NodeID %s, got %s", "node-1", router.NodeID)
	}

	if router.Rules.MaxDepth != 5 {
		t.Errorf("Expected MaxDepth 5, got %d", router.Rules.MaxDepth)
	}

	if router.Splitters.ChunkSize != 1024 {
		t.Errorf("Expected ChunkSize 1024, got %d", router.Splitters.ChunkSize)
	}

	if !router.Duplicators.Enabled {
		t.Errorf("Expected Duplicators to be Enabled")
	}

	if router.ReduceStrategy.Timeout != 2*time.Second {
		t.Errorf("Expected ReduceStrategy timeout of 2s, got %v", router.ReduceStrategy.Timeout)
	}
}
