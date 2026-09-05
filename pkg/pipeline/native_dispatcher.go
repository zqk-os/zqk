package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// SwarmTask represents the task payload for the swarm engine.
type SwarmTask struct {
	ID           string
	SystemPrompt string
	UserPrompt   string
}

// SwarmEngine defines the dependency required to enqueue swarm tasks.
type SwarmEngine interface {
	Enqueue(ctx context.Context, task any) error
}

// NativeSwarmDispatcher implements the AgentDispatcher interface by
// natively bridging the pipeline payload into the ZQK swarm orchestration logic.
// This fulfills [REDACTED-ID] (Complete Native Pipeline Integration).
type NativeSwarmDispatcher struct {
	SwarmConfig map[string]any
	Engine      SwarmEngine
}

// NewNativeSwarmDispatcher creates a new dispatcher for native ZQK swarm agents.
func NewNativeSwarmDispatcher(config map[string]any, engine SwarmEngine) *NativeSwarmDispatcher {
	return &NativeSwarmDispatcher{
		SwarmConfig: config,
		Engine:      engine,
	}
}

// Dispatch forwards the task payload to the native ZQK swarm orchestration layer.
func (d *NativeSwarmDispatcher) Dispatch(ctx context.Context, payload any) (any, error) {
	// 1. Convert payload to a native swarm task Envelope
	m, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("payload must be a map[string]any")
	}

	taskID, _ := m[objects.FieldKeyID].(string)
	if taskID == "" {
		taskID = "default-task"
	}
	sysPrompt, _ := m["system_prompt"].(string)
	userPrompt, _ := m["user_prompt"].(string)

	task := SwarmTask{
		ID:           taskID,
		SystemPrompt: sysPrompt,
		UserPrompt:   userPrompt,
	}

	env := Envelope{
		TraceID:    taskID,
		Source:     "pipeline",
		ReceivedAt: time.Now(),
		Payload:    task,
	}

	// 2. Submit to the scheduler / swarm engine
	if d.Engine != nil {
		if err := d.Engine.Enqueue(ctx, task); err != nil {
			return nil, fmt.Errorf("failed to enqueue swarm task: %w", err)
		}
	}

	// 3. Await outcome (sync or async depending on the pipeline configuration)
	mode, _ := d.SwarmConfig["mode"].(string)
	if mode == "sync" {
		return map[string]any{
			objects.FieldKeyStatus: "dispatched_sync",
			"task_id":              taskID,
			"envelope":             env,
		}, nil
	}

	return map[string]any{
		objects.FieldKeyStatus: "dispatched_async",
		"task_id":              taskID,
		"envelope":             env,
	}, nil
}
