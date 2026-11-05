package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/featureflags"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRepairYAMLPipeline_Scenarios(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := logging.NewEventLogger(context.Background())
	cmd := NewRepairYAMLCmd()

	// 1. Missing cmd
	err := RunRepairYAMLViaPipeline(nil, tmpDir, "foo.yaml", "", "", false, logger)
	assert.Error(t, err)

	// 2. Missing projectRoot
	err = RunRepairYAMLViaPipeline(cmd, "", "foo.yaml", "", "", false, logger)
	assert.Error(t, err)

	// 3. Missing file and id
	err = RunRepairYAMLViaPipeline(cmd, tmpDir, "", "", "", false, logger)
	assert.Error(t, err)

	// 4. File does not exist
	err = RunRepairYAMLViaPipeline(cmd, tmpDir, "non_existent.yaml", "", "", false, logger)
	assert.Error(t, err)

	// 5. Valid YAML file (normalization detects no repair needed)
	validFile := filepath.Join(tmpDir, "valid.yaml")
	require.NoError(t, fileutil.WriteFile(validFile, []byte("id: GOAL-001\nkind: goal\ntitle: My Goal\n"), paths.FilePerm644))

	err = RunRepairYAMLViaPipeline(cmd, tmpDir, validFile, "", "goal", true, logger)
	assert.NoError(t, err)
}

func TestPathCachePipeline_DecideActionAndWritePaths(t *testing.T) {
	t.Parallel()

	// Test decidePathCacheAction matrix
	assert.Equal(t, pathCacheActionBuild, decidePathCacheAction(false, false, false, false))
	assert.Equal(t, pathCacheActionRefreshWait, decidePathCacheAction(true, true, true, false))
	assert.Equal(t, pathCacheActionRefreshAsync, decidePathCacheAction(true, true, false, false))
	assert.Equal(t, pathCacheActionNone, decidePathCacheAction(true, false, false, true))
	assert.Equal(t, pathCacheActionNone, decidePathCacheAction(true, false, false, false))

	// Test writePathCacheResolvedPaths
	tmpDir := t.TempDir()
	cmd := NewPathCacheCmd()
	err := writePathCacheResolvedPaths(cmd, tmpDir)
	assert.NoError(t, err)
}

func TestRepairCASCorruptionPipeline_Validation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := logging.NewEventLogger(context.Background())
	cmd := NewRepairCASCorruptionCmd()

	// Missing cmd
	err := RunRepairCASCorruptionViaPipeline(nil, tmpDir, "", true, "", false, false, logger)
	assert.Error(t, err)

	// Missing projectRoot
	err = RunRepairCASCorruptionViaPipeline(cmd, "", "", true, "", false, false, logger)
	assert.Error(t, err)

	// Dry run in clean project with empty input file
	inputFile := filepath.Join(tmpDir, ".zqk", "system-health", "tier1-cas-corruption.tsv")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(inputFile), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(inputFile, []byte(""), paths.FilePerm644))

	err = RunRepairCASCorruptionViaPipeline(cmd, tmpDir, inputFile, true, "", false, false, logger)
	assert.NoError(t, err)
}

func TestImprovementReport_AdaptiveAndFormat(t *testing.T) {
	t.Parallel()

	// 1. defaultAdaptivePlan
	plan := defaultAdaptivePlan()
	assert.Equal(t, "standard", plan.Mode)
	assert.Equal(t, "1h", plan.RecommendedNextCadence)

	// 2. deriveAdaptivePlan with nil
	derivedNil := deriveAdaptivePlan(nil)
	assert.Equal(t, "standard", derivedNil.Mode)
	assert.Contains(t, derivedNil.Reasons[0], "no previous report")

	// 3. deriveAdaptivePlan with previous high load
	prevHighLoad := &ImprovementReportOutput{
		Snapshot: ImprovementReportSnapshot{
			HealthData: map[string]any{
				"scheduler_pool_declined": float64(5),
			},
		},
	}
	derivedHighLoad := deriveAdaptivePlan(prevHighLoad)
	assert.Equal(t, "lite", derivedHighLoad.Mode)
	assert.Equal(t, "2h", derivedHighLoad.RecommendedNextCadence)

	// 4. deriveAdaptivePlan with unstable signals
	prevUnstable := &ImprovementReportOutput{
		Snapshot: ImprovementReportSnapshot{
			HealthData: map[string]any{
				"check_blocking": 3,
			},
		},
	}
	derivedUnstable := deriveAdaptivePlan(prevUnstable)
	assert.Equal(t, "focused", derivedUnstable.Mode)
	assert.Equal(t, "30m", derivedUnstable.RecommendedNextCadence)

	// 5. cadenceToCron
	assert.Equal(t, "*/30 * * * *", cadenceToCron("30m"))
	assert.Equal(t, "0 */2 * * *", cadenceToCron("2h"))
	assert.Equal(t, "0 * * * *", cadenceToCron("1h"))

	// 6. parseTimeRFC3339
	tm, ok := parseTimeRFC3339("2026-11-05T12:00:00Z")
	assert.True(t, ok)
	assert.Equal(t, 2026, tm.Year())

	_, ok = parseTimeRFC3339("")
	assert.False(t, ok)

	// 7. countWorkItemsByPriority and countByP
	items := []ImprovementWorkItem{
		{Priority: "P0", Title: "Fix critical bug"},
		{Priority: "P1", Title: "Improve docs"},
		{Priority: "P1", Title: "Optimize query"},
	}
	counts := countWorkItemsByPriority(items)
	assert.Equal(t, 1, counts["P0"])
	assert.Equal(t, 2, counts["P1"])
	assert.Equal(t, 1, countByP(items, "P0"))
	assert.Equal(t, 2, countByP(items, "P1"))
	assert.Equal(t, 0, countByP(items, "P3"))

	// 8. buildComparative
	snap := ImprovementReportSnapshot{
		ObjectCount: map[string]any{"goal": 3, "requirement": 10},
	}
	comp := buildComparative(prevHighLoad, snap)
	assert.Equal(t, 0, comp.ObjectCountDelta)

	// 9. buildSummary
	sum := buildSummary(snap, comp, items)
	assert.Contains(t, sum, "Prioritized work items")

	// 10. formatImprovementReportTxt
	outReport := ImprovementReportOutput{
		GeneratedAt: "2026-11-05T12:00:00Z",
		Snapshot: ImprovementReportSnapshot{
			AdaptivePlan: plan,
			ObjectCount:  snap.ObjectCount,
		},
		Comparative: comp,
		WorkItems:   items,
	}
	txtBytes := formatImprovementReportTxt(outReport)
	assert.Contains(t, string(txtBytes), "Improvement Report")
}

func TestDataCells_FlagsAndFormatting(t *testing.T) {
	t.Parallel()

	// 1. validateDataCellsFlags
	assert.NoError(t, validateDataCellsFlags(false, false, false, false))
	assert.NoError(t, validateDataCellsFlags(true, true, false, false))
	assert.NoError(t, validateDataCellsFlags(false, false, true, false))

	// Invalid combinations
	assert.Error(t, validateDataCellsFlags(true, false, true, false))  // envelopePolicyDryRun with jsonOut
	assert.Error(t, validateDataCellsFlags(false, true, false, false)) // jsonEnvelope without jsonOut

	// 2. filterDataCellsByKind
	descriptors := []datacell.CellKindDescriptor{
		{Kind: "goal", CellID: "CELL-GOAL", StorageProfileWire: "stream"},
		{Kind: "backlog_item", CellID: "CELL-BLI", StorageProfileWire: "stream"},
	}
	filteredAll, err := filterDataCellsByKind(descriptors, "")
	assert.NoError(t, err)
	assert.Len(t, filteredAll, 2)

	// 3. Helpers
	assert.Equal(t, "present", contractField(true, "present"))
	assert.Equal(t, "", contractField(false, "present"))

	objMap := map[string]any{"name": "hello", "active": true}
	assert.Equal(t, "hello", stringFromObjectMap(objMap, "name"))
	assert.Equal(t, "", stringFromObjectMap(objMap, "missing"))
	assert.True(t, boolFromObjectMap(objMap, "active"))
	assert.False(t, boolFromObjectMap(objMap, "missing"))

	// 4. buildDataCellJSONRows
	tmpDir := t.TempDir()
	rows := buildDataCellJSONRows(tmpDir, descriptors)
	assert.Len(t, rows, 2)
	assert.Equal(t, "goal", rows[0].Kind)

	// 5. operationalEnvelopeTableFooter with parsed profile
	descriptorsWithProfile := []datacell.CellKindDescriptor{
		{Kind: "goal", CellID: "CELL-GOAL", StorageProfileWire: "stream", ParsedProfile: datacell.ProfileStream},
	}
	footer := operationalEnvelopeTableFooter(descriptorsWithProfile)
	assert.NotEmpty(t, footer)
}

func TestDashboard_PCSStatusAndHistorical(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Excellent", getPCSStatus(90.0))
	assert.Equal(t, "Good", getPCSStatus(75.0))
	assert.Equal(t, "Fair", getPCSStatus(50.0))
	assert.Equal(t, "Poor", getPCSStatus(30.0))
	assert.Equal(t, "Critical", getPCSStatus(10.0))
}

func TestSpecWriter_FieldOperations(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	specsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	sw, err := NewSpecWriter(tmpDir, "ACC-TEST", nil)
	require.NoError(t, err)
	require.NotNil(t, sw)

	spec := &objects.Spec{
		Ontology: "test_kind",
		Fields:   make(map[string]any),
	}

	// 1. createField
	opCreate := &FieldOperation{
		Operation: "create",
		FieldName: "title",
		FieldDef: map[string]any{
			"type":     "string",
			"required": true,
		},
	}
	err = sw.createField(spec, opCreate)
	assert.NoError(t, err)
	assert.Contains(t, spec.Fields, "title")

	// Creating duplicate field returns error
	err = sw.createField(spec, opCreate)
	assert.Error(t, err)

	// 2. modifyField
	opModify := &FieldOperation{
		Operation: "modify",
		FieldName: "title",
		FieldDef: map[string]any{
			"type":     "string",
			"required": false,
		},
	}
	err = sw.modifyField(spec, opModify)
	assert.NoError(t, err)

	// 3. deprecateField
	opDeprecate := &FieldOperation{
		Operation:  "deprecate",
		FieldName:  "title",
		ReplacedBy: "summary",
	}
	err = sw.deprecateField(spec, opDeprecate)
	assert.NoError(t, err)

	// 4. archiveField
	opArchive := &FieldOperation{
		Operation: "archive",
		FieldName: "title",
		Reason:    "legacy field replaced",
	}
	err = sw.archiveField(spec, opArchive)
	assert.NoError(t, err)

	// 5. deleteField
	opDelete := &FieldOperation{
		Operation: "delete",
		FieldName: "title",
	}
	err = sw.deleteField(spec, opDelete)
	assert.NoError(t, err)

	// 6. GetFieldChanges
	oldSpec := &objects.Spec{
		Ontology: "test_kind",
		Fields: map[string]any{
			"status": map[string]any{"type": "string"},
		},
	}
	newSpec := &objects.Spec{
		Ontology: "test_kind",
		Fields: map[string]any{
			"status":      map[string]any{"type": "string", "enum": []string{"open", "closed"}},
			"description": map[string]any{"type": "string"},
		},
	}
	changes, err := sw.GetFieldChanges(oldSpec, newSpec)
	assert.NoError(t, err)
	assert.NotEmpty(t, changes)

	// 7. WriteSpec
	err = sw.WriteSpec("test_kind", newSpec)
	assert.NoError(t, err)
}

func TestInitSnapshotHelpers_PureFunctions(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// validateSnapshotFile
	err := validateSnapshotFile(filepath.Join(tmpDir, "missing.tar.gz"))
	assert.Error(t, err)

	dummyFile := filepath.Join(tmpDir, "dummy.tar.gz")
	require.NoError(t, fileutil.WriteFile(dummyFile, []byte("data"), paths.FilePerm644))
	assert.NoError(t, validateSnapshotFile(dummyFile))

	// ensureObjectFields
	obj := map[string]any{"name": "test"}
	ensureObjectFields(obj, "GOAL-001", "goal")
	assert.Equal(t, "GOAL-001", obj[objects.FieldKeyID])
	assert.Equal(t, "goal", obj[objects.FieldKeyKind])

	// verifyProjectRoot
	assert.False(t, verifyProjectRoot(tmpDir)) // missing .zqk/process

	// handleWipeMode
	sysLogger := logging.GetLoggerFromProfile("system")
	err = handleWipeMode(tmpDir, false, false, sysLogger)
	assert.NoError(t, err)

	// setupSnapshotProjectDirectories
	err = setupSnapshotProjectDirectories(tmpDir, true)
	assert.NoError(t, err)
}

func TestSnapshotScenarioImplHelpers_Filtering(t *testing.T) {
	t.Parallel()

	// removeDuplicateIDs
	inputIDs := []string{"A", "B", "A", "C", "B"}
	deduped := removeDuplicateIDs(inputIDs)
	assert.Equal(t, []string{"A", "B", "C"}, deduped)
	assert.Empty(t, removeDuplicateIDs(nil))

	// checkSnapableTrait
	specWithTrait := &objects.Spec{Traits: []string{"snapable", "versioned"}}
	specWithoutTrait := &objects.Spec{Traits: []string{"auditable"}}
	assert.True(t, checkSnapableTrait(specWithTrait))
	assert.False(t, checkSnapableTrait(specWithoutTrait))
	assert.False(t, checkSnapableTrait(nil))

	// buildListFilterFromPreset
	presetMap := map[string]any{
		"filters":  map[string]any{"status": "active"},
		"sort_by":  "created_at",
		"sort_asc": true,
		"limit":    25,
	}
	filter := buildListFilterFromPreset("goal", presetMap)
	assert.Equal(t, "goal", filter.Kind)
	assert.Equal(t, "active", filter.Filters["status"])
	assert.Equal(t, "created_at", filter.SortBy)
	assert.True(t, filter.SortAsc)
	assert.Equal(t, 25, filter.Limit)

	// buildListFilterFromDefault
	defFilter := buildListFilterFromDefault("requirement", presetMap)
	assert.Equal(t, "requirement", defFilter.Kind)
	assert.Equal(t, 25, defFilter.Limit)

	// mergeListFilter
	base := storage.ListFilter{Kind: "goal", Limit: 10}
	extra := &storage.ListFilter{SortBy: "title", Limit: 50}
	merged := mergeListFilter(base, extra)
	assert.Equal(t, "goal", merged.Kind)
	assert.Equal(t, "title", merged.SortBy)
	assert.Equal(t, 50, merged.Limit)
}

func TestFeatureFlagsAndCliHooks_Governance(t *testing.T) {
	tmpDir := t.TempDir()

	// Feature flags helpers
	secCtx := getSecurityContextForFeatureFlags()
	assert.NotNil(t, secCtx)
	assert.True(t, hasPermission(secCtx, "feature_flags:read"))
	assert.True(t, hasPermission(secCtx, "feature_flags:write"))

	cmdFlags := NewFeatureFlagsCmd()
	assert.NotNil(t, cmdFlags)

	ff := featureflags.GetGlobalFeatureFlags(tmpDir)
	assert.NoError(t, setFeatureFlag(cmdFlags, ff, "test_flag", true))
	assert.NoError(t, getFeatureFlag(cmdFlags, ff, "test_flag"))
	assert.NoError(t, listFeatureFlags(cmdFlags, ff))

	// CLI Hooks helpers
	assert.NoError(t, requireCliHooksArgs([]string{"a", "b"}, 2, "usage"))
	assert.Error(t, requireCliHooksArgs([]string{"a"}, 2, "usage"))
	assert.NoError(t, requireCliHookWritePermission())

	cmdHooks := NewCliHooksCmd()
	assert.NotNil(t, cmdHooks)
}

func TestPolicyInterrupts_ListAndAck(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	cmd := NewPolicyInterruptsCmd()

	// List in empty project directly calling runPolicyInterruptsList
	err = runPolicyInterruptsList(cmd, nil)
	assert.NoError(t, err)

	// appendPolicyAck
	err = appendPolicyAck(tmpDir, "qa-disparity-001", "ACC-TEST", "resolved in cycle 43", "resumed")
	assert.NoError(t, err)
}

func TestEvolve_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	cmd := NewEvolveCmd()

	opts := &EvolveOptions{
		DryRun:     true,
		BrandOnly:  true,
		Limit:      3,
		OutputPath: filepath.Join(tmpDir, "evolve.json"),
	}

	// Change dir to tmpDir
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	err = runEvolve(cmd, opts)
	assert.NoError(t, err)
}
