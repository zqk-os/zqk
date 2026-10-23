package object

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestInProcess_UpdateAll_Positive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	obj1 := map[string]any{
		objects.FieldKeyID:                       "BLI-UPD-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Update All Item 1",
		objects.FieldKeyDescription:              "Valid description for update all test item 1.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "low",
		objects.FieldKeyPriorityTier:             "P3",
		objects.FieldKeyPriorityPlanRef:          "PRI-UPD-001",
		objects.FieldKeyStatus:                   "planned",
	}
	err = provider.Create(ctx, secCtx, obj1)
	require.NoError(t, err)

	// Dry run update all
	out, err := executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "priority=high", "--dry-run")
	assert.NoError(t, err, "update --all dry-run output: %s", out)

	// Actual update all with filter and add-missing-fields
	out, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "priority=high", "--filter", "status=planned", "--add-missing-fields")
	assert.NoError(t, err, "update --all output: %s", out)

	// Guard against manual status update on --all
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "status=completed")
	assert.Error(t, err)

	// Guard against manual ref field update on --all
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "goal_refs=GOAL-001")
	assert.Error(t, err)

	// Guard against manual provenance update on --all
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "created_at=2026-01-01T00:00:00Z")
	assert.Error(t, err)

	// Object update error on invalid field
	out, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "non_existent_field=bad")
	assert.NoError(t, err)
	assert.Contains(t, out, "Errors: 1")

	// Append operation on --all returns error during flag parse
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "tags+=critical")
	assert.Error(t, err)

	// Verify priority was updated
	readObj, err := provider.Read(ctx, secCtx, "BLI-UPD-001")
	require.NoError(t, err)
	assert.Equal(t, "high", readObj[objects.FieldKeyPriority])
}

func TestInProcess_UpdateAll_NoObjectsAndNegatives(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// No objects of kind found
	out, err := executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "priority=high")
	assert.NoError(t, err)
	assert.Contains(t, out, "No objects of kind backlog_item found")

	// Missing --kind flag
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--field", "priority=high")
	assert.Error(t, err)

	// Invalid kind
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "non_existent_kind_xyz", "--field", "priority=high")
	assert.Error(t, err)

	// Invalid filter format
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "priority=high", "--filter", "invalid_filter_without_equals")
	assert.Error(t, err)
}

func TestInProcess_UpdateAll_KindResolutionDirect(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	cmd := NewUpdateCmd()
	var buf bytes.Buffer
	testCtx := setTestCLIContext(t, cmd, tempDir, provider)
	testCtx = pkgctx.WithCommandOutputWriter(testCtx, &buf)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(testCtx)

	require.NoError(t, cmd.Flags().Set("all", "true"))
	require.NoError(t, cmd.Flags().Set("kind", "invalid_kind_for_direct_test"))
	err = runUpdateAll(cmd, []string{})
	assert.Error(t, err)

	cmdValid := NewUpdateCmd()
	var bufValid bytes.Buffer
	testCtxValid := setTestCLIContext(t, cmdValid, tempDir, provider)
	testCtxValid = pkgctx.WithCommandOutputWriter(testCtxValid, &bufValid)
	cmdValid.SetOut(&bufValid)
	cmdValid.SetErr(&bufValid)
	cmdValid.SetContext(testCtxValid)
	require.NoError(t, cmdValid.Flags().Set("all", "true"))
	require.NoError(t, cmdValid.Flags().Set("kind", "backlog_item"))
	require.NoError(t, cmdValid.Flags().Set("field", "priority=high"))
	err = runUpdateAll(cmdValid, []string{})
	assert.NoError(t, err)
}
