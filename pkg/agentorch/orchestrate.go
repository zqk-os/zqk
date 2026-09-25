// Package agentorch provides legacy agent orchestration compatibility.
//
// Deprecated: Multi-agent execution runtimes and task dispatch are canonically
// provided by:
//   - pkg/orchestration: session management, intent synthesis, subagent runners, and worktrees.
//   - pkg/primaryorch: host and primary orchestrator resolution, waking, and IPC bindings.
//   - pkg/swarm: distributed agent worker pool execution and task claim dispatching.
//
// This package is retained as a compatibility shim and will be retired in v1.0.
package agentorch

import (
	"context"
	"sync"
)

// Engine executes multi-agent orchestration batches.
// Deprecated: Use pkg/orchestration.Manager or pkg/swarm.Dispatcher instead.
type Engine struct {
	mu      sync.Mutex
	agentID string
	active  map[string]bool
}

// NewEngine creates an orchestration engine.
func NewEngine(agentID string) *Engine {
	return &Engine{
		agentID: agentID,
		active:  make(map[string]bool),
	}
}

// Dispatch dispatches a task to an agent seat.
func (e *Engine) Dispatch(ctx context.Context, taskID, seatID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active[taskID] = true
	return nil
}

// IsActive returns whether a task is active.
func (e *Engine) IsActive(taskID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active[taskID]
}
