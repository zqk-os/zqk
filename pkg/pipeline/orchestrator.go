package pipeline

import (
	"context"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// ObjectStore defines the minimal interface required by the orchestrator to read/write state.
type ObjectStore interface {
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, data map[string]any) error
}

// PipelinePlugin defines the contract for any executable stage in a pipeline.
type PipelinePlugin interface {
	// Name returns the unique string identifier for the plugin
	Name() string

	// Execute runs the plugin logic, mutating the payload or initiating a subagent
	Execute(ctx context.Context, payload map[string]any) (map[string]any, error)

	// Validate ensures the output of this plugin satisfies the next stage's requirements
	Validate(ctx context.Context, output map[string]any) error
}

// Orchestrator manages the execution of multi-agent pipelines based on pipeline_execution records.
type Orchestrator struct {
	store   ObjectStore
	plugins map[string]PipelinePlugin
}

// NewOrchestrator creates a new multi-agent pipeline orchestrator.
func NewOrchestrator(store ObjectStore) *Orchestrator {
	return &Orchestrator{
		store:   store,
		plugins: make(map[string]PipelinePlugin),
	}
}

// RegisterPlugin adds a new plugin to the orchestrator's registry.
func (o *Orchestrator) RegisterPlugin(p PipelinePlugin) {
	o.plugins[p.Name()] = p
}

// Advance processes a single state transition for the given pipeline_execution object.
// The execution logic follows the state machine:
// Pending -> PreProcessing -> Orchestrating (AgentDispatch -> AgentExecution -> HandoffVerification) -> PostProcessing
func (o *Orchestrator) Advance(ctx context.Context, secCtx *pkgctx.SecurityContext, executionID string) error {
	execObj, err := o.store.Read(ctx, secCtx, executionID)
	if err != nil {
		return fmt.Errorf("failed to read pipeline_execution %s: %w", executionID, err)
	}
	if objects.GetString(execObj, objects.FieldKeyKind) != "pipeline_execution" {
		return fmt.Errorf("object %s is not a pipeline_execution", executionID)
	}

	stage := objects.GetString(execObj, objects.FieldKeyCurrentStage)
	if stage == "" {
		stage = "pending"
	}

	payload, ok := execObj[objects.FieldKeyContextPayload].(map[string]any)
	if !ok || payload == nil {
		payload = make(map[string]any)
	}

	var nextStage string

	switch stage {
	case "pending":
		nextStage = "pre_processing"
	case "pre_processing":
		nextStage = "agent_dispatch"
	case "agent_dispatch":
		nextStage = "agent_execution"
	case "agent_execution":
		// Yielding execution back to the orchestrator to verify the output
		nextStage = "handoff_verification"
	case "handoff_verification":
		// Could transition to post_processing or back to agent_dispatch
		nextStage = "post_processing"
	case "post_processing":
		nextStage = "success"
	case "success", "failure":
		// Terminal states
		return nil
	default:
		nextStage = "failure"
	}

	updates := map[string]any{
		objects.FieldKeyCurrentStage:   nextStage,
		objects.FieldKeyContextPayload: payload,
	}

	return o.store.Update(ctx, secCtx, executionID, updates)
}
