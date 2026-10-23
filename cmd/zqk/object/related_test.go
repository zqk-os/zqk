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

func TestInProcess_Related_Positive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	longTitle := "A Very Long Title That Definitely Exceeds Fifty Seven Characters For Truncation Verification Purposes"
	goal := map[string]any{
		objects.FieldKeyID:            "GOAL-REL-001",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         longTitle,
		objects.FieldKeyDescription:   "Valid description for related goal.",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTarget:        "100",
		objects.FieldKeyMetric:        "count",
	}
	err = provider.Create(ctx, secCtx, goal)
	require.NoError(t, err)

	objA := map[string]any{
		objects.FieldKeyID:                       "BLI-REL-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Related Node A",
		objects.FieldKeyDescription:              "Valid description for related node A.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "high",
		objects.FieldKeyPriorityTier:             "P1",
		objects.FieldKeyPriorityPlanRef:          "PRI-REL-001",
		objects.FieldKeyGoalRefs:                 []any{"GOAL-REL-001"},
		objects.FieldKeyStatus:                   "planned",
	}
	err = provider.Create(ctx, secCtx, objA)
	require.NoError(t, err)

	out, err := executeObjectCommand(t, tempDir, provider, "related", "BLI-REL-001")
	assert.NoError(t, err, "related default output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "related", "BLI-REL-001", "--depth", "2", "--relationship", "parent")
	assert.NoError(t, err, "related depth/rel output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "related", "BLI-REL-001", "--format", "json")
	assert.NoError(t, err, "related json output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "related", "BLI-REL-001", "--format", "yaml")
	assert.NoError(t, err, "related yaml output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "related", "BLI-REL-001", "--format", "table")
	assert.NoError(t, err, "related table output: %s", out)
}

func TestInProcess_Related_NegativeAndEdgeCases(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// Missing ID argument
	_, err = executeObjectCommand(t, tempDir, provider, "related")
	assert.Error(t, err)

	// Non-existent ID
	_, err = executeObjectCommand(t, tempDir, provider, "related", "BLI-NONEXISTENT-888")
	assert.Error(t, err)
}
