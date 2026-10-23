package object

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestInProcess_Path_Positive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	goalA := map[string]any{
		objects.FieldKeyID:            "GOAL-PATH-001",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Target Path Goal Node With Very Long Descriptive Title Exceeding Column Width For Truncation",
		objects.FieldKeyDescription:   "Valid description for path goal.",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTarget:        "100",
		objects.FieldKeyMetric:        "count",
	}
	err = provider.Create(ctx, secCtx, goalA)
	require.NoError(t, err)

	objA := map[string]any{
		objects.FieldKeyID:                       "BLI-PATH-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Source Path Node A",
		objects.FieldKeyDescription:              "Valid description for path node A.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          "PRI-PATH-001",
		objects.FieldKeyGoalRefs:                 []any{"GOAL-PATH-001"},
		objects.FieldKeyStatus:                   "planned",
	}
	err = provider.Create(ctx, secCtx, objA)
	require.NoError(t, err)

	out, err := executeObjectCommand(t, tempDir, provider, "path", "BLI-PATH-001", "GOAL-PATH-001")
	assert.NoError(t, err, "path default output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "path", "BLI-PATH-001", "GOAL-PATH-001", "--format", "json")
	assert.NoError(t, err, "path json output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "path", "BLI-PATH-001", "GOAL-PATH-001", "--format", "yaml")
	assert.NoError(t, err, "path yaml output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "path", "BLI-PATH-001", "GOAL-PATH-001", "--format", "table")
	assert.NoError(t, err, "path table output: %s", out)
	assert.Contains(t, out, "Path (length:")
}

func TestInProcess_Path_NegativeAndEmpty(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// Missing arguments (< 2 args)
	_, err = executeObjectCommand(t, tempDir, provider, "path")
	assert.Error(t, err)

	_, err = executeObjectCommand(t, tempDir, provider, "path", "BLI-PATH-001")
	assert.Error(t, err)

	// Disconnected / non-existent IDs return message with no error
	out, err := executeObjectCommand(t, tempDir, provider, "path", "BLI-NONEXIST-1", "BLI-NONEXIST-2")
	assert.NoError(t, err)
	assert.Contains(t, out, "No path found between")
}
