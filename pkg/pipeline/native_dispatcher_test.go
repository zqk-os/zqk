package pipeline

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

type mockEngine struct {
	tasks []any
}

func (m *mockEngine) Enqueue(ctx context.Context, task any) error {
	m.tasks = append(m.tasks, task)
	return nil
}

func TestNativeSwarmDispatcher(t *testing.T) {
	config := map[string]any{"mode": "sync"}
	engine := &mockEngine{}
	dispatcher := NewNativeSwarmDispatcher(config, engine)

	if dispatcher.SwarmConfig["mode"] != "sync" {
		t.Errorf("Expected config mode sync, got %v", dispatcher.SwarmConfig["mode"])
	}

	payload := map[string]any{
		objects.FieldKeyID: "test-task",
		"system_prompt":    "You are a helpful assistant.",
		"user_prompt":      "Hello",
	}

	res, err := dispatcher.Dispatch(context.Background(), payload)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if resMap, ok := res.(map[string]any); ok {
		if resMap[objects.FieldKeyStatus] != "dispatched_sync" {
			t.Errorf("Expected status dispatched_sync, got %v", resMap[objects.FieldKeyStatus])
		}
	} else {
		t.Errorf("Expected result to be map[string]any")
	}

	if len(engine.tasks) != 1 {
		t.Errorf("Expected queue size 1, got %d", len(engine.tasks))
	}
}
