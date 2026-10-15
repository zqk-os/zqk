package object

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestInProcess_SubcommandCoverageElevation(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// 1. Seed a backlog_item in 'planned' status with required fields for demote testing
	bliObj := map[string]any{
		objects.FieldKeyID:                        "BLI-COV-001",
		objects.FieldKeyKind:                      objects.KindBacklogItem,
		objects.FieldKeyStatus:                    "planned",
		objects.FieldKeyProblemStatement:          "Valid problem statement of sufficient length for elevation testing.",
		objects.FieldKeyDescription:               "Valid problem statement/description of sufficient length for elevation testing.",
		objects.FieldKeyAcceptanceConsiderations: "Acceptance considerations for demote validation.",
		objects.FieldKeyPriority:                  "high",
		objects.FieldKeyPriorityTier:              "P1",
		objects.FieldKeyPriorityPlanRef:           "PRI-COV-PLAN",
		objects.FieldKeyMilestoneRefs:             []any{"MIL-COV-001"},
		objects.FieldKeyTitle:                     "Coverage Elevation Backlog Item",
	}
	err = provider.Create(ctx, secCtx, bliObj)
	require.NoError(t, err)

	// 2. Seed an evolution_management object
	evoObj := map[string]any{
		objects.FieldKeyID:                  "EVO-COV-001",
		objects.FieldKeyKind:                objects.KindEvolutionManagement,
		objects.FieldKeyTitle:               "Evolution Cycle Test",
		objects.FieldKeyStrategy:            "adaptive",
		objects.FieldKeyPeriod:              time.Now().Format("2006-01-02"),
		objects.FieldKeyDescription:         "Evolution management object for coverage testing.",
		objects.FieldKeyAdaptiveAdjustments: []string{"Adjustment 1"},
		objects.FieldKeyStatus:              objects.ObjectStatusComplete,
	}
	err = provider.Create(ctx, secCtx, evoObj)
	require.NoError(t, err)

	// 3. Seed a strategic_plan object
	splanObj := map[string]any{
		objects.FieldKeyID:          "STRAT-PLAN-COV-001",
		objects.FieldKeyKind:        objects.KindStrategicPlan,
		objects.FieldKeyTitle:       "Strategic Plan Coverage Test",
		objects.FieldKeyDescription: "Strategic plan object for elevation test suite.",
		objects.FieldKeyStatus:      "active",
		objects.FieldKeyVision:       "Strategic vision statement with sufficient length.",
	}
	err = provider.Create(ctx, secCtx, splanObj)
	require.NoError(t, err)

	// 4. Seed a workstream_transition object
	wstransObj := map[string]any{
		objects.FieldKeyID:          "WST-COV-001",
		objects.FieldKeyKind:        objects.KindWorkstreamTransition,
		objects.FieldKeyTitle:       "Workstream Transition Coverage Test",
		objects.FieldKeyDescription: "Workstream transition test object.",
		objects.FieldKeyStatus:      "active",
		objects.FieldKeyTrigger:     "milestone_completion",
	}
	err = provider.Create(ctx, secCtx, wstransObj)
	require.NoError(t, err)

	// 5. Test Evoman list & show
	out, err := executeObjectCommand(t, tempDir, provider, "evoman", "list")
	assert.NoError(t, err, "evoman list output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "evoman", "show")
	assert.NoError(t, err, "evoman show output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "evoman", "show", "EVO-COV-001")
	assert.NoError(t, err, "evoman show EVO-COV-001 output: %s", out)

	// 6. Test Splan list & show
	out, err = executeObjectCommand(t, tempDir, provider, "splan", "list")
	assert.NoError(t, err, "splan list output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "splan", "show")
	assert.NoError(t, err, "splan show output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "splan", "show", "STRAT-PLAN-COV-001")
	assert.NoError(t, err, "splan show STRAT-PLAN-COV-001 output: %s", out)

	// 7. Test Wstrans list & show
	out, err = executeObjectCommand(t, tempDir, provider, "wstrans", "list")
	assert.NoError(t, err, "wstrans list output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "wstrans", "show")
	assert.NoError(t, err, "wstrans show output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "wstrans", "show", "WST-COV-001")
	assert.NoError(t, err, "wstrans show WST-COV-001 output: %s", out)

	// 8. Test Export (file output, stdout, filter, json format)
	exportTarget := filepath.Join(tempDir, "exported_items.yaml")
	out, err = executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "-o", exportTarget)
	assert.NoError(t, err, "export -o output: %s", out)
	fileBytes, err := fileutil.ReadFile(exportTarget)
	assert.NoError(t, err)
	assert.Contains(t, string(fileBytes), "BLI-COV-001")

	// Export to stdout
	out, err = executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "-o", "-")
	assert.NoError(t, err)
	assert.Contains(t, out, "BLI-COV-001")

	// Export using --out flag
	exportOutTarget := filepath.Join(tempDir, "exported_items_out.yaml")
	out, err = executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "--out", exportOutTarget)
	assert.NoError(t, err, "export --out output: %s", out)
	fileBytesOut, err := fileutil.ReadFile(exportOutTarget)
	assert.NoError(t, err)
	assert.Contains(t, string(fileBytesOut), "BLI-COV-001")

	// Export with filter and json format
	out, err = executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "--filter", "status=planned", "--format", "json", "-o", "-")
	assert.NoError(t, err)
	assert.Contains(t, out, "BLI-COV-001")

	// Export negative: missing kind
	out, err = executeObjectCommand(t, tempDir, provider, "export")
	assert.Error(t, err)

	// Export negative: invalid kind
	out, err = executeObjectCommand(t, tempDir, provider, "export", "non_existent_kind_xyz")
	assert.Error(t, err)

	// 9. Test Demote positive: planned -> validated
	out, err = executeObjectCommand(t, tempDir, provider, "demote", "BLI-COV-001")
	assert.NoError(t, err, "demote output: %s", out)

	// Verify BLI-COV-001 status changed to 'validated'
	readObj, err := provider.Read(ctx, secCtx, "BLI-COV-001")
	require.NoError(t, err)
	assert.Equal(t, "validated", readObj[objects.FieldKeyStatus])

	// 10. Test Demote negative: already at lowest status ('validated')
	out, err = executeObjectCommand(t, tempDir, provider, "demote", "BLI-COV-001")
	assert.Error(t, err, "expected error demoting object already at lowest status, output: %s", out)

	// 11. Test Demote negative: non-existent ID
	out, err = executeObjectCommand(t, tempDir, provider, "demote", "BLI-DOES-NOT-EXIST")
	assert.Error(t, err)

	// 12. Test Demote negative: missing argument
	out, err = executeObjectCommand(t, tempDir, provider, "demote")
	assert.Error(t, err)
}
