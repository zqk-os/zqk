package agentdelivery

import (
	"context"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/policy"
)

// SubagentInvocation contains the normalized, vendor-neutral parameters for launching a subagent.
type SubagentInvocation struct {
	TaskID    string                      `json:"task_id"`
	TypeName  string                      `json:"type_name"`
	Role      string                      `json:"role"`
	Prompt    string                      `json:"prompt"`
	Model     string                      `json:"model,omitempty"`
	Workspace string                      `json:"workspace,omitempty"`
	SessionID string                      `json:"session_id,omitempty"`
	TDE       *policy.TrustDomainEnvelope `json:"tde,omitempty"`
	Metadata  map[string]any              `json:"metadata,omitempty"`
}

// SubagentInvoker is a pluggable adapter interface that dispatches an invocation
// to an external agent execution runtime (e.g. IDE subagent tool, background process, RPC).
type SubagentInvoker interface {
	Name() string
	Invoke(ctx context.Context, invocation SubagentInvocation) (Result, error)
}

// SubagentInvokerFunc is a functional adapter for SubagentInvoker.
type SubagentInvokerFunc func(ctx context.Context, invocation SubagentInvocation) (Result, error)

func (f SubagentInvokerFunc) Name() string { return "subagent-func" }
func (f SubagentInvokerFunc) Invoke(ctx context.Context, invocation SubagentInvocation) (Result, error) {
	return f(ctx, invocation)
}

var (
	// DefaultSubagentDeliverer is the registered runtime deliverer for subagent execution targets.
	DefaultSubagentDeliverer Deliverer
)

// SubagentDeliverer routes tasks to subagents while strictly enforcing context parity with local LLM execution.
type SubagentDeliverer struct {
	Invoker          SubagentInvoker
	DefaultTypeName  string
	DefaultRole      string
	DefaultModel     string
	DefaultWorkspace string
}

// NewSubagentDeliverer creates a new SubagentDeliverer wrapping a SubagentInvoker.
func NewSubagentDeliverer(invoker SubagentInvoker) *SubagentDeliverer {
	return &SubagentDeliverer{
		Invoker:          invoker,
		DefaultTypeName:  "self",
		DefaultRole:      "Subagent Worker",
		DefaultModel:     "inherit",
		DefaultWorkspace: "inherit",
	}
}

func (d *SubagentDeliverer) Name() string {
	if d.Invoker != nil {
		return "subagent:" + d.Invoker.Name()
	}
	return "subagent"
}

// Deliver dispatches the prompt to the subagent runtime. It verifies that prompt context
// is not empty and preserves the complete bounded context and governance envelope.
func (d *SubagentDeliverer) Deliver(ctx context.Context, p Prompt) (Result, error) {
	if d.Invoker == nil {
		return Result{}, errfmt.Errorf("subagent delivery: invoker adapter is not configured")
	}

	promptStr := strings.TrimSpace(string(p.Markdown))
	if promptStr == "" {
		return Result{}, errfmt.Errorf("subagent delivery: context parity violation: prompt markdown cannot be empty")
	}

	invocation := SubagentInvocation{
		TypeName:  d.DefaultTypeName,
		Role:      d.DefaultRole,
		Prompt:    promptStr,
		Model:     d.DefaultModel,
		Workspace: d.DefaultWorkspace,
		SessionID: p.SessionID,
		TDE:       p.TDE,
		Metadata:  make(map[string]any),
	}

	if p.TDE != nil {
		invocation.Metadata["tde_id"] = p.TDE.ID
		invocation.Metadata["tde_scope"] = p.TDE.Scope
	}

	return d.Invoker.Invoke(ctx, invocation)
}
