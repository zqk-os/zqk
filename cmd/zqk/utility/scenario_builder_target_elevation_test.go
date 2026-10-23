package utility

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestScenarioBuilder_DataLoader_LockAndMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)

	// 1. getLockMetricsRecorder
	recorder := builder.getLockMetricsRecorder()
	assert.NotNil(t, recorder)

	// 2. withLock
	var mu sync.Mutex
	executed := false
	withLock(&mu, func() {
		executed = true
	})
	assert.True(t, executed)

	// 3. withLockTimeout - success
	ctx := context.Background()
	err := builder.withLockTimeout(ctx, &mu, "success_op", func() error {
		return nil
	})
	assert.NoError(t, err)

	// 4. withLockTimeout - timeout/cancelled
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = builder.withLockTimeout(cancelCtx, &mu, "timeout_op", func() error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})
	assert.Error(t, err)

	// 5. withLockTimeoutDuration
	err = builder.withLockTimeoutDuration(&mu, 100*time.Millisecond, "duration_op", func() error {
		return nil
	})
	assert.NoError(t, err)
}

func TestScenarioBuilder_DataLoader_PathsAndCache(t *testing.T) {
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
	builder.config = &ScenarioBuilderConfig{
		TargetDir: proj.Root,
	}

	// 1. reconstructFilePath variations
	p1 := builder.reconstructFilePath("ACC-001", objects.KindAccount)
	assert.NotEmpty(t, p1)
	p2 := builder.reconstructFilePath("account:john", objects.KindAccount)
	assert.Contains(t, p2, "account-john.yaml")
	p3 := builder.reconstructFilePath("GOAL-001", objects.KindGoal)
	assert.NotEmpty(t, p3)
	p4 := builder.reconstructFilePath("XYZ", "invalid_unknown_kind")
	assert.Empty(t, p4)

	// Nil config / empty target dir
	bEmpty := NewScenarioBuilder("", storageProvider, coordinator)
	bEmpty.config = nil
	assert.Empty(t, bEmpty.reconstructFilePath("GOAL-001", objects.KindGoal))

	// 2. getObjectFilePath
	filePath := builder.getObjectFilePath("GOAL-001", objects.KindGoal)
	assert.NotEmpty(t, filePath)

	// Ensure file exists on disk so updateObjectIDCacheSync stat check succeeds
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(filePath), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filePath, []byte("id: GOAL-001\nkind: goal\n"), paths.FilePerm644))

	// 3. updateObjectIDCacheSync
	err := builder.updateObjectIDCacheSync("GOAL-001", objects.KindGoal, filePath)
	assert.NoError(t, err)

	// Cache should now hit
	cachedPath := builder.getObjectFilePath("GOAL-001", objects.KindGoal)
	assert.Contains(t, cachedPath, "GOAL-001.yaml")
}

func TestScenarioBuilder_Dependency_CAS(t *testing.T) {
	builder := &ScenarioBuilder{}
	assert.True(t, builder.usesContentAddressableStorage(objects.KindAccount))
	assert.True(t, builder.usesContentAddressableStorage(objects.KindKeystoreEntry))
	assert.False(t, builder.usesContentAddressableStorage(objects.KindGoal))
	assert.False(t, builder.usesContentAddressableStorage(objects.KindBacklogItem))
}

func TestScenarioBuilder_GenerateObject_AllKindsAndDiversities(t *testing.T) {
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
	builder.config = &ScenarioBuilderConfig{
		TargetDir:        proj.Root,
		DefaultCount:     5,
		DefaultDiversity: 3,
		StartTime:        time.Now().Add(-24 * time.Hour),
		TimeRange:        24 * time.Hour,
	}

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

	specLoader := objects.GetGlobalSpecLoader()
	validStatuses := []string{scenarioBuilderStatusExploring}

	for i, kind := range kinds {
		spec, _ := specLoader.LoadSpecWithInheritance(kind + ".yaml")

		// Low diversity
		objLow := builder.generateObject(kind, i, 1, spec, validStatuses)
		assert.NotNil(t, objLow)
		assert.Equal(t, kind, objLow[objects.FieldKeyKind])

		// High diversity (>5)
		objHigh := builder.generateObject(kind, i, 8, spec, validStatuses)
		assert.NotNil(t, objHigh)
		assert.Equal(t, kind, objHigh[objects.FieldKeyKind])
	}
}

func TestScenarioBuilder_CreateHelpers_UpdateScenario(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
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
	builder.config = &ScenarioBuilderConfig{
		TargetDir: proj.Root,
		Kinds:     []string{objects.KindGoal},
	}

	ctx := storage.WithSkipWriteBehind(context.Background())

	// Create initial scenario with valid prefix SCE-
	scenarioID := "SCE-998"
	err := builder.createNewScenario(ctx, "Test Scenario", scenarioID)
	require.NoError(t, err)

	// updateScenarioOnConflict
	err = builder.updateScenarioOnConflict(ctx, "Test Scenario", scenarioID)
	assert.NoError(t, err)

	// updateExistingScenario
	existing := map[string]any{
		objects.FieldKeyID: scenarioID,
	}
	err = builder.updateExistingScenario(ctx, existing)
	assert.NoError(t, err)
}

func TestScenarioBuilder_CopyHelpers_HashAndReconstruct(t *testing.T) {
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

	ctx := storage.WithSkipWriteBehind(context.Background())
	obj := map[string]any{
		objects.FieldKeyID:    "GOAL-HASH-001",
		objects.FieldKeyKind:  objects.KindGoal,
		objects.FieldKeyTitle: "Hash Goal",
	}

	// 1. handleHashChange - reject
	cfgReject := &ScenarioCopyConfig{
		ChangePolicy: "reject",
	}
	resObj, keep := handleHashChange(ctx, builder, storageProvider, "GOAL-HASH-001", objects.KindGoal, "hash1", "hash2", cfgReject, obj)
	assert.False(t, keep)
	assert.Nil(t, resObj)

	// 2. handleHashChange - reconstruct (error fallback)
	cfgRecon := &ScenarioCopyConfig{
		ChangePolicy:      "reconstruct",
		SourceProjectRoot: proj.Root,
	}
	resObj, keep = handleHashChange(ctx, builder, storageProvider, "GOAL-HASH-001", objects.KindGoal, "hash1", "hash2", cfgRecon, obj)
	assert.True(t, keep)
	assert.NotNil(t, resObj)

	// 3. handleHashChange - include
	cfgInclude := &ScenarioCopyConfig{
		ChangePolicy: "include",
	}
	resObj, keep = handleHashChange(ctx, builder, storageProvider, "GOAL-HASH-001", objects.KindGoal, "hash1", "hash2", cfgInclude, obj)
	assert.True(t, keep)
	assert.NotNil(t, resObj)

	// 4. verifyObjectHash - no snapshot
	cfgNoSnap := &ScenarioCopyConfig{
		SnapshotHashes: map[string]string{},
	}
	resObj, keep = verifyObjectHash(ctx, builder, storageProvider, "GOAL-HASH-001", objects.KindGoal, cfgNoSnap, obj)
	assert.True(t, keep)
	assert.NotNil(t, resObj)
}

func TestFixHashes_ValidateObjectFields(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Missing ID
	id, kind, valid := validateObjectFields(map[string]any{"kind": "goal"}, "path/to/file.yaml", false, logger)
	assert.False(t, valid)
	assert.Empty(t, id)
	assert.Empty(t, kind)

	// Missing Kind
	id, kind, valid = validateObjectFields(map[string]any{"id": "GOAL-001"}, "path/to/file.yaml", false, logger)
	assert.False(t, valid)
	assert.Empty(t, id)
	assert.Empty(t, kind)

	// Valid
	id, kind, valid = validateObjectFields(map[string]any{"id": "GOAL-001", "kind": "goal"}, "path/to/file.yaml", false, logger)
	assert.True(t, valid)
	assert.Equal(t, "GOAL-001", id)
	assert.Equal(t, "goal", kind)
}

func TestScenarioBuilder_BuildFromObjects_WithRealObject(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	targetDir := filepath.Join(proj.Root, "copy-target")
	require.NoError(t, fileutil.MkdirAll(datacell.ProcessPrimaryDir(targetDir), paths.DirPerm755))

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, targetDir)
	builder := NewScenarioBuilder(targetDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create an object in proj
	goalID := "GOAL-BUILD-001"
	goalObj := map[string]any{
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyID:     goalID,
		objects.FieldKeyTitle:  "Build Goal",
		objects.FieldKeyStatus: "active",
	}
	ctx := storage.WithSkipWriteBehind(context.Background())
	require.NoError(t, proj.FileStorage.Create(ctx, builder.secCtx, goalObj))

	builder.copyConfig = &ScenarioCopyConfig{
		TargetDir:         targetDir,
		SourceProjectRoot: proj.Root,
		ObjectIDs:         []string{goalID},
		SnapshotHashes:    make(map[string]string),
	}

	err := builder.BuildFromObjects(ctx)
	assert.NoError(t, err)
}

func TestScenarioBuilder_BuildFromDataFile_Pipeline(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	targetDir := filepath.Join(proj.Root, "pipeline-target")
	require.NoError(t, fileutil.MkdirAll(datacell.ProcessPrimaryDir(targetDir), paths.DirPerm755))

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, targetDir)
	builder := NewScenarioBuilder(targetDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.config = &ScenarioBuilderConfig{
		TargetDir: targetDir,
		Kinds:     []string{objects.KindGoal},
	}

	builder.dataFileObjects = []map[string]any{
		{
			objects.FieldKeyKind:   objects.KindGoal,
			objects.FieldKeyID:     "GOAL-PIPE-001",
			objects.FieldKeyTitle:  "Pipeline Goal",
			objects.FieldKeyStatus: "active",
		},
	}

	ctx := storage.WithSkipWriteBehind(context.Background())
	err := builder.BuildFromDataFile(ctx)
	assert.NoError(t, err)
}

func TestScenarioBuilder_ProcessObjectsSequentially_ForceAndDuplicates(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
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

	goalID := "GOAL-SEQ-DUP-001"
	builder.dataFileObjects = []map[string]any{
		{
			objects.FieldKeyKind:   objects.KindGoal,
			objects.FieldKeyID:     goalID,
			objects.FieldKeyTitle:  "Original Goal",
			objects.FieldKeyStatus: "active",
		},
		{
			objects.FieldKeyKind:   objects.KindGoal,
			objects.FieldKeyID:     goalID,
			objects.FieldKeyTitle:  "Duplicate Goal Without Force",
			objects.FieldKeyStatus: "active",
		},
	}

	ctx := storage.WithSkipWriteBehind(context.Background())

	// 1. Without force: creates first, skips duplicate
	builder.force = false
	created, skipped := builder.processObjectsSequentially(ctx, state)
	assert.Equal(t, 1, created)
	assert.Equal(t, 1, skipped)

	// 2. With force: updates existing duplicate
	builder.force = true
	builder.dataFileObjects = []map[string]any{
		{
			objects.FieldKeyKind:   objects.KindGoal,
			objects.FieldKeyID:     goalID,
			objects.FieldKeyTitle:  "Duplicate Goal With Force",
			objects.FieldKeyStatus: "active",
		},
	}
	created, skipped = builder.processObjectsSequentially(ctx, state)
	assert.Equal(t, 1, created)
	assert.Equal(t, 0, skipped)
}

func TestScenarioBuilder_CreateObjectInLayer_ForceAndDuplicates(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
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

	goalID := "GOAL-LAYER-DUP-001"
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyID:     goalID,
		objects.FieldKeyTitle:  "Layer Goal",
		objects.FieldKeyStatus: "active",
	}

	ctx := storage.WithSkipWriteBehind(context.Background())
	var layerCreated int
	var layerCreatedMu, layerSkippedMu sync.Mutex

	// 1. Initial creation
	builder.force = false
	err := builder.createObjectInLayer(ctx, obj, 0, 0, state, &layerCreatedMu, &layerSkippedMu, &layerCreated)
	assert.NoError(t, err)
	assert.Equal(t, 1, layerCreated)

	// 2. Duplicate without force (silently skips)
	err = builder.createObjectInLayer(ctx, obj, 1, 0, state, &layerCreatedMu, &layerSkippedMu, &layerCreated)
	assert.NoError(t, err)

	// 3. Duplicate with force (updates existing)
	builder.force = true
	objUpdate := map[string]any{
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyID:     goalID,
		objects.FieldKeyTitle:  "Layer Goal Updated With Force",
		objects.FieldKeyStatus: "active",
	}
	err = builder.createObjectInLayer(ctx, objUpdate, 2, 0, state, &layerCreatedMu, &layerSkippedMu, &layerCreated)
	assert.NoError(t, err)
	assert.Equal(t, 2, layerCreated)
}

func TestReadObjectFromSource(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())
	ctx := context.Background()

	// 1. Not found error
	_, err := readObjectFromSource(ctx, proj.FileStorage, "NONEXISTENT", logger)
	assert.Error(t, err)

	// 2. Found
	goalID := "GOAL-READ-001"
	require.NoError(t, proj.FileStorage.Create(ctx, pkgctx.NewSystemSecurityContext(), map[string]any{
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyID:     goalID,
		objects.FieldKeyTitle:  "Read Goal",
		objects.FieldKeyStatus: "active",
	}))
	obj, err := readObjectFromSource(ctx, proj.FileStorage, goalID, logger)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
}

func TestScenarioBuilder_CopyConfigFile_ErrorsAndSuccess(t *testing.T) {
	builder := &ScenarioBuilder{}
	tmpDir := t.TempDir()

	// 1. Missing source
	err := builder.copyConfigFile(filepath.Join(tmpDir, "missing.yaml"), filepath.Join(tmpDir, "target.yaml"))
	assert.Error(t, err)

	// 2. Success
	srcPath := filepath.Join(tmpDir, "src.yaml")
	dstPath := filepath.Join(tmpDir, "dst.yaml")
	require.NoError(t, fileutil.WriteFile(srcPath, []byte("key: value\n"), paths.FilePerm644))
	err = builder.copyConfigFile(srcPath, dstPath)
	assert.NoError(t, err)
}
