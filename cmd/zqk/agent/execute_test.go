package agent

import (
	"strings"
	"testing"
)

func TestResolveExecuteSystemPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantPart string
	}{
		{
			name:     "empty prompt uses autonomous coding prompt",
			input:    "",
			wantPart: "autonomous software engineering agent in ZQK",
		},
		{
			name:     "default helpful assistant prompt overridden",
			input:    "You are a helpful assistant.",
			wantPart: "autonomous software engineering agent in ZQK",
		},
		{
			name:     "custom system prompt preserved",
			input:    "Custom prompt for testing.",
			wantPart: "Custom prompt for testing.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveExecuteSystemPrompt(tc.input)
			if !strings.Contains(got, tc.wantPart) {
				t.Fatalf("resolveExecuteSystemPrompt(%q) = %q, want containing %q", tc.input, got, tc.wantPart)
			}
		})
	}
}

func TestExecuteCommandFlags(t *testing.T) {
	t.Parallel()

	cmd := NewExecuteCmd()
	taskIDFlag := cmd.Flags().Lookup("task-id")
	if taskIDFlag == nil {
		t.Fatal("missing --task-id flag on agent execute")
	}
	promptFlag := cmd.Flags().Lookup("prompt")
	if promptFlag == nil {
		t.Fatal("missing --prompt flag on agent execute")
	}
	sysPromptFlag := cmd.Flags().Lookup("system-prompt")
	if sysPromptFlag == nil {
		t.Fatal("missing --system-prompt flag on agent execute")
	}
	mcpPathFlag := cmd.Flags().Lookup("mcp-path")
	if mcpPathFlag == nil {
		t.Fatal("missing --mcp-path flag on agent execute")
	}
}

func TestFormatTaskStepsSection(t *testing.T) {
	t.Parallel()

	t.Run("nil or empty steps", func(t *testing.T) {
		t.Parallel()
		if got := formatTaskStepsSection(nil); got != "" {
			t.Fatalf("expected empty string for nil steps, got %q", got)
		}
		if got := formatTaskStepsSection([]any{}); got != "" {
			t.Fatalf("expected empty string for empty slice, got %q", got)
		}
	})

	t.Run("structured task steps", func(t *testing.T) {
		t.Parallel()
		steps := []any{
			map[string]any{
				"title":       "Implementation Phase",
				"status":      "pending_implementation",
				"description": "Write code changes.",
			},
			map[string]any{
				"title":       "Validation Phase",
				"status":      "pending",
				"description": "Run test suite.",
				"command":     "./bin/zqk agent validate",
			},
		}
		got := formatTaskStepsSection(steps)
		if !strings.Contains(got, "Sequenced Task Steps") {
			t.Fatalf("expected header 'Sequenced Task Steps', got: %s", got)
		}
		if !strings.Contains(got, "1. **Implementation Phase** [pending_implementation]") {
			t.Fatalf("expected step 1 formatted, got: %s", got)
		}
		if !strings.Contains(got, "Verification Command: `./bin/zqk agent validate`") {
			t.Fatalf("expected verification command, got: %s", got)
		}
	})
}
