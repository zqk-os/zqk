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
