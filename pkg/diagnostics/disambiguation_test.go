package diagnostics_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/diagnostics"
)

func TestGenerateDisambiguation_FirstAttemptEmpty(t *testing.T) {
	opts := diagnostics.DefaultDisambiguationOptions()
	ctx := diagnostics.GenerateDisambiguation(nil, opts)

	if ctx.Attempt != 1 {
		t.Errorf("expected attempt 1 for empty history, got %d", ctx.Attempt)
	}
	if len(ctx.NegativeConstraints) != 0 {
		t.Errorf("expected 0 negative constraints, got %d", len(ctx.NegativeConstraints))
	}

	prompt := diagnostics.FormatDisambiguationPrompt(ctx)
	if prompt != "" {
		t.Errorf("expected empty prompt for attempt 1 without history, got %q", prompt)
	}
}

func TestGenerateDisambiguation_ProgressiveNegativeConstraints(t *testing.T) {
	env1 := diagnostics.ExtractFailureEnvelope(
		1,
		1,
		errors.New("build failed with syntax error in handler"),
		[]string{"run_command go test ./cmd/...", "edit_file pkg/handler.go"},
		[]string{"zero syntax errors required"},
		nil,
	)

	opts := diagnostics.DefaultDisambiguationOptions()
	ctx := diagnostics.GenerateDisambiguation([]diagnostics.FailureEnvelope{env1}, opts)

	if ctx.Attempt != 2 {
		t.Errorf("expected attempt 2, got %d", ctx.Attempt)
	}
	if len(ctx.NegativeConstraints) != 2 {
		t.Fatalf("expected 2 negative constraints, got %d", len(ctx.NegativeConstraints))
	}

	// Should contain the actions that failed
	prompt := diagnostics.FormatDisambiguationPrompt(ctx)
	if !strings.Contains(prompt, "Adaptive Retry Guidance (Attempt 2)") {
		t.Error("prompt should mention Attempt 2 header")
	}
	if !strings.Contains(prompt, "Negative Constraints") {
		t.Error("prompt should include Negative Constraints section")
	}
	if !strings.Contains(prompt, "edit_file pkg/handler.go") {
		t.Error("prompt should mention failing action as a negative constraint")
	}
	if !strings.Contains(prompt, "zero syntax errors required") {
		t.Error("prompt should mention failing constraint in guidance")
	}
}

func TestGenerateDisambiguation_ZeroEntropyDelta(t *testing.T) {
	// Attempt 1 and Attempt 2 repeat the exact same failure
	env1 := diagnostics.ExtractFailureEnvelope(
		1,
		1,
		errors.New("connection refused to daemon"),
		[]string{"start_daemon"},
		nil,
		nil,
	)
	env2 := diagnostics.ExtractFailureEnvelope(
		2,
		1,
		errors.New("connection refused to daemon"),
		[]string{"start_daemon"},
		nil,
		nil,
	)

	opts := diagnostics.DefaultDisambiguationOptions()
	ctx := diagnostics.GenerateDisambiguation([]diagnostics.FailureEnvelope{env1, env2}, opts)

	if len(ctx.Deltas) != 1 {
		t.Fatalf("expected 1 delta record, got %d", len(ctx.Deltas))
	}
	if ctx.Deltas[0].EntropyIntroduced {
		t.Error("expected zero entropy for identical failure")
	}
	if !strings.Contains(ctx.Deltas[0].DifferentialSummary, "Zero diagnostic delta") {
		t.Errorf("expected zero diagnostic delta summary, got %q", ctx.Deltas[0].DifferentialSummary)
	}

	prompt := diagnostics.FormatDisambiguationPrompt(ctx)
	if !strings.Contains(prompt, "Prior attempt produced zero new evidence") {
		t.Error("prompt should advise pivoting when zero new evidence was introduced")
	}
}

func TestFormatDisambiguationPrompt_BoundedContext(t *testing.T) {
	env1 := diagnostics.ExtractFailureEnvelope(
		1,
		1,
		errors.New("error message"),
		[]string{"action1", "action2", "action3"},
		nil,
		nil,
	)

	opts := diagnostics.DisambiguationOptions{
		MaxPromptBytes:   100,
		MaxNegativeRules: 5,
		MaxGuidanceItems: 2,
	}
	ctx := diagnostics.GenerateDisambiguation([]diagnostics.FailureEnvelope{env1}, opts)
	prompt := diagnostics.FormatDisambiguationPrompt(ctx)

	if len(prompt) > 100 {
		t.Errorf("prompt length %d exceeded max bound 100", len(prompt))
	}
}

func TestGenerateDisambiguation_DeterministicOutput(t *testing.T) {
	env1 := diagnostics.ExtractFailureEnvelope(
		1,
		1,
		errors.New("error A"),
		[]string{"action_b", "action_a", "action_c"},
		nil,
		nil,
	)

	opts := diagnostics.DefaultDisambiguationOptions()
	ctx1 := diagnostics.GenerateDisambiguation([]diagnostics.FailureEnvelope{env1}, opts)
	ctx2 := diagnostics.GenerateDisambiguation([]diagnostics.FailureEnvelope{env1}, opts)

	prompt1 := diagnostics.FormatDisambiguationPrompt(ctx1)
	prompt2 := diagnostics.FormatDisambiguationPrompt(ctx2)

	if prompt1 != prompt2 {
		t.Errorf("expected identical prompt formatting, got %q vs %q", prompt1, prompt2)
	}
}
