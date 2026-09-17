package impl

import (
	"context"
	"testing"
)

func TestBridge(t *testing.T) {
	bridge := NewBridge(nil, nil)

	t.Run("Connect semantic to structural", func(t *testing.T) {
		err := bridge.Connect(context.Background(), "sem-1", "graph-1")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}
