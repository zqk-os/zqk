package utility

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestScenarioBuilder_BuildGeneration(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	targetDir := filepath.Join(proj.Root, "build-target")
	require.NoError(t, fileutil.MkdirAll(datacell.ProcessPrimaryDir(targetDir), paths.DirPerm755))

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, targetDir)
	builder := NewScenarioBuilder(targetDir, storageProvider, coordinator)
	builder.config.TargetDir = targetDir
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Configure builder with small count and link probability
	builder.
		WithKinds("goal", "backlog_item").
		WithCount("goal", 2).
		WithCount("backlog_item", 2).
		WithDefaultDiversity(5).
		WithLinkProbability(0.8)

	ctx := storage.WithSkipWriteBehind(context.Background())

	// Run Build
	err := builder.Build(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, builder.generated["goal"])
	assert.Equal(t, 2, builder.generated["backlog_item"])
}

func TestScenarioBuilder_ProcessObjectsSequentially(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	require.NoError(t, fileutil.MkdirAll(processDir, paths.DirPerm755))

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
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

	// Prepare dataFileObjects:
	// 1. Valid goal
	// 2. Backlog item referencing the goal
	// 3. Object referencing non-existent goal (should be skipped)
	// 4. Object missing required fields (should be skipped)
	builder.dataFileObjects = []map[string]any{
		{
			objects.FieldKeyKind:   objects.KindGoal,
			objects.FieldKeyID:     "GOAL-SEQ-001",
			objects.FieldKeyTitle:  "Sequential Goal",
			objects.FieldKeyStatus: "active",
		},
		{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-SEQ-001",
			objects.FieldKeyTitle:           "Sequential BLI",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyRequirementRefs: []any{"REQ-NONEXISTENT-999"}, // Will test reference existence check
		},
		{
			objects.FieldKeyKind: "invalid_kind_without_title",
		},
	}

	ctx := storage.WithSkipWriteBehind(context.Background())
	created, skipped := builder.processObjectsSequentially(ctx, state)

	assert.True(t, created >= 1)
	assert.True(t, skipped >= 1)
}

func TestReconstructObjectState(t *testing.T) {
	tmpDir := t.TempDir()
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	storageProvider := setupTestStorageProvider(t, tmpDir)
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	ctx := storage.WithSkipWriteBehind(context.Background())

	// Non-existent object should return error
	_, err := builder.reconstructObjectState(ctx, storageProvider, "BLI-NONE", "backlog_item", "hash123", tmpDir)
	assert.Error(t, err)
}
