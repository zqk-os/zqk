package supervision

import (
	"strings"
	"testing"
)

func TestExtractFailureAnchor(t *testing.T) {
	cases := []struct {
		name     string
		stderr   string
		stdout   string
		expected string
	}{
		{
			name:     "Go test failure",
			stderr:   "",
			stdout:   "=== RUN   TestValidation\n--- FAIL: TestValidation (0.01s)\nFAIL\n",
			expected: "TestValidation",
		},
		{
			name:     "Compiler error anchor",
			stderr:   "pkg/storage/store.go:124:18: undefined: TargetModel\n",
			stdout:   "",
			expected: "pkg/storage/store.go:124:18",
		},
		{
			name:     "Panic message",
			stderr:   "panic: runtime error: index out of range [5] with length 2\n",
			stdout:   "",
			expected: "runtime error: index out of range [5] with length 2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractFailureAnchor(tc.stderr, tc.stdout)
			if got != tc.expected {
				t.Errorf("ExtractFailureAnchor() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestExtractFocusedDiagnosticFiltering(t *testing.T) {
	noisyStderr := `
[BUILD] Starting compilation with 12 workers...
[INFO] Loading dependencies from vendor cache
[WARN] Deprecated flag used in build configuration
pkg/core/handler.go:45:12: cannot use val (variable of type int) as string in argument to format
[INFO] Generating intermediate artifacts
[INFO] Build step 4 completed
`
	focused := ExtractFocusedDiagnostic(noisyStderr, "", 1, 10)
	if !strings.Contains(focused, "cannot use val") {
		t.Errorf("expected focused diagnostic to extract compiler error line, got: %s", focused)
	}
	if strings.Contains(focused, "Starting compilation with 12 workers") {
		t.Errorf("expected noise to be stripped out, got: %s", focused)
	}
}

func TestFormatContrastiveRetryPrompt(t *testing.T) {
	basePrompt := "# Task: Implement Agnostic Event Streamer"
	env := SupervisionEnvelope{
		TaskID:      "ATK-100",
		Attempt:     2,
		MaxAttempts: 3,
		Diagnostic: &FailureDiagnostic{
			Attempt:           1,
			ExitCode:          1,
			Phase:             PhaseCompile,
			FailureAnchor:     "pkg/streamer/event.go:88:5",
			FocusedDiagnostic: "pkg/streamer/event.go:88:5: undefined: EventType",
			ObservedAnomaly:   "Compilation failed due to missing symbol definition.",
		},
		NegativeBoundaries: []NegativeHypothesis{
			{
				Attempt:             1,
				HypothesisSummary:   "Inlining EventType string literal caused type mismatch.",
				InvalidatedApproach: "Direct type assertion to string failed against the EventType enum contract.",
				ForbiddenActions:    []string{"Do not cast raw interface{} directly to string without type switch."},
			},
		},
		Exemplars: []CanonicalExemplar{
			{
				Category:    "enums",
				Title:       "Idiomatic Go String Enum",
				Description: "Define typed string constants with stringer implementation.",
				PatternCode: "type EventType string\nconst EventCreated EventType = \"created\"",
			},
		},
		ProgressiveScope: &ProgressiveScope{
			SuggestedSubGateCommand: "go test -v ./pkg/streamer/...",
			FocusedTarget:           "pkg/streamer/event.go",
			IterationRationale:      "Compile and verify streamer package before running whole-repo integration tests.",
		},
	}

	result := FormatContrastiveRetryPrompt(basePrompt, env)

	// Verify all 5 pillars are present in formatted prompt
	if !strings.Contains(result, "Attempt 2 of 3") {
		t.Errorf("missing attempt indicator")
	}
	if !strings.Contains(result, "Focused Diagnostic Delta") {
		t.Errorf("missing Diagnostic Delta pillar")
	}
	if !strings.Contains(result, "pkg/streamer/event.go:88:5: undefined: EventType") {
		t.Errorf("missing focused error snippet")
	}
	if !strings.Contains(result, "Negative Hypotheses (Invalidated Paths to Avoid)") {
		t.Errorf("missing Negative Hypothesis pillar")
	}
	if !strings.Contains(result, "Canonical Reference Exemplars") {
		t.Errorf("missing Canonical Exemplar pillar")
	}
	if !strings.Contains(result, "Progressive Scope Narrowing") {
		t.Errorf("missing Progressive Scope pillar")
	}
	if !strings.Contains(result, "go test -v ./pkg/streamer/...") {
		t.Errorf("missing sub-gate command")
	}
}
