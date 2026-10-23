package utility

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestValidateYAML_InitializeContextAndSummary(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	cmd := &cobra.Command{Use: "test"}
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	// registerHashes = false
	c, l, err := initializeValidationContext(cmd, false)
	assert.NoError(t, err)
	assert.Nil(t, c)
	assert.Nil(t, l)

	// registerHashes = true with valid command
	c, l, err = initializeValidationContext(cmd, true)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.NotNil(t, l)

	// outputValidationSummary
	var out bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &out))
	result := &ValidateYAMLResult{
		ValidCount:     5,
		InvalidCount:   1,
		RegisterHashes: true,
		FixedCount:     3,
		Errors:         []string{"sample error 1"},
	}
	outputValidationSummary(cmd, result)
	assert.Contains(t, out.String(), "Validation summary")
	assert.Contains(t, out.String(), "3 hashes registered")
	assert.Contains(t, out.String(), "sample error 1")
}

func TestFixHashes_ShowProgressAndCollectFiles(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	// 1. showProgress
	showProgress(cmd, true, 0, 50)  // quiet -> no-op
	showProgress(cmd, false, 0, 50) // initial batch
	assert.Contains(t, buf.String(), "Processing 50 objects...")

	buf.Reset()
	showProgress(cmd, false, 100, 200) // milestone batch
	assert.Contains(t, buf.String(), "Processing 100/200 objects...")

	// 2. collectFilesFromArgs
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "valid.yaml")
	require.NoError(t, fileutil.WriteFile(yamlFile, []byte("kind: goal\n"), paths.FilePerm644))
	txtFile := filepath.Join(tmpDir, "notes.txt")
	require.NoError(t, fileutil.WriteFile(txtFile, []byte("text"), paths.FilePerm644))

	// Single files
	files, errs := collectFilesFromArgs([]string{yamlFile, txtFile}, false, false)
	assert.Equal(t, 2, len(files))
	assert.True(t, len(errs) > 0) // warning about .txt

	// Directory without recursive
	_, dirErrs := collectFilesFromArgs([]string{tmpDir}, false, false)
	assert.True(t, len(dirErrs) > 0)
	assert.Contains(t, dirErrs[0], "use --recursive")

	// Directory with recursive
	recFiles, recErrs := collectFilesFromArgs([]string{tmpDir}, true, false)
	assert.Equal(t, 1, len(recFiles))
	assert.Empty(t, recErrs)

	// Non-existent file
	_, notFoundErrs := collectFilesFromArgs([]string{filepath.Join(tmpDir, "ghost.yaml")}, false, false)
	assert.True(t, len(notFoundErrs) > 0)
	assert.Contains(t, notFoundErrs[0], "Error accessing")
}

func TestVersion_GetExecutableNameDirect(t *testing.T) {
	name := getExecutableName()
	assert.NotEmpty(t, name)
}

func TestScenarioBuilder_CopyHelpers_UpdateScenarioObjectAndCalculateTotal(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	builder := NewScenarioBuilder(proj.Root, proj.FileStorage, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// updateScenarioObject with empty scenarioID
	builder.scenarioID = ""
	updateScenarioObject(context.Background(), builder, &ScenarioCopyConfig{})

	// updateScenarioObject with existing scenarioID
	scenarioID := "SCE-777"
	scenarioObj := map[string]any{
		objects.FieldKeyKind:          objects.KindScenario,
		objects.FieldKeyID:            scenarioID,
		objects.FieldKeyTitle:         "Copy Scenario",
		objects.FieldKeyStatus:        scenarioBuilderStatusProposed,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	ctx := storage.WithSkipWriteBehind(context.Background())
	require.NoError(t, proj.FileStorage.Create(ctx, builder.secCtx, scenarioObj))

	builder.scenarioID = scenarioID
	cfg := &ScenarioCopyConfig{
		ObjectIDs:      []string{"GOAL-001"},
		SnapshotHashes: map[string]string{"GOAL-001": "h1"},
		HashMappings:   map[string]string{"h1": "h2"},
	}
	updateScenarioObject(ctx, builder, cfg)

	// calculateTotalCopied
	counts := map[string]int{
		"goal":      5,
		"milestone": 3,
	}
	total := calculateTotalCopied(counts)
	assert.Equal(t, 8, total)
}

func TestScenarioBuilder_Copy_ReconstructObjectState_Direct(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	builder := NewScenarioBuilder(proj.Root, proj.FileStorage, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	ctx := storage.WithSkipWriteBehind(context.Background())
	goalID := "GOAL-RECON-001"
	goalObj := map[string]any{
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeyID:            goalID,
		objects.FieldKeyTitle:         "Reconstruct Goal",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, builder.secCtx, goalObj))

	// 1. Non-existent object
	_, err := builder.reconstructObjectState(ctx, proj.FileStorage, "GOAL-GHOST", "goal", "target_hash", proj.Root)
	assert.Error(t, err)

	// 2. Hash already matches current
	currentHash, err := builder.getObjectHash(ctx, proj.FileStorage, goalID, "goal", proj.Root)
	require.NoError(t, err)
	reconstructed, err := builder.reconstructObjectState(ctx, proj.FileStorage, goalID, "goal", currentHash, proj.Root)
	assert.NoError(t, err)
	assert.NotNil(t, reconstructed)

	// 3. Hash differs -> returns not yet implemented error
	_, err = builder.reconstructObjectState(ctx, proj.FileStorage, goalID, "goal", "different_hash_value", proj.Root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet fully implemented")
}

func TestScenarioBuilder_CopyHelpers_LoadSpecForKind(t *testing.T) {
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())

	// Valid kind
	spec := loadSpecForKind(objects.KindGoal, logger)
	assert.NotNil(t, spec)

	// Invalid kind
	invalidSpec := loadSpecForKind("non_existent_kind_xyz", logger)
	assert.Nil(t, invalidSpec)
}

func TestScenarioBuilder_DataLoader_References_CollectAndMemoized(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	builder := NewScenarioBuilder(proj.Root, proj.FileStorage, coordinator)

	// 1. collectReferenceStrings
	anyRefs := collectReferenceStrings([]any{"REF-1", "REF-2", 42, ""})
	assert.Equal(t, []string{"REF-1", "REF-2"}, anyRefs)

	strRefs := collectReferenceStrings([]string{"REF-3", "REF-4", ""})
	assert.Equal(t, []string{"REF-3", "REF-4"}, strRefs)

	nilRefs := collectReferenceStrings("not-a-list")
	assert.Empty(t, nilRefs)

	// 2. referenceExistsMemoized and verifyReferencedObjectsExistMemoized
	idStream := map[string]string{"STREAM-001": "OBJ-001"}
	idStreamMu := &sync.Mutex{}
	refCache := map[string]bool{"CACHE-001": true}
	refCacheMu := &sync.Mutex{}

	ctx := context.Background()

	// Hit in idStream
	assert.True(t, builder.referenceExistsMemoized(ctx, "STREAM-001", idStream, idStreamMu, refCache, refCacheMu))

	// Hit in refCache
	assert.True(t, builder.referenceExistsMemoized(ctx, "CACHE-001", idStream, idStreamMu, refCache, refCacheMu))

	// Missing in all
	assert.False(t, builder.referenceExistsMemoized(ctx, "MISSING-001", idStream, idStreamMu, refCache, refCacheMu))

	// verifyReferencedObjectsExistMemoized - valid references
	validObj := map[string]any{
		"owner_ref":     "STREAM-001",
		"criteria_refs": []string{"CACHE-001"},
	}
	err := builder.verifyReferencedObjectsExistMemoized(ctx, validObj, "goal", idStream, idStreamMu, refCache, refCacheMu)
	assert.NoError(t, err)

	// verifyReferencedObjectsExistMemoized - missing reference
	invalidObj := map[string]any{
		"goal_ref": "MISSING-001",
	}
	err = builder.verifyReferencedObjectsExistMemoized(ctx, invalidObj, "req", idStream, idStreamMu, refCache, refCacheMu)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "referenced objects do not exist")
}

func TestScenarioBuilder_EventSubscriber_AllProfiles(t *testing.T) {
	profiles := []string{
		ScenarioBuilderProfileName,
		string(pkgctx.ProfileMCP),
		string(pkgctx.ProfileSystem),
		string(pkgctx.ProfileAIAgent),
		string(pkgctx.ProfileDebug),
		string(pkgctx.ProfileHuman),
		"",
		"custom_unknown_profile",
	}

	for _, p := range profiles {
		sub := NewScenarioBuilderEventSubscriber(p)
		assert.NotNil(t, sub)
		assert.True(t, sub.IsActive())
		sub.Deactivate()
		assert.False(t, sub.IsActive())
	}
}

func TestScenarioBuilder_HelpersBuilder_CreateScenarioBuilder(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	targetDir := filepath.Join(proj.Root, "sb_target")
	t.Cleanup(func() {
		storage.SetCacheOperationHandler(nil)
		cli.UnregisterStorageForProjectRoot(targetDir)
		storage.ShutdownHashRegistriesForProjectTesting(targetDir)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})
	cmd := &cobra.Command{Use: "scenario-builder"}
	stdCtx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(stdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	b, err := createScenarioBuilder(cmd, targetDir)
	assert.NoError(t, err)
	assert.NotNil(t, b)
}

func TestScenarioBuilder_CreateNewScenario_ConflictBranch(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	builder := NewScenarioBuilder(proj.Root, proj.FileStorage, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	ctx := storage.WithSkipWriteBehind(context.Background())
	scenarioID := "SCE-GAP-CONFLICT-777"
	scenarioName := "conflict-scenario"

	// First creation
	err := builder.createNewScenario(ctx, scenarioName, scenarioID)
	assert.NoError(t, err)

	// Second creation with same ID -> triggers already exists / conflict branch
	err = builder.createNewScenario(ctx, scenarioName, scenarioID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scenario already exists")
}
