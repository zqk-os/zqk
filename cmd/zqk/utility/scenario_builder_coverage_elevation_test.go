package utility

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	instancebuilders "github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestScenarioBuilder_InfrastructureFileAndDirCopy(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Test copyFile
	sourceFile := filepath.Join(tmpDir, "source.txt")
	require.NoError(t, fileutil.WriteFile(sourceFile, []byte("test content"), paths.FilePerm644))

	targetFile := filepath.Join(tmpDir, "nested", "target.txt")
	err := builder.copyFile(sourceFile, targetFile)
	require.NoError(t, err)

	content, err := fileutil.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))

	// Test copyFile error path: non-existent source
	err = builder.copyFile(filepath.Join(tmpDir, "nonexistent.txt"), filepath.Join(tmpDir, "out.txt"))
	assert.Error(t, err)

	// Test copyDirectory
	sourceDir := filepath.Join(tmpDir, "srcdir")
	require.NoError(t, fileutil.MkdirAll(sourceDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(sourceDir, "file1.txt"), []byte("f1"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(sourceDir, "file2.txt"), []byte("f2"), paths.FilePerm644))

	targetDir := filepath.Join(tmpDir, "destdir")
	err = builder.copyDirectory(sourceDir, targetDir)
	require.NoError(t, err)

	assert.True(t, fileutil.Exists(filepath.Join(targetDir, "file1.txt")))
	assert.True(t, fileutil.Exists(filepath.Join(targetDir, "file2.txt")))

	// Test copyDirectory error path: non-existent source
	err = builder.copyDirectory(filepath.Join(tmpDir, "nonexistent_dir"), filepath.Join(tmpDir, "out_dir"))
	assert.Error(t, err)
}

func TestScenarioBuilder_HandleLoadFromScenario(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := proj.FileStorage
	builder := NewScenarioBuilder(proj.Root, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	ctx := storage.WithSkipWriteBehind(context.Background())

	// 1. Create a valid scenario object in storage
	scenarioID := "SCE-999"
	scenarioObj := map[string]any{
		objects.FieldKeyKind:          objects.KindScenario,
		objects.FieldKeyID:            scenarioID,
		objects.FieldKeyTitle:         "Test Scenario: my-scenario",
		objects.FieldKeyStatus:        scenarioBuilderStatusProposed,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyDataGenerationConfig: map[string]any{
			"kinds":         []any{"goal", "milestone"},
			"default_count": 10,
		},
	}
	require.NoError(t, storageProvider.Create(storage.WithSkipWriteBehind(ctx), builder.secCtx, scenarioObj))

	// Test loading directly by ID
	err := handleLoadFromScenario(builder, scenarioID, proj.Root)
	require.NoError(t, err)
	assert.Equal(t, scenarioID, builder.scenarioID)
	assert.Equal(t, []string{"goal", "milestone"}, builder.config.Kinds)

	// Test loading by name fallback (when scenarioID is empty, targetDir base provides identifier)
	targetDirWithName := filepath.Join(proj.Root, scenarioID)
	err = handleLoadFromScenario(builder, "", targetDirWithName)
	require.NoError(t, err)
	assert.Equal(t, scenarioID, builder.scenarioID)

	// Test failure when neither ID nor name match
	err = handleLoadFromScenario(builder, "SCE-NONEXISTENT", filepath.Join(proj.Root, "totally-unknown"))
	assert.Error(t, err)
}

func TestScenarioBuilder_EventSubscriberDeactivate(t *testing.T) {
	subscriber := NewScenarioBuilderEventSubscriber("SCN-TEST")
	assert.True(t, subscriber.IsActive())
	subscriber.Deactivate()
	assert.False(t, subscriber.IsActive())
	assert.Equal(t, []string{}, subscriber.EventTypes())
}

func TestScenarioBuilder_MetricRecorderFactory(t *testing.T) {
	factory := NewMetricRecorderFactory(true)
	assert.NotNil(t, factory)

	builder := NewMetricBuilder("test_op")
	assert.NotNil(t, builder)

	noop := GetNoOpMetricRecorder()
	assert.NotNil(t, noop)
}

func TestScenarioBuilder_LifecycleToObjectMap_FullFeatures(t *testing.T) {
	lc := &objects.Lifecycle{
		ObjectType: "custom_type",
		Extends:    "base_object",
		StatusMapping: map[string]string{
			"start": "proposed",
		},
		Statuses: []objects.Status{
			{
				Value:         "init",
				Display:       "Initialization",
				Origin:        true,
				Terminal:      false,
				Archive:       false,
				System:        false,
				Preconditions: []string{"none"},
				Description:   "Initial state",
			},
			{
				Value:    "done",
				Display:  "Completed",
				Terminal: true,
			},
		},
		Transitions: []objects.Transition{
			{
				From:          "init",
				To:            "done",
				Description:   "Completion transition",
				Manual:        true,
				Auto:          false,
				Preconditions: []string{"check_all"},
			},
		},
		PercentComplete: objects.PercentCompleteConfig{
			Method: "weighted",
			DefaultByStatus: map[string]any{
				"init": 0,
				"done": 100,
			},
			MilestoneBased: map[string]any{
				"phase1": 50,
			},
		},
	}

	obj, err := lifecycleToObjectMap(lc, "custom_type", "001")
	require.NoError(t, err)
	assert.NotNil(t, obj)
	assert.Equal(t, "LIFECYCLE-CUSTOM-TYPE-001", obj[objects.FieldKeyID])
	assert.Equal(t, "base_object", obj[objects.FieldKeyExtends])
}

func TestConfigureBuilderFromFlags_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)

	// Missing kinds
	flags := &ScenarioBuilderFlags{
		TargetDir: tmpDir,
		Kinds:     []string{},
	}
	err := configureBuilderFromFlags(builder, flags)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--kinds is required")

	// Valid kinds
	flags.Kinds = []string{"goal"}
	flags.Counts = map[string]int{"goal": 10}
	flags.Diversity = map[string]int{"goal": 5}
	flags.DefaultCount = 10
	flags.DefaultDiversity = 5
	flags.LinkProb = 0.5
	flags.TimeRange = 24 * time.Hour

	err = configureBuilderFromFlags(builder, flags)
	assert.NoError(t, err)
	assert.Equal(t, 10, builder.config.Counts["goal"])
}

func TestScenarioBuilder_CopyScenarioBuilderProfileAndConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Test copyScenarioBuilderProfile
	err := builder.copyScenarioBuilderProfile(tmpDir)
	assert.NoError(t, err)

	expectedProfilePath := filepath.Join(tmpDir, "pkg", "cli", "profiles", ScenarioBuilderProfileName+".yaml")
	assert.True(t, fileutil.Exists(expectedProfilePath))

	// Test copyConfigFile
	srcConfig := filepath.Join(tmpDir, "sample_config.yaml")
	require.NoError(t, fileutil.WriteFile(srcConfig, []byte("key: value\n"), paths.FilePerm644))

	dstConfig := filepath.Join(tmpDir, "copied_config.yaml")
	err = builder.copyConfigFile(srcConfig, dstConfig)
	assert.NoError(t, err)
	assert.True(t, fileutil.Exists(dstConfig))

	// Error path: source does not exist
	err = builder.copyConfigFile(filepath.Join(tmpDir, "nonexistent.yaml"), dstConfig)
	assert.Error(t, err)
}

func TestScenarioBuilder_CreateObjectInLayer(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, proj.Root)
	builder := NewScenarioBuilder(proj.Root, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	counter := 0
	state := &layerProcessingState{
		referenceCache:      make(map[string]bool),
		referenceCacheMu:    &sync.Mutex{},
		idStream:            make(map[string]string),
		idStreamMu:          &sync.Mutex{},
		activeValidations:   &sync.WaitGroup{},
		validationCounter:   &counter,
		validationCounterMu: &sync.Mutex{},
	}

	layerCreatedMu := &sync.Mutex{}
	layerSkippedMu := &sync.Mutex{}
	layerCreated := 0

	ctx := storage.WithSkipWriteBehind(context.Background())

	// 1. Valid goal object
	goalObj := map[string]any{
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyID:     "GOAL-LAYER-001",
		objects.FieldKeyTitle:  "Layer Goal",
		objects.FieldKeyStatus: "active",
	}

	err := builder.createObjectInLayer(ctx, goalObj, 0, 0, state, layerCreatedMu, layerSkippedMu, &layerCreated)
	assert.NoError(t, err)
	assert.Equal(t, 1, layerCreated)

	// 2. Invalid object missing required fields -> should return error and not crash
	invalidObj := map[string]any{
		objects.FieldKeyKind: "goal",
		// missing title/status
	}
	_ = builder.createObjectInLayer(ctx, invalidObj, 1, 0, state, layerCreatedMu, layerSkippedMu, &layerCreated)
}

func TestScenarioBuilder_BuildFromObjects(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	targetDir := filepath.Join(proj.Root, "target_scenario")
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(targetDir)
		storage.ShutdownHashRegistriesForProjectTesting(targetDir)
		cli.UnregisterStorageForProjectRoot(proj.Root)
		storage.ShutdownHashRegistriesForProjectTesting(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	builder := NewScenarioBuilder(proj.Root, proj.FileStorage, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	builder.config.TargetDir = targetDir
	builder.copyConfig = &ScenarioCopyConfig{
		SourceProjectRoot: proj.Root,
		TargetDir:         targetDir,
		ObjectIDs:         []string{"OBJ-NONEXISTENT"},
		SnapshotHashes:    make(map[string]string),
	}

	ctx := context.Background()
	err := builder.BuildFromObjects(ctx)
	assert.NoError(t, err)

	err = builder.Build(ctx)
	assert.NoError(t, err)
}

func TestScenarioBuilder_AddKindSpecificFieldsToBuilder(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	builder := NewScenarioBuilder(proj.Root, proj.FileStorage, coordinator)

	kinds := []string{
		objects.KindAccount,
		scenarioBuilderKindBacklogItem,
		objects.KindGoal,
		objects.KindMilestone,
		objects.KindWorkstream,
		objects.KindRequirement,
		objects.KindCriteria,
		objects.KindTestCase,
		objects.KindAuditEvent,
		objects.KindChangeJournalEntry,
		objects.KindDocEntry,
	}

	for _, kind := range kinds {
		b := instancebuilders.NewForKind(kind, objects.DefaultSchemaVersion)
		builder.addKindSpecificFieldsToBuilder(b, kind, 0, 10, nil)
		builder.addKindSpecificFieldsToBuilder(b, kind, 1, 2, nil)
	}

	err := builder.createLinks(context.Background())
	assert.NoError(t, err)

	title := fixtureScenarioObjectTitle("goal", 3)
	assert.Equal(t, "Fixture: goal #3", title)
}

func TestScenarioBuilder_HandleHashChange_And_VerifyObjectHash(t *testing.T) {
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

	ctx := context.Background()
	obj := map[string]any{
		objects.FieldKeyID:   "GOAL-001",
		objects.FieldKeyKind: "goal",
	}

	configReject := &ScenarioCopyConfig{
		ChangePolicy:      "reject",
		SourceProjectRoot: proj.Root,
	}
	resObj, ok := handleHashChange(ctx, builder, proj.FileStorage, "GOAL-001", "goal", "hash1", "hash2", configReject, obj)
	assert.Nil(t, resObj)
	assert.False(t, ok)

	configInclude := &ScenarioCopyConfig{
		ChangePolicy:      "include",
		SourceProjectRoot: proj.Root,
	}
	resObj, ok = handleHashChange(ctx, builder, proj.FileStorage, "GOAL-001", "goal", "hash1", "hash2", configInclude, obj)
	assert.NotNil(t, resObj)
	assert.True(t, ok)

	configReconstruct := &ScenarioCopyConfig{
		ChangePolicy:      "reconstruct",
		SourceProjectRoot: proj.Root,
	}
	_, _ = handleHashChange(ctx, builder, proj.FileStorage, "GOAL-001", "goal", "hash1", "hash2", configReconstruct, obj)

	configVerify := &ScenarioCopyConfig{
		SnapshotHashes: map[string]string{
			"GOAL-001": "somehash",
		},
		ChangePolicy:      "reject",
		SourceProjectRoot: proj.Root,
	}
	res, proceed := verifyObjectHash(ctx, builder, proj.FileStorage, "GOAL-002", "goal", configVerify, obj)
	assert.True(t, proceed)
	assert.NotNil(t, res)

	res, _ = verifyObjectHash(ctx, builder, proj.FileStorage, "GOAL-001", "goal", configVerify, obj)
	assert.NotNil(t, res)
}

func TestScenarioBuilder_UpdateScenarioOnConflict(t *testing.T) {
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

	ctx := context.Background()
	scenarioID := "SCE-888"
	scenarioObj := map[string]any{
		objects.FieldKeyKind:          objects.KindScenario,
		objects.FieldKeyID:            scenarioID,
		objects.FieldKeyTitle:         "Conflict Test",
		objects.FieldKeyStatus:        scenarioBuilderStatusProposed,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, proj.FileStorage.Create(storage.WithSkipWriteBehind(ctx), builder.secCtx, scenarioObj))

	err := builder.updateScenarioOnConflict(ctx, "Updated Conflict", scenarioID)
	assert.NoError(t, err)

	existing, err := proj.FileStorage.Read(ctx, builder.secCtx, scenarioID)
	require.NoError(t, err)
	err = builder.updateExistingScenario(ctx, existing)
	assert.NoError(t, err)

	err = builder.updateScenarioOnConflict(ctx, "NonExistent", "SCE-000")
	assert.Error(t, err)
}

func TestScenarioBuilder_LockHelpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	builder := NewScenarioBuilder(proj.Root, proj.FileStorage, coordinator)

	var mu sync.Mutex

	called := false
	withLock(&mu, func() {
		called = true
	})
	assert.True(t, called)

	ctx := context.Background()
	err := builder.withLockTimeout(ctx, &mu, "test_op", func() error {
		return nil
	})
	assert.NoError(t, err)

	expectedErr := fmt.Errorf("custom error")
	err = builder.withLockTimeout(ctx, &mu, "test_op_err", func() error {
		return expectedErr
	})
	assert.ErrorIs(t, err, expectedErr)

	err = builder.withLockTimeoutDuration(&mu, 10*time.Millisecond, "test_timeout", func() error {
		time.Sleep(60 * time.Millisecond)
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestConfigFileWatcher_SetupDefaultWatchers(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	watcher := NewConfigFileWatcher(tmpDir, coordinator, logger)
	require.NotNil(t, watcher)

	watcher.SetupDefaultWatchers()
	assert.Equal(t, 4, len(watcher.watchedFiles))

	for filename, info := range watcher.watchedFiles {
		assert.NotEmpty(t, filename)
		assert.NotNil(t, info.ReloadFn)
		err := info.ReloadFn()
		assert.NoError(t, err)
	}
}
