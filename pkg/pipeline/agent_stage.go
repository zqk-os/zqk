package pipeline

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/pipeline/plugins"
)

// AgentDispatcher defines the interface for dispatching a task to an autonomous agent.
// By abstracting the dispatcher, the pipeline remains decoupled from specific agent runtimes
// (e.g., Native Swarm sync-loops, MCP instances, or external LLM APIs).
type AgentDispatcher interface {
	// Dispatch sends the task payload to the agent and returns the result or an updated state.
	Dispatch(ctx context.Context, payload any) (any, error)
}

// AgentStageOptions configures the behavior of an AgentStage.
type AgentStageOptions struct {
	// Dispatcher is the mechanism used to send the task to the agent.
	Dispatcher AgentDispatcher

	// TaskBuilder optionally transforms the pipeline payload into the specific
	// format required by the dispatcher before sending.
	TaskBuilder func(pctx *Context, payload any) (any, error)
}

// AgentStage creates a StageFunc that delegates work to an external agent.
// This enables native multi-agent orchestration pipelines within the Knowledge Kernel.
func AgentStage(opts AgentStageOptions) StageFunc {
	statePlugin := plugins.NewGraphExclusiveStatePlugin()

	return func(pctx *Context, payload any) (any, error) {
		if opts.Dispatcher == nil {
			return payload, fmt.Errorf("AgentStage requires a valid Dispatcher")
		}

		dispatchPayload := payload
		if opts.TaskBuilder != nil {
			var err error
			dispatchPayload, err = opts.TaskBuilder(pctx, payload)
			if err != nil {
				return payload, fmt.Errorf("AgentStage task building failed: %w", err)
			}
		}

		if payloadMap, ok := dispatchPayload.(map[string]any); ok {
			validatedPayload, err := statePlugin.Execute(pctx.Ctx, payloadMap)
			if err != nil {
				return payload, fmt.Errorf("AgentStage state tracking violation: %w", err)
			}
			dispatchPayload = validatedPayload
		}

		result, err := opts.Dispatcher.Dispatch(pctx.Ctx, dispatchPayload)
		if err != nil {
			return payload, fmt.Errorf("AgentStage dispatch failed: %w", err)
		}

		return result, nil
	}
}
