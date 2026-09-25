package adapter

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/agentdelivery"
)

// AntigravitySubagentCaller is a function type that calls the Antigravity invoke_subagent tool.
type AntigravitySubagentCaller func(ctx context.Context, typeName, role, prompt, model, workspace string) (string, error)

// AntigravityAdapter adapts SubagentInvocation to Antigravity's invoke_subagent interface.
type AntigravityAdapter struct {
	caller AntigravitySubagentCaller
}

// NewAntigravityAdapter creates a new Antigravity subagent invocation adapter.
func NewAntigravityAdapter(caller AntigravitySubagentCaller) *AntigravityAdapter {
	return &AntigravityAdapter{caller: caller}
}

func (a *AntigravityAdapter) Name() string {
	return "antigravity"
}

func (a *AntigravityAdapter) Invoke(ctx context.Context, invocation agentdelivery.SubagentInvocation) (agentdelivery.Result, error) {
	if a.caller == nil {
		return agentdelivery.Result{}, fmt.Errorf("antigravity adapter: caller function not configured")
	}
	typeName := invocation.TypeName
	if typeName == "" {
		typeName = "self"
	}
	role := invocation.Role
	if role == "" {
		role = "Subagent Worker"
	}
	model := invocation.Model
	if model == "" {
		model = "inherit"
	}
	workspace := invocation.Workspace
	if workspace == "" {
		workspace = "inherit"
	}

	res, err := a.caller(ctx, typeName, role, invocation.Prompt, model, workspace)
	if err != nil {
		return agentdelivery.Result{}, fmt.Errorf("antigravity invoke_subagent failed: %w", err)
	}

	return agentdelivery.Result{
		DeliveredTo:    fmt.Sprintf("antigravity:%s:%s (%s)", typeName, role, res),
		HTTPStatusCode: 200,
	}, nil
}
