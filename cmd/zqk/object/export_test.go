package object

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestInProcess_Export_Comprehensive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	obj := map[string]any{
		objects.FieldKeyID:                       "BLI-EXP-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Export Comprehensive Item",
		objects.FieldKeyDescription:              "Valid description for export testing.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          "PRI-EXP-001",
		objects.FieldKeyMilestoneRefs:            []any{"MIL-EXP-001"},
		objects.FieldKeyStatus:                   "planned",
	}
	err = provider.Create(ctx, secCtx, obj)
	require.NoError(t, err)

	// 1. Export with --output flag alias
	outPath := filepath.Join(tempDir, "exported_alias.yaml")
	out, err := executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "--output", outPath)
	assert.NoError(t, err, "export --output output: %s", out)
	data, err := fileutil.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "BLI-EXP-001")

	// 2. Export with table format defaulting to YAML
	out, err = executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "--format", "table", "--out", "-")
	assert.NoError(t, err, "export table to yaml output: %s", out)
	assert.Contains(t, out, "BLI-EXP-001")

	// 3. Export to an unwritable / non-existent directory
	invalidPath := filepath.Join(tempDir, "non_existent_subdir", "deep", "exported.yaml")
	_, err = executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "--out", invalidPath)
	assert.Error(t, err, "expected error exporting to unwritable path")
}
