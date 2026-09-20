package bridge

import (
	"context"
	"testing"
)

func TestMemoryOntologyBridge(t *testing.T) {
	bridge := NewMemoryOntologyBridge(nil, nil)

	t.Run("Initialize connection", func(t *testing.T) {
		err := bridge.Connect(context.Background(), "sem-1", "graph-1")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}
