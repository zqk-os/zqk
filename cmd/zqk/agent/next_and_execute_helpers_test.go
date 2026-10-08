package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNext_RequirePriorityPlanRef(t *testing.T) {
	// Missing ref
	_, err := requirePriorityPlanRef(map[string]any{}, "ATK-1")
	assert.ErrorContains(t, err, "priority_plan_ref is missing")

	// Empty string ref
	_, err = requirePriorityPlanRef(map[string]any{objects.FieldKeyPriorityPlanRef: ""}, "ATK-1")
	assert.ErrorContains(t, err, "priority_plan_ref is missing")

	// Valid ref
	ref, err := requirePriorityPlanRef(map[string]any{objects.FieldKeyPriorityPlanRef: "PRI-100"}, "ATK-1")
	require.NoError(t, err)
	assert.Equal(t, "PRI-100", ref)
}

func TestNext_RequireAtkWorktreeTornDown(t *testing.T) {
	tmpDir := t.TempDir()

	// Clean directory -> worktree torn down -> success
	err := requireAtkWorktreeTornDown(tmpDir, "ATK-CLEAN-001")
	require.NoError(t, err)

	// Existing worktree directory -> error
	dirs := paths.AgentWorktreeLookupDirs(tmpDir, "ATK-DIRTY-001")
	if len(dirs) > 0 {
		require.NoError(t, fileutil.EnsureDir(dirs[0]))
		err = requireAtkWorktreeTornDown(tmpDir, "ATK-DIRTY-001")
		assert.ErrorContains(t, err, "worktree")
	}
}

func TestNext_RequireCommitsMergedToIntegration(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// No commit_hashes -> error
	taskNoCommits := map[string]any{
		objects.FieldKeyCommitHashes: []any{},
	}
	err := requireCommitsMergedToIntegration(ctx, tmpDir, "ATK-1", "PRI-1", taskNoCommits)
	assert.ErrorContains(t, err, "no commit_hashes found")
}

func TestNext_VerifyDocsEvalNextEvidence(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Description has no findings path
	err := verifyDocsEvalNextEvidence(tmpDir, "Regular description with no path")
	assert.ErrorContains(t, err, "no .../findings/*.jsonl path")

	// 2. Description has findings path but file is missing
	descWithMissingFile := "Review output at .zqk/runs/findings/eval.jsonl"
	err = verifyDocsEvalNextEvidence(tmpDir, descWithMissingFile)
	assert.ErrorContains(t, err, "docs_eval evidence missing")

	// 3. File exists with empty lines / REPLACE_ME -> fails
	findingsRel := ".zqk/runs/findings/eval.jsonl"
	findingsAbs := filepath.Join(tmpDir, filepath.FromSlash(findingsRel))
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(findingsAbs)))
	require.NoError(t, fileutil.WriteStandardFile(findingsAbs, []byte("REPLACE_ME\n")))

	err = verifyDocsEvalNextEvidence(tmpDir, descWithMissingFile)
	assert.ErrorContains(t, err, "SUCCESS_GATE failed")

	// 4. File exists with valid finding_id -> passes!
	validPayload := `{"finding_id": "FIND-001", "summary": "Found issue"}` + "\n"
	require.NoError(t, fileutil.WriteStandardFile(findingsAbs, []byte(validPayload)))

	err = verifyDocsEvalNextEvidence(tmpDir, descWithMissingFile)
	require.NoError(t, err)
}

func TestExecute_FormatTaskStepsSection(t *testing.T) {
	steps := []any{
		map[string]any{
			objects.FieldKeyTitle:       "Step 1",
			objects.FieldKeyDescription: "Do initial setup",
		},
	}
	section := formatTaskStepsSection(steps)
	assert.NotEmpty(t, section)
	assert.Contains(t, section, "Step 1")

	// Empty steps
	assert.Empty(t, formatTaskStepsSection(nil))
}

func TestExecute_CommandValidation(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing prompt and task-id
	out, err := executeAgentCommand(t, tempDir, provider, "execute")
	assert.ErrorContains(t, err, "requires --prompt, --task-id, or a task ID argument")
	_ = out

	// 2. Nonexistent task
	_, err = executeAgentCommand(t, tempDir, provider, "execute", "ATK-NONEXISTENT")
	assert.ErrorContains(t, err, "failed to read task")

	// 3. Task with empty description
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	taskEmpty := map[string]any{
		objects.FieldKeyID:                 "ATK-EMPTY-PROMPT",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Empty task",
		objects.FieldKeyDescription:        "",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, taskEmpty, objects.ObjectStatusApproved)

	_, err = executeAgentCommand(t, tempDir, provider, "execute", "ATK-EMPTY-PROMPT")
	assert.Error(t, err)
}

func TestExecute_NonAgentTaskTarget(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	_, err := executeAgentCommand(t, tempDir, provider, "execute", objects.ConstPersonaDefaultOperator)
	assert.Error(t, err)
}

func TestExecute_WithPromptFlag(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	_, err := executeAgentCommand(t, tempDir, provider, "execute", "--prompt", "Direct Prompt Test")
	assert.Error(t, err)
}
