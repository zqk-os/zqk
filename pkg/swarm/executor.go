package swarm

import (
	"context"

	"github.com/zqk-os/zqk/pkg/llm"
)

// Executor defines the interface for executing tool calls.
type Executor interface {
	GetTools(ctx context.Context) ([]llm.ToolDefinition, error)
	ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error)
	Close() error
}
