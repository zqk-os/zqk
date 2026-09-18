package swarm

import (
	"context"

	"github.com/zqk-os/zqk/pkg/llm"
)

// AgentHook allows decoupled observers to intercept the agent's LLM interaction loop.
type AgentHook interface {
	// PreTool is called before a tool executes. Returning an error aborts the swarm run.
	PreTool(ctx context.Context, call llm.ToolCall) error
	
	// PostTool is called after a tool executes. It can return an optional steering prompt
	// to be injected into the context window, or an error to abort the swarm run.
	PostTool(ctx context.Context, call llm.ToolCall, result string, err error) (string, error)
	
	// OnSuccess is called asynchronously after a successful tool execution.
	OnSuccess(ctx context.Context, call llm.ToolCall, result string)
}
