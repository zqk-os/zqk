package object

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const testDeleteReasonCode = "--reason-code=legitimate-test-clean-up-of-isolated-temporary-object"

func seedDeleteTargetObject(t *testing.T, provider storage.ObjectStorageProvider, id string) {
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	obj := map[string]any{
		objects.FieldKeyID:                       id,
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Delete Target Item",
		objects.FieldKeyDescription:              "Valid description for delete test target.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          "PRI-DEL-001",
		objects.FieldKeyStatus:                   "planned",
	}
	err := provider.Create(ctx, secCtx, obj)
	require.NoError(t, err)
}

func TestInProcess_Delete_SinglePositive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Dry run delete (requires --cascade or --unlink-references)
	seedDeleteTargetObject(t, provider, "BLI-DEL-DRY")
	out, err := executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-DRY", "--dry-run", "--cascade")
	assert.NoError(t, err, "delete dry-run output: %s", out)

	// Dry run delete with json format
	out, err = executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-DRY", "--dry-run", "--cascade", "--format", "json")
	assert.NoError(t, err, "delete dry-run json output: %s", out)

	exists, err := provider.Exists(ctx, secCtx, "BLI-DEL-DRY")
	require.NoError(t, err)
	assert.True(t, exists, "dry-run must not delete the object")

	// Actual delete with --cascade and reason code
	seedDeleteTargetObject(t, provider, "BLI-DEL-001")
	out, err = executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-001", "--cascade", testDeleteReasonCode)
	assert.NoError(t, err, "delete output: %s", out)

	exists, err = provider.Exists(ctx, secCtx, "BLI-DEL-001")
	require.NoError(t, err)
	assert.False(t, exists, "object should be deleted")
}

func TestInProcess_Delete_CascadeAndUnlink(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Delete with --cascade and reason code
	seedDeleteTargetObject(t, provider, "BLI-DEL-CAS")
	out, err := executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-CAS", "--cascade", testDeleteReasonCode)
	assert.NoError(t, err, "delete cascade output: %s", out)

	exists, err := provider.Exists(ctx, secCtx, "BLI-DEL-CAS")
	require.NoError(t, err)
	assert.False(t, exists)

	// Delete with --unlink-references and reason code
	seedDeleteTargetObject(t, provider, "BLI-DEL-UNLINK")
	out, err = executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-UNLINK", "--unlink-references", testDeleteReasonCode)
	assert.NoError(t, err, "delete unlink output: %s", out)

	exists, err = provider.Exists(ctx, secCtx, "BLI-DEL-UNLINK")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestInProcess_Delete_BulkAndNegative(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Multiple IDs delete with --cascade and reason code
	seedDeleteTargetObject(t, provider, "BLI-DEL-M1")
	seedDeleteTargetObject(t, provider, "BLI-DEL-M2")
	out, err := executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-M1", "BLI-DEL-M2", "--cascade", testDeleteReasonCode)
	assert.NoError(t, err, "bulk delete output: %s", out)

	exists1, _ := provider.Exists(ctx, secCtx, "BLI-DEL-M1")
	exists2, _ := provider.Exists(ctx, secCtx, "BLI-DEL-M2")
	assert.False(t, exists1)
	assert.False(t, exists2)

	// Negative: No ID provided
	_, err = executeObjectCommand(t, tempDir, provider, "delete")
	assert.Error(t, err)

	// Negative: Refused without --cascade or --unlink-references
	seedDeleteTargetObject(t, provider, "BLI-DEL-NOFLAG")
	_, err = executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-NOFLAG")
	assert.Error(t, err)

	// Negative: Conflicting flags --unlink-references and --cascade
	seedDeleteTargetObject(t, provider, "BLI-DEL-CONF")
	_, err = executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-CONF", "--unlink-references", "--cascade")
	assert.Error(t, err)
}

func TestInProcess_Delete_FileFlag(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	seedDeleteTargetObject(t, provider, "BLI-DEL-FILE")

	idsFile := filepath.Join(tempDir, "ids-to-delete.yaml")
	err = fileutil.WriteFile(idsFile, []byte("- BLI-DEL-FILE\n"), paths.FilePerm644)
	require.NoError(t, err)

	out, err := executeObjectCommand(t, tempDir, provider, "delete", "--file", idsFile, "--cascade", testDeleteReasonCode)
	assert.NoError(t, err, "delete --file output: %s", out)

	exists, err := provider.Exists(ctx, secCtx, "BLI-DEL-FILE")
	require.NoError(t, err)
	assert.False(t, exists)
}
