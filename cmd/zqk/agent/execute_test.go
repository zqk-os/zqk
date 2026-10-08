package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
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
				"title":                "Implementation Phase",
				objects.FieldKeyStatus: objects.ObjectStatusPendingImplementation,
				"description":          "Write code changes.",
			},
			map[string]any{
				"title":                "Validation Phase",
				objects.FieldKeyStatus: objects.ObjectStatusPending,
				"description":          "Run test suite.",
				"command":              "./bin/zqk agent validate",
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

func TestExecuteCmd_ExecutionInputValidation(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing both prompt and task ID
	_, err := executeAgentCommand(t, tempDir, provider, "execute")
	assert.ErrorContains(t, err, "requires --prompt, --task-id, or a task ID argument")

	// 2. Non-existent task ID
	_, err = executeAgentCommand(t, tempDir, provider, "execute", "ATK-NONEXISTENT")
	assert.ErrorContains(t, err, "failed to read task")
}

func TestExecuteCmd_Branches(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Non-agent-task kind (e.g. backlog_item) builds prompt via BuildTaskPrompt
	bliID := "BLI-EXEC-BRANCH-001"
	bli := map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Backlog Item for Execution",
		objects.FieldKeyDescription:     "Build a new feature in Go.",
		objects.FieldKeyStatus:          objects.ObjectStatusOriginated,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyPriorityPlanRef: "PRI-TEST-PLAN",
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, bli))
	_, err := executeAgentCommand(t, tempDir, provider, "execute", bliID, "--mcp-path", "/nonexistent/mcp")
	assert.ErrorContains(t, err, "failed to initialize MCP executor")

	// 2. Task with execution prompt and MCP failure
	taskWithPromptID := "ATK-WITH-PROMPT"
	taskWithPrompt := map[string]any{
		objects.FieldKeyID:                 taskWithPromptID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "With Prompt Task",
		objects.FieldKeyDescription:        "Execution instructions for agent worker.",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyTitle:  "Step 1",
				objects.FieldKeyStatus: objects.ObjectStatusApproved,
			},
		},
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, taskWithPrompt))
	_, err = executeAgentCommand(t, tempDir, provider, "execute", taskWithPromptID, "--mcp-path", "/nonexistent/mcp")
	assert.ErrorContains(t, err, "failed to initialize MCP executor")

	// 3. Task with Task Envelope
	taskEnvID := "ATK-WITH-ENVELOPE"
	taskEnv := map[string]any{
		objects.FieldKeyID:                 taskEnvID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Envelope Task",
		objects.FieldKeyDescription:        "zqk_task_envelope_v1\nContext: test",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, taskEnv))
	_, err = executeAgentCommand(t, tempDir, provider, "execute", taskEnvID, "--mcp-path", "/nonexistent/mcp")
	assert.ErrorContains(t, err, "failed to initialize MCP executor")

	// 4. Prompt only with MCP failure
	_, err = executeAgentCommand(t, tempDir, provider, "execute", "--prompt", "do work", "--mcp-path", "/nonexistent/mcp")
	assert.ErrorContains(t, err, "failed to initialize MCP executor")
}
