package scheduler

import (
	"context"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestEscalationProvider_Interface(t *testing.T) {
	// Verify that all providers implement the interface at compile time
	var _ EscalationProvider = &InboxEscalationProvider{}
	var _ EscalationProvider = &CommandEscalationProvider{}
}

func TestInboxEscalationProvider_Escalate(t *testing.T) {
	dir := t.TempDir()
	provider := &InboxEscalationProvider{
		InboxDir: dir,
	}

	notice := EscalationNotice{
		Title:    "Test Escalation",
		Severity: EscalationSeverityWarning,
		Issues:   []string{"test issue 1", "test issue 2"},
		Context: map[string]any{
			"consecutive_failures": 3,
			"last_stage":           "cap_stage_review",
		},
		SuggestedActions: []string{"run diagnostics"},
	}

	err := provider.Escalate(context.Background(), notice)
	if err != nil {
		t.Fatalf("Escalate failed: %v", err)
	}

	// Verify file was written
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read inbox dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 file in inbox, got %d", len(entries))
	}
}

func TestCommandEscalationProvider_Escalate(t *testing.T) {
	provider := &CommandEscalationProvider{
		Command: "echo",
		Args:    []string{"escalation received"},
	}

	notice := EscalationNotice{
		Title:    "Test Escalation",
		Severity: EscalationSeverityCritical,
		Issues:   []string{"test issue"},
	}

	err := provider.Escalate(context.Background(), notice)
	if err != nil {
		t.Fatalf("Escalate failed: %v", err)
	}
}

func TestCommandEscalationProvider_PromptFileSubstitution(t *testing.T) {
	dir := t.TempDir()
	provider := &CommandEscalationProvider{
		Command:     "cat",
		Args:        []string{"{prompt_file}"},
		ProjectRoot: dir,
	}

	notice := EscalationNotice{
		Title:    "Substitution Test",
		Severity: EscalationSeverityWarning,
		Issues:   []string{"issue"},
	}

	err := provider.Escalate(context.Background(), notice)
	if err != nil {
		t.Fatalf("Escalate failed: %v", err)
	}
}

func TestEscalationChain_TieredEscalation(t *testing.T) {
	var agentCalled, humanCalled bool

	agentProvider := &testEscalationProvider{onEscalate: func(_ EscalationNotice) error {
		agentCalled = true
		return nil
	}}
	humanProvider := &testEscalationProvider{onEscalate: func(_ EscalationNotice) error {
		humanCalled = true
		return nil
	}}

	chain := &EscalationChain{
		Tiers: []EscalationTier{
			{Threshold: 3, Provider: agentProvider, Name: "agent"},
			{Threshold: 6, Provider: humanProvider, Name: "human"},
		},
	}

	// Below threshold — no escalation
	chain.Evaluate(context.Background(), 2, EscalationNotice{Title: "test"})
	if agentCalled || humanCalled {
		t.Fatal("Should not escalate below threshold")
	}

	// At agent threshold — agent only
	chain.Evaluate(context.Background(), 3, EscalationNotice{Title: "test"})
	if !agentCalled {
		t.Fatal("Agent should be called at threshold 3")
	}
	if humanCalled {
		t.Fatal("Human should NOT be called at threshold 3")
	}

	// At human threshold — human called
	agentCalled = false
	chain.Evaluate(context.Background(), 6, EscalationNotice{Title: "test"})
	if !humanCalled {
		t.Fatal("Human should be called at threshold 6")
	}
}

// testEscalationProvider is a mock for testing.
type testEscalationProvider struct {
	onEscalate func(EscalationNotice) error
}

func (p *testEscalationProvider) Escalate(_ context.Context, n EscalationNotice) error {
	if p.onEscalate != nil {
		return p.onEscalate(n)
	}
	return nil
}
