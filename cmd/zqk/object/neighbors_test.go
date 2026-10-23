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

func seedGraphTestObjects(t *testing.T, provider storage.ObjectStorageProvider) {
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	objA := map[string]any{
		objects.FieldKeyID:                       "BLI-GRAPH-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Source Graph Node",
		objects.FieldKeyDescription:              "Valid description for graph node A.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          "PRI-GRAPH-001",
		objects.FieldKeyMilestoneRefs:            []any{"MIL-GRAPH-001"},
		objects.FieldKeyStatus:                   "planned",
	}
	err := provider.Create(ctx, secCtx, objA)
	require.NoError(t, err)

	objB := map[string]any{
		objects.FieldKeyID:                       "BLI-GRAPH-002",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Target Graph Node",
		objects.FieldKeyDescription:              "Valid description for graph node B.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "low",
		objects.FieldKeyPriorityTier:             "P3",
		objects.FieldKeyPriorityPlanRef:          "PRI-GRAPH-001",
		objects.FieldKeyStatus:                   "planned",
	}
	err = provider.Create(ctx, secCtx, objB)
	require.NoError(t, err)
}

func TestInProcess_Neighbors_Positive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	seedGraphTestObjects(t, provider)

	out, err := executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-GRAPH-001")
	assert.NoError(t, err, "neighbors default output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-GRAPH-001", "--direction", "outgoing")
	assert.NoError(t, err, "neighbors outgoing output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-GRAPH-001", "--direction", "incoming")
	assert.NoError(t, err, "neighbors incoming output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-GRAPH-001", "--direction", "both")
	assert.NoError(t, err, "neighbors both output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-GRAPH-001", "--format", "json")
	assert.NoError(t, err, "neighbors json output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-GRAPH-001", "--format", "yaml")
	assert.NoError(t, err, "neighbors yaml output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-GRAPH-001", "--format", "table")
	assert.NoError(t, err, "neighbors table output: %s", out)
}

func TestInProcess_Neighbors_Negative(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	seedGraphTestObjects(t, provider)

	// Missing ID argument
	_, err = executeObjectCommand(t, tempDir, provider, "neighbors")
	assert.Error(t, err)

	// Invalid direction
	_, err = executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-GRAPH-001", "--direction", "sideways")
	assert.Error(t, err)

	// Non-existent ID
	_, err = executeObjectCommand(t, tempDir, provider, "neighbors", "BLI-NONEXISTENT-999")
	assert.Error(t, err)
}
