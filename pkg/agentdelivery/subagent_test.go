package agentdelivery_test

import (
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentdelivery"
	"github.com/zqk-os/zqk/pkg/agentdelivery/adapter"
	"github.com/zqk-os/zqk/pkg/policy"
)

func TestSubagentDeliverer_ContextParityViolation(t *testing.T) {
	invoker := agentdelivery.SubagentInvokerFunc(func(ctx context.Context, invocation agentdelivery.SubagentInvocation) (agentdelivery.Result, error) {
		return agentdelivery.Result{DeliveredTo: "ok"}, nil
	})

	deliverer := agentdelivery.NewSubagentDeliverer(invoker)

	// Empty prompt markdown violates context parity
	_, err := deliverer.Deliver(context.Background(), agentdelivery.Prompt{
		Markdown: []byte(""),
	})
	if err == nil {
		t.Fatal("expected error on empty markdown, got nil")
	}
	if !strings.Contains(err.Error(), "context parity violation") {
		t.Fatalf("expected context parity violation error, got: %v", err)
	}
}

func TestSubagentDeliverer_SuccessfulDeliveryWithTDE(t *testing.T) {
	var captured agentdelivery.SubagentInvocation

	invoker := agentdelivery.SubagentInvokerFunc(func(ctx context.Context, invocation agentdelivery.SubagentInvocation) (agentdelivery.Result, error) {
		captured = invocation
		return agentdelivery.Result{DeliveredTo: "mock:subagent"}, nil
	})

	deliverer := agentdelivery.NewSubagentDeliverer(invoker)
	tde := &policy.TrustDomainEnvelope{
		ID:    "TDE-123",
		Scope: "agent_orchestration",
	}

	prompt := agentdelivery.Prompt{
		Markdown:  []byte("# Orchestration Context\nTask instructions here"),
		SessionID: "sess-999",
		TDE:       tde,
	}

	res, err := deliverer.Deliver(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeliveredTo != "mock:subagent" {
		t.Fatalf("expected DeliveredTo mock:subagent, got %s", res.DeliveredTo)
	}

	if captured.Prompt != "# Orchestration Context\nTask instructions here" {
		t.Fatalf("expected prompt context parity, got %s", captured.Prompt)
	}

	if captured.SessionID != "sess-999" {
		t.Fatalf("expected session ID sess-999, got %s", captured.SessionID)
	}

	if captured.Metadata["tde_id"] != "TDE-123" {
		t.Fatalf("expected tde_id in metadata, got %v", captured.Metadata["tde_id"])
	}
}

func TestAntigravityAdapter_Invoke(t *testing.T) {
	var calledType, calledRole, calledPrompt, calledModel, calledWorkspace string

	caller := func(ctx context.Context, typeName, role, prompt, model, workspace string) (string, error) {
		calledType = typeName
		calledRole = role
		calledPrompt = prompt
		calledModel = model
		calledWorkspace = workspace
		return "subagent-conv-1", nil
	}

	agAdapter := adapter.NewAntigravityAdapter(caller)
	deliverer := agentdelivery.NewSubagentDeliverer(agAdapter)

	prompt := agentdelivery.Prompt{
		Markdown:  []byte("## Full Task Context"),
		SessionID: "sess-1",
	}

	res, err := deliverer.Deliver(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected delivery error: %v", err)
	}

	if !strings.Contains(res.DeliveredTo, "antigravity:self:Subagent Worker") {
		t.Fatalf("expected deliveredTo to mention antigravity, got: %s", res.DeliveredTo)
	}

	if calledPrompt != "## Full Task Context" {
		t.Fatalf("expected context parity in caller, got: %s", calledPrompt)
	}

	if calledType != "self" || calledRole != "Subagent Worker" {
		t.Fatalf("expected self and Subagent Worker, got type=%s role=%s", calledType, calledRole)
	}

	if calledModel != "inherit" || calledWorkspace != "inherit" {
		t.Fatalf("expected inherit for model/workspace, got model=%s workspace=%s", calledModel, calledWorkspace)
	}
}
