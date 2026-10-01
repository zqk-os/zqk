package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestStorageExtended_Wave17_BucketingStrategyDefaultsAndLoader(t *testing.T) {
	ctx := context.Background()
	testRoot, _, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	t.Run("NewDefaultBucketStrategyRegistry_and_EnsureAllKinds", func(t *testing.T) {
		reg, err := NewDefaultBucketStrategyRegistry(ctx, testRoot)
		require.NoError(t, err)
		require.NotNil(t, reg)

		// EnsureAllKindsHaveStrategies
		strategies, err := reg.EnsureAllKindsHaveStrategies(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, strategies)

		// getAllSystemKinds and getCommonSystemKinds
		allKinds := reg.getAllSystemKinds()
		assert.NotEmpty(t, allKinds)
		commonKinds := reg.getCommonSystemKinds()
		assert.NotEmpty(t, commonKinds)
	})

	t.Run("BucketStrategyLoader_Methods", func(t *testing.T) {
		loader, err := NewBucketStrategyLoader(ctx, testRoot)
		require.NoError(t, err)
		require.NotNil(t, loader)

		all, err := loader.GetAllStrategies(ctx)
		require.NoError(t, err)
		assert.NotNil(t, all)

		err = loader.Reload(ctx)
		assert.NoError(t, err)

		bType := loader.GetBackendType()
		assert.NotEmpty(t, bType)

		gLoader, err := GetGlobalStrategyLoader(ctx, testRoot)
		require.NoError(t, err)
		assert.NotNil(t, gLoader)
	})
}

type mockStorageWave17 struct {
	mockStorageWave12
}

func (m *mockStorageWave17) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	return &QueryResult{Objects: []map[string]any{}}, nil
}

func TestStorageExtended_Wave17_GraphBucketStrategyStorage(t *testing.T) {
	ctx := context.Background()

	mock := &mockStorageWave17{}
	graphStorage := NewGraphBucketStrategyStorage(mock)
	require.NotNil(t, graphStorage)

	assert.Equal(t, "graph", graphStorage.GetBackendType())

	t.Run("LoadStrategy_and_LoadAllStrategies", func(t *testing.T) {
		// LoadStrategy
		strat, err := graphStorage.LoadStrategy(ctx, "STRAT-1")
		require.NoError(t, err)
		assert.NotNil(t, strat)

		// LoadAllStrategies
		strats, err := graphStorage.LoadAllStrategies(ctx)
		require.NoError(t, err)
		assert.Empty(t, strats)

		// LoadStrategiesForKind
		matching, err := graphStorage.LoadStrategiesForKind(ctx, "backlog_item")
		require.NoError(t, err)
		assert.Empty(t, matching)
	})

	t.Run("SaveStrategy_and_DeleteStrategy", func(t *testing.T) {
		// Missing ID -> error
		err := graphStorage.SaveStrategy(ctx, map[string]any{"type": "chrono"})
		require.Error(t, err)

		// Valid ID -> save
		strat := map[string]any{
			objects.FieldKeyID:        "STRAT-1",
			objects.FieldKeyAppliesTo: []any{"backlog_item"},
			objects.FieldKeyEnabled:   true,
		}
		err = graphStorage.SaveStrategy(ctx, strat)
		require.NoError(t, err)

		// DeleteStrategy
		err = graphStorage.DeleteStrategy(ctx, "STRAT-1")
		require.NoError(t, err)
	})
}

func TestStorageExtended_Wave17_Builtin_and_AsyncBulkDelete(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("IsBuiltInByID", func(t *testing.T) {
		assert.True(t, IsBuiltInByID("TYPE-system-core"))
		assert.True(t, IsBuiltInByID("BUILTIN-resolver-default"))
		assert.True(t, IsBuiltInByID("COMP-TYPE-101"))
		assert.False(t, IsBuiltInByID("GOAL-001"))
		assert.False(t, IsBuiltInByID("ITEM-123"))
	})

	t.Run("BulkDeleteJobManager_GetJobStatus_and_CleanupFailedJobs", func(t *testing.T) {
		mockStorage := &mockStorageWave17{}
		mgr := NewBulkDeleteJobManager(mockStorage)
		require.NotNil(t, mgr)

		// Non-existent job
		_, err := mgr.GetJobStatus("missing-job")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "job not found")

		// Job with failed errors -> CleanupFailedJobs succeeds
		now := time.Now()
		job := &BulkDeleteJob{
			ID:        "job-1",
			ObjectIDs: []string{"A", "B"},
			Status:    bulkDeleteJobStatusFailed,
			Errors: []BulkOperationError{
				{ID: "A", Message: "cannot delete A"},
			},
			StartedAt: &now,
		}
		mgr.jobs["job-1"] = job

		retrieved, err := mgr.GetJobStatus("job-1")
		require.NoError(t, err)
		assert.Equal(t, "job-1", retrieved.ID)
		assert.Len(t, retrieved.Errors, 1)

		cleanupJob, err := mgr.CleanupFailedJobs(ctx, secCtx, "job-1", 2)
		require.NoError(t, err)
		assert.NotNil(t, cleanupJob)

		// Job with no errors -> CleanupFailedJobs returns error
		jobNoErr := &BulkDeleteJob{
			ID:        "job-no-err",
			ObjectIDs: []string{"A"},
			Status:    bulkDeleteJobStatusCompleted,
			StartedAt: &now,
		}
		mgr.jobs["job-no-err"] = jobNoErr
		_, err = mgr.CleanupFailedJobs(ctx, secCtx, "job-no-err", 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no failed deletions")

		// Non-existent job for cleanup
		_, err = mgr.CleanupFailedJobs(ctx, secCtx, "missing-job", 2)
		require.Error(t, err)
	})
}

func TestStorageExtended_Wave17_DependencyGraph_and_CASWhitebox(t *testing.T) {
	t.Run("DependencyGraph_AddDependency", func(t *testing.T) {
		g := NewDependencyGraph()
		require.NotNil(t, g)

		// Add non-existent nodes
		err := g.AddDependency("depA", "depB")
		assert.NoError(t, err)

		// Add nodes then dependency
		g.nodes["node1"] = &DependencyNode{ID: "node1"}
		g.nodes["node2"] = &DependencyNode{ID: "node2"}

		err = g.AddDependency("node1", "node2")
		require.NoError(t, err)
		assert.Contains(t, g.nodes["node1"].DependsOn, "node2")
		assert.Contains(t, g.nodes["node2"].Dependents, "node1")

		// One node missing
		err = g.AddDependency("node1", "missingNode")
		assert.NoError(t, err)
	})

	t.Run("FileObjectStorage_GetContentAddressableStorageForTest", func(t *testing.T) {
		_, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
		cas, err := fos.GetContentAddressableStorageForTest(objects.KindAuditAggregationMetric)
		require.NoError(t, err)
		assert.NotNil(t, cas)
	})
}

func TestStorageExtended_Wave17_ChangeJournalShutdownHandler(t *testing.T) {
	ctx := context.Background()
	handler := &ChangeJournalShutdownHandler{}

	assert.Equal(t, "change_journal", handler.GetName())
	assert.True(t, handler.IsCritical())
	assert.True(t, handler.IsDrained())
	assert.Equal(t, int64(0), handler.GetPendingCount())

	err := handler.InitiateShutdown()
	assert.NoError(t, err)

	err = handler.Drain(ctx)
	assert.NoError(t, err)
}
