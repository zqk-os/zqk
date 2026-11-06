package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestNewPrepareContextCmd(t *testing.T) {
	cmd := NewPrepareContextCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil prepare context command")
	}
	if cmd.Flags().Lookup("persona-ref") == nil {
		t.Errorf("expected --persona-ref flag")
	}
	if cmd.Flags().Lookup("description") == nil {
		t.Errorf("expected --description flag")
	}
	if cmd.Flags().Lookup("task") == nil {
		t.Errorf("expected --task flag")
	}
	if cmd.Flags().Lookup("task-id") == nil {
		t.Errorf("expected --task-id flag")
	}
}

func TestPrepareContextCmd_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--persona-ref", "PER-TESTER",
		"--description", "Sample test task description",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("unexpected error executing prepare-context: %v (out: %s)", err, out)
	}

	if !strings.Contains(out, "prompt") {
		t.Errorf("expected output to contain prompt key, got: %s", out)
	}
}

func TestPrepareContextCmd_PositionalAndFlagAliases(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	taskObj := map[string]any{
		objects.FieldKeyID:                 "ATK-TEST-PREPARE-100",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Test Prepare Context Task",
		objects.FieldKeyDescription:        "TASK_ENVELOPE\nDetailed task description for testing context hydration",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, taskObj))

	// 1. Positional invocation: zqk agent prepare-context <ATK>
	outPos, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"ATK-TEST-PREPARE-100",
		"--format", "json",
	)
	require.NoError(t, err)
	assert.Contains(t, outPos, "ATK-TEST-PREPARE-100")
	assert.Contains(t, outPos, "prompt")

	// 2. Flag invocation with --task-id: zqk agent prepare-context --task-id <ATK>
	outTaskID, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--task-id", "ATK-TEST-PREPARE-100",
		"--format", "json",
	)
	require.NoError(t, err)
	assert.Contains(t, outTaskID, "ATK-TEST-PREPARE-100")
	assert.Equal(t, outPos, outTaskID)

	// 3. Flag invocation with --task: zqk agent prepare-context --task <ATK>
	outTask, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--task", "ATK-TEST-PREPARE-100",
		"--format", "json",
	)
	require.NoError(t, err)
	assert.Contains(t, outTask, "ATK-TEST-PREPARE-100")
	assert.Equal(t, outPos, outTask)

	// 4. Positional and matching flag: zqk agent prepare-context <ATK> --task <ATK>
	outMatch, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"ATK-TEST-PREPARE-100",
		"--task", "ATK-TEST-PREPARE-100",
		"--format", "json",
	)
	require.NoError(t, err)
	assert.Equal(t, outPos, outMatch)

	// 5. Positional takes precedence when flag differs
	taskObj2 := map[string]any{
		objects.FieldKeyID:                 "ATK-TEST-PREPARE-200",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Second Task for Precedence Check",
		objects.FieldKeyDescription:        "TASK_ENVELOPE\nSecond task description",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, taskObj2))

	outPrecedence, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"ATK-TEST-PREPARE-100",
		"--task", "ATK-TEST-PREPARE-200",
		"--format", "json",
	)
	require.NoError(t, err)
	assert.Contains(t, outPrecedence, "ATK-TEST-PREPARE-100")
	assert.NotContains(t, outPrecedence, "ATK-TEST-PREPARE-200")

	// 6. Matching --task and --task-id flags work identically
	outBothMatching, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--task", "ATK-TEST-PREPARE-100",
		"--task-id", "ATK-TEST-PREPARE-100",
		"--format", "json",
	)
	require.NoError(t, err)
	assert.Equal(t, outPos, outBothMatching)

	// 7. Format agent-prompt works identically with flags
	outPromptPos, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"ATK-TEST-PREPARE-100",
		"--format", "agent-prompt",
	)
	require.NoError(t, err)

	outPromptFlag, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--task-id", "ATK-TEST-PREPARE-100",
		"--format", "agent-prompt",
	)
	require.NoError(t, err)
	assert.Equal(t, outPromptPos, outPromptFlag)

	// 8. Negative Adversarial Boundary: Conflicting flags --task and --task-id
	_, errConflict := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--task", "ATK-TEST-PREPARE-100",
		"--task-id", "ATK-TEST-PREPARE-200",
		"--format", "json",
	)
	require.Error(t, errConflict)
	assert.Contains(t, errConflict.Error(), "conflicting flags")
	assert.Contains(t, errConflict.Error(), "--task")
	assert.Contains(t, errConflict.Error(), "--task-id")

	// 9. Negative Adversarial Boundary: Malformed task identifiers with whitespace
	_, errWhitespacePos := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"ATK TEST BAD",
		"--format", "json",
	)
	require.Error(t, errWhitespacePos)
	assert.Contains(t, errWhitespacePos.Error(), "malformed task identifier")
	assert.Contains(t, errWhitespacePos.Error(), "whitespace")

	_, errWhitespaceTask := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--task", "ATK BAD TASK",
		"--format", "json",
	)
	require.Error(t, errWhitespaceTask)
	assert.Contains(t, errWhitespaceTask.Error(), "malformed task identifier")
	assert.Contains(t, errWhitespaceTask.Error(), "whitespace")

	_, errWhitespaceTaskID := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--task-id", "ATK BAD TASKID",
		"--format", "json",
	)
	require.Error(t, errWhitespaceTaskID)
	assert.Contains(t, errWhitespaceTaskID.Error(), "malformed task identifier")
	assert.Contains(t, errWhitespaceTaskID.Error(), "whitespace")

	// 10. Negative Adversarial Boundary: Malformed task identifiers with invalid characters
	_, errInvalidCharPos := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"../ATK-TRAVERSAL",
		"--format", "json",
	)
	require.Error(t, errInvalidCharPos)
	assert.Contains(t, errInvalidCharPos.Error(), "malformed task identifier")
	assert.Contains(t, errInvalidCharPos.Error(), "invalid characters")

	_, errInvalidCharFlag := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--task", "ATK/INVALID",
		"--format", "json",
	)
	require.Error(t, errInvalidCharFlag)
	assert.Contains(t, errInvalidCharFlag.Error(), "malformed task identifier")
	assert.Contains(t, errInvalidCharFlag.Error(), "invalid characters")
}

func TestPreparedContext_HasSemanticPayload(t *testing.T) {
	// Empty context
	pEmpty := &PreparedContext{}
	assert.False(t, pEmpty.HasSemanticContext())

	// Context with problem_statement
	pProblem := &PreparedContext{
		Task: map[string]any{"problem_statement": "A serious issue"},
	}
	assert.True(t, pProblem.HasSemanticContext())

	// Context with acceptance_criteria
	pCrit := &PreparedContext{
		Task: map[string]any{objects.FieldKeyAcceptanceCriteria: []any{"Criteria 1"}},
	}
	assert.True(t, pCrit.HasSemanticContext())

	// Context with semantic dep
	pDep := &PreparedContext{
		Deps: []map[string]any{
			{objects.FieldKeyDescription: "Dependency description"},
		},
	}
	assert.True(t, pDep.HasSemanticContext())
}

func TestAssemblePreparedContext_WithUpstreamArtifacts(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Upstream task 1: string slice of artifacts
	up1 := map[string]any{
		objects.FieldKeyID:            "ATK-UP-001",
		objects.FieldKeyKind:          objects.KindAgentTask,
		objects.FieldKeyTitle:         "Upstream Task 1",
		objects.FieldKeyDescription:   "Description for up 1",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyArtifacts:     []any{"path/to/art1.go", "path/to/art2.go"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, up1))

	// Upstream task 2: artifact slice
	up2 := map[string]any{
		objects.FieldKeyID:            "ATK-UP-002",
		objects.FieldKeyKind:          objects.KindAgentTask,
		objects.FieldKeyTitle:         "Upstream Task 2",
		objects.FieldKeyDescription:   "Description for up 2",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyArtifacts:     []any{"path/to/single_art.go"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, up2))

	// Downstream task referencing both
	down := map[string]any{
		objects.FieldKeyID:                 "ATK-DOWN-001",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Downstream Integration Task",
		objects.FieldKeyDescription:        "TASK_ENVELOPE\nDownstream description that is longer to pass check",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		"context_refs":                     []any{"ATK-UP-001", "ATK-UP-002"},
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, down))

	prep, err := AssemblePreparedContext(ctx, secCtx, provider, PreparedContextInput{
		ProjectRoot: tempDir,
		TaskID:      "ATK-DOWN-001",
		IncludeTDD:  true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, prep.Prompt)
	assert.Contains(t, prep.Prompt, "Upstream Verified Deliverables")
	assert.Contains(t, prep.Prompt, "path/to/art1.go")
	assert.Contains(t, prep.Prompt, "path/to/single_art.go")
}
