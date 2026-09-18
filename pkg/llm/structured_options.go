package llm

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// OpenAI-compatible tool_choice values. Empty means omit the field (provider default).
// TRACK: BLI-SWM-002 — 7B native function-calling; markdown recovery is the self-heal.
const (
	ToolChoiceAuto     = "auto"
	ToolChoiceRequired = "required"
	ToolChoiceNone     = "none"
)

// StructuredCompletionOptions is the per-turn tool-calling contract.
type StructuredCompletionOptions struct {
	// ToolChoice is auto|required|none. Empty omits the field.
	ToolChoice string
	// ToolChoiceFunction pins one function when the provider accepts
	// tool_choice but ignores the string "required" (typical 7B local
	// OpenAI-compat). Empty keeps the string form.
	ToolChoiceFunction string
}

// StructuredOptionsClient is implemented by clients that can pin tool_choice.
// Engine type-asserts this; mocks that only implement Client keep working.
type StructuredOptionsClient interface {
	GenerateStructuredCompletionWithOptions(ctx context.Context, messages []Message, tools []ToolDefinition, opts StructuredCompletionOptions) (StructuredCompletionResponse, error)
}

// StructuredCompletion prefers WithOptions when the client supports it.
func StructuredCompletion(ctx context.Context, client Client, messages []Message, tools []ToolDefinition, opts StructuredCompletionOptions) (StructuredCompletionResponse, error) {
	if client == nil {
		return StructuredCompletionResponse{}, errfmt.Errorf("LLM client is nil")
	}
	if c, ok := client.(StructuredOptionsClient); ok {
		return c.GenerateStructuredCompletionWithOptions(ctx, messages, tools, opts)
	}
	return client.GenerateStructuredCompletion(ctx, messages, tools)
}
