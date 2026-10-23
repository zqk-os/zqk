package utility

import (
	"context"
	"path/filepath"
	"testing"

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
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestCopyDirectoryHelpers(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. validateSourceDirectory
	_, err := validateSourceDirectory(filepath.Join(tmpDir, "nonexistent"))
	assert.Error(t, err)

	dummyFile := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, fileutil.WriteFile(dummyFile, []byte("hello"), paths.FilePerm644))
	_, err = validateSourceDirectory(dummyFile)
	assert.Error(t, err)

	info, err := validateSourceDirectory(tmpDir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())

	// 2. shouldSkipFile
	assert.True(t, shouldSkipFile(info, "._file"))
	assert.True(t, shouldSkipFile(info, ".hidden"))
	assert.False(t, shouldSkipFile(info, "regular.txt"))

	// 3. copyFile and copyDirectory
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	targetCopyFile := filepath.Join(tmpDir, "copied_file.txt")
	err = builder.copyFile(dummyFile, targetCopyFile)
	require.NoError(t, err)

	copiedContent, err := fileutil.ReadFile(targetCopyFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), copiedContent)

	// copyDirectory
	targetSubDir := filepath.Join(tmpDir, "dest_dir")
	err = builder.copyDirectory(tmpDir, targetSubDir)
	require.NoError(t, err)
}

func TestCopyObjects_CaptureAndExecute(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	targetDir := filepath.Join(proj.Root, "scenario-target")
	require.NoError(t, fileutil.MkdirAll(datacell.ProcessPrimaryDir(targetDir), paths.DirPerm755))

	// Create source backlog item in proj.Root
	itemsDir := filepath.Join(datacell.ProcessPrimaryDir(proj.Root), "backlog_items")
	require.NoError(t, fileutil.MkdirAll(itemsDir, paths.DirPerm755))
	itemFile := filepath.Join(itemsDir, "BLI-001.yaml")
	itemData := []byte("id: BLI-001\nkind: backlog_item\ntitle: Source BLI\nstatus: planned\n")
	require.NoError(t, fileutil.WriteFile(itemFile, itemData, paths.FilePerm644))

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, targetDir)
	builder := NewScenarioBuilder(targetDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	cfg := &ScenarioCopyConfig{
		SourceProjectRoot: proj.Root,
		TargetDir:         targetDir,
		ObjectIDs:         []string{"BLI-001"},
		IDPrefixOverride:  "TEST-",
		NamespaceOverride: "test:scenario",
		PIIFields:         make(map[string]bool),
		FieldOverrides:    map[string]any{"status": "proposed"},
		PreserveState:     false,
		ChangePolicy:      "include",
		HashMappings:      make(map[string]string),
		SnapshotHashes:    make(map[string]string),
	}

	ctx := storage.WithSkipWriteBehind(context.Background())

	// Capture snapshot hashes
	err := builder.CaptureSnapshotHashes(ctx, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.SnapshotHashes["BLI-001"])

	// CopyObjectsFromProject
	err = builder.CopyObjectsFromProject(ctx, cfg)
	require.NoError(t, err)

	// calculateTotalCopied
	assert.True(t, calculateTotalCopied(map[string]int{"backlog_item": 2, "goal": 3}) == 5)
}

func TestScenarioBuilder_UpdateReferences(t *testing.T) {
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

	ctx := context.Background()

	// Create referenced target objects to satisfy Tier 2 reference validation
	require.NoError(t, storageProvider.Create(ctx, builder.secCtx, map[string]any{
		objects.FieldKeyKind:          "account",
		objects.FieldKeyID:            "ACC-ORIGINAL-001",
		objects.FieldKeyUsername:      "user-orig",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}))
	require.NoError(t, storageProvider.Create(ctx, builder.secCtx, map[string]any{
		objects.FieldKeyKind:          "account",
		objects.FieldKeyID:            "ACC-NEW-001",
		objects.FieldKeyUsername:      "user-new",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}))
	require.NoError(t, storageProvider.Create(ctx, builder.secCtx, map[string]any{
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyID:            "CRIT-ORIGINAL-001",
		objects.FieldKeyTitle:         "Crit Orig 1",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyStatus:        "originated",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}))
	require.NoError(t, storageProvider.Create(ctx, builder.secCtx, map[string]any{
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyID:            "CRIT-ORIGINAL-002",
		objects.FieldKeyTitle:         "Crit Orig 2",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyStatus:        "originated",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}))
	require.NoError(t, storageProvider.Create(ctx, builder.secCtx, map[string]any{
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyID:            "CRIT-NEW-001",
		objects.FieldKeyTitle:         "Crit New 1",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyStatus:        "originated",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}))
	require.NoError(t, storageProvider.Create(ctx, builder.secCtx, map[string]any{
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyID:            "CRIT-NEW-002",
		objects.FieldKeyTitle:         "Crit New 2",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyStatus:        "originated",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}))

	// Create an object with references
	obj := map[string]any{
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyID:            "BLI-TEST-001",
		objects.FieldKeyTitle:         "Referenced BLI",
		objects.FieldKeyOwnerRef:      "ACC-ORIGINAL-001",
		objects.FieldKeyCriteriaRefs:  []any{"CRIT-ORIGINAL-001", "CRIT-ORIGINAL-002"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, storageProvider.Create(ctx, builder.secCtx, obj))

	idMapping := map[string]string{
		"BLI-TEST-001":      "BLI-TEST-001",
		"ACC-ORIGINAL-001":  "ACC-NEW-001",
		"CRIT-ORIGINAL-001": "CRIT-NEW-001",
		"CRIT-ORIGINAL-002": "CRIT-NEW-002",
	}

	err := builder.updateReferences(ctx, idMapping, &ScenarioCopyConfig{})
	assert.NoError(t, err)

	// Verify references updated
	updatedObj, err := storageProvider.Read(ctx, builder.secCtx, "BLI-TEST-001")
	require.NoError(t, err)
	assert.Equal(t, "ACC-NEW-001", updatedObj[objects.FieldKeyOwnerRef])

	critRefs, ok := updatedObj[objects.FieldKeyCriteriaRefs].([]any)
	require.True(t, ok)
	assert.Contains(t, critRefs, "CRIT-NEW-001")
	assert.Contains(t, critRefs, "CRIT-NEW-002")
}

func TestScenarioCopyConfig_ValidationAndHelpers(t *testing.T) {
	// Empty ObjectIDs should fail validation
	err := validateCopyConfig(&ScenarioCopyConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no object IDs specified")

	// Valid config
	err = validateCopyConfig(&ScenarioCopyConfig{ObjectIDs: []string{"BLI-001"}})
	assert.NoError(t, err)

	// inferKindFromID
	idValidator := validation.GetIDValidator()
	_ = idValidator.LoadPatterns()
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())

	kind := inferKindFromID("BLI-12345", idValidator, logger)
	assert.Equal(t, objects.KindBacklogItem, kind)

	// Unknown prefix
	kindUnknown := inferKindFromID("UNKNOWN-12345", idValidator, logger)
	assert.Empty(t, kindUnknown)
}
