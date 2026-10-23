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

func TestInProcess_Rename_Positive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	obj := map[string]any{
		objects.FieldKeyID:                       "BLI-REN-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Original Title",
		objects.FieldKeyDescription:              "Valid description for rename test.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          "PRI-REN-001",
		objects.FieldKeyStatus:                   "planned",
	}
	err = provider.Create(ctx, secCtx, obj)
	require.NoError(t, err)

	// Dry run rename
	out, err := executeObjectCommand(t, tempDir, provider, "rename", "BLI-REN-001", "BLI-REN-DRY", "--dry-run")
	assert.NoError(t, err, "rename dry-run output: %s", out)
	assert.Contains(t, out, "Would rename object BLI-REN-001 to BLI-REN-DRY")

	// Verify old object still exists after dry-run
	exists, err := provider.Exists(ctx, secCtx, "BLI-REN-001")
	require.NoError(t, err)
	assert.True(t, exists)

	// Actual rename
	out, err = executeObjectCommand(t, tempDir, provider, "rename", "BLI-REN-001", "BLI-REN-002", "--update-references")
	assert.NoError(t, err, "rename output: %s", out)
	assert.Contains(t, out, "Object BLI-REN-001 renamed to BLI-REN-002")

	// Verify old ID is gone and new ID exists
	existsOld, err := provider.Exists(ctx, secCtx, "BLI-REN-001")
	require.NoError(t, err)
	assert.False(t, existsOld)

	existsNew, err := provider.Exists(ctx, secCtx, "BLI-REN-002")
	require.NoError(t, err)
	assert.True(t, existsNew)
}

func TestInProcess_Rename_Negative(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// Missing args (< 2)
	_, err = executeObjectCommand(t, tempDir, provider, "rename")
	assert.Error(t, err)

	_, err = executeObjectCommand(t, tempDir, provider, "rename", "BLI-REN-001")
	assert.Error(t, err)

	// Non-existent ID for dry run
	_, err = executeObjectCommand(t, tempDir, provider, "rename", "BLI-NONEXISTENT", "BLI-NEW", "--dry-run")
	assert.Error(t, err)

	// Non-existent ID for actual rename
	_, err = executeObjectCommand(t, tempDir, provider, "rename", "BLI-NONEXISTENT", "BLI-NEW")
	assert.Error(t, err)
}
