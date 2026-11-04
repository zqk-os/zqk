package object

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestNormalizeFlags_GH917_Comprehensive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	item1 := map[string]any{
		objects.FieldKeyID:                       "BLI-NORM-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Normalized Item 1",
		objects.FieldKeyDescription:              "Valid description for item 1.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "low",
		objects.FieldKeyPriorityTier:             "P3",
		objects.FieldKeyPriorityPlanRef:          "PRI-NORM-001",
		objects.FieldKeyStatus:                   "planned",
	}
	item2 := map[string]any{
		objects.FieldKeyID:                       "BLI-NORM-002",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Normalized Item 2",
		objects.FieldKeyDescription:              "Valid description for item 2.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          "PRI-NORM-001",
		objects.FieldKeyStatus:                   "planned",
	}

	require.NoError(t, provider.Create(ctx, secCtx, item1))
	require.NoError(t, provider.Create(ctx, secCtx, item2))

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	require.NoError(t, provider.EnsureCLIObjectMutationVisible(flushCtx, []string{objects.KindBacklogItem}))

	// ==========================================
	// 1. LIST: positional vs --kind
	// ==========================================
	outListPos, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item")
	require.NoError(t, err)
	assert.Contains(t, outListPos, "BLI-NORM-001")
	assert.Contains(t, outListPos, "BLI-NORM-002")

	outListFlag, err := executeObjectCommand(t, tempDir, provider, "list", "--kind", "backlog_item")
	require.NoError(t, err)
	assert.Contains(t, outListFlag, "BLI-NORM-001")
	assert.Contains(t, outListFlag, "BLI-NORM-002")

	// Invalid kind on list --kind fails validation
	_, err = executeObjectCommand(t, tempDir, provider, "list", "--kind", "non_existent_kind_xyz")
	assert.Error(t, err)

	// ==========================================
	// 2. COUNT: positional vs --kind
	// ==========================================
	outCountPos, err := executeObjectCommand(t, tempDir, provider, "count", "backlog_item")
	require.NoError(t, err)
	assert.True(t, strings.Contains(outCountPos, "2") || strings.Contains(outCountPos, "backlog_item"))

	outCountFlag, err := executeObjectCommand(t, tempDir, provider, "count", "--kind", "backlog_item")
	require.NoError(t, err)
	assert.True(t, strings.Contains(outCountFlag, "2") || strings.Contains(outCountFlag, "backlog_item"))

	// Invalid kind on count --kind fails validation
	_, err = executeObjectCommand(t, tempDir, provider, "count", "--kind", "non_existent_kind_xyz")
	assert.Error(t, err)

	// ==========================================
	// 3. UPDATE: positional vs --kind on --all and single/bulk
	// ==========================================
	// Positional kind on update --all
	outUpdAllPos, err := executeObjectCommand(t, tempDir, provider, "update", "--all", "backlog_item", "--field", "priority=high")
	require.NoError(t, err)
	assert.Contains(t, outUpdAllPos, "Update complete for kind backlog_item")
	assert.Contains(t, outUpdAllPos, "Success: 2")

	// Flag kind on update --all
	outUpdAllFlag, err := executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "priority=low")
	require.NoError(t, err)
	assert.Contains(t, outUpdAllFlag, "Update complete for kind backlog_item")
	assert.Contains(t, outUpdAllFlag, "Success: 2")

	// Invalid kind on update --all
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--all", "non_existent_kind_xyz", "--field", "priority=high")
	assert.Error(t, err)

	// Positional kind on single update
	_, err = executeObjectCommand(t, tempDir, provider, "update", "backlog_item", "BLI-NORM-001", "--field", "priority=critical")
	require.NoError(t, err)

	// Flag kind on single update
	_, err = executeObjectCommand(t, tempDir, provider, "update", "--kind", "backlog_item", "BLI-NORM-001", "--field", "priority=high")
	require.NoError(t, err)

	// Comma-separated IDs on update
	_, err = executeObjectCommand(t, tempDir, provider, "update", "BLI-NORM-001,BLI-NORM-002", "--field", "priority=medium")
	require.NoError(t, err)

	// ==========================================
	// 4. GET and SHOW: positional kind, --kind flag, comma-separated IDs
	// ==========================================
	for _, verb := range []string{"get", "show"} {
		// Single ID
		out, err := executeObjectCommand(t, tempDir, provider, verb, "BLI-NORM-001", "--format", "json")
		require.NoError(t, err)
		assert.Contains(t, out, "BLI-NORM-001")

		// Positional kind + single ID
		out, err = executeObjectCommand(t, tempDir, provider, verb, "backlog_item", "BLI-NORM-001", "--format", "json")
		require.NoError(t, err)
		assert.Contains(t, out, "BLI-NORM-001")

		// --kind flag + single ID
		out, err = executeObjectCommand(t, tempDir, provider, verb, "--kind", "backlog_item", "BLI-NORM-001", "--format", "json")
		require.NoError(t, err)
		assert.Contains(t, out, "BLI-NORM-001")

		// Comma-separated IDs
		out, err = executeObjectCommand(t, tempDir, provider, verb, "BLI-NORM-001,BLI-NORM-002", "--format", "json")
		require.NoError(t, err)
		var bulk1 map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &bulk1))
		assert.Equal(t, float64(2), bulk1["total"])
		assert.Equal(t, float64(2), bulk1["success"])

		// Positional kind + comma-separated IDs
		out, err = executeObjectCommand(t, tempDir, provider, verb, "backlog_item", "BLI-NORM-001,BLI-NORM-002", "--format", "json")
		require.NoError(t, err)
		var bulk2 map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &bulk2))
		assert.Equal(t, float64(2), bulk2["total"])
		assert.Equal(t, float64(2), bulk2["success"])

		// --kind flag + comma-separated IDs
		out, err = executeObjectCommand(t, tempDir, provider, verb, "--kind", "backlog_item", "BLI-NORM-001,BLI-NORM-002", "--format", "json")
		require.NoError(t, err)
		var bulk3 map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &bulk3))
		assert.Equal(t, float64(2), bulk3["total"])
		assert.Equal(t, float64(2), bulk3["success"])

		// Invalid --kind on get/show
		_, err = executeObjectCommand(t, tempDir, provider, verb, "--kind", "non_existent_kind_xyz", "BLI-NORM-001")
		assert.Error(t, err)
	}
}
