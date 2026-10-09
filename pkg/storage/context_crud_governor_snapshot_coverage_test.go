package storage

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestStorageExtended_ContextAndCRUDFacade(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteUnlinkContext", func(t *testing.T) {
		assert.False(t, UnlinkReferencesBeforeDelete(ctx))

		unlinkCtx := WithUnlinkReferencesBeforeDelete(ctx)
		assert.True(t, UnlinkReferencesBeforeDelete(unlinkCtx))
	})

	t.Run("CRUDFacadeExports", func(t *testing.T) {
		testRoot, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

		// ReadObjectFile
		_, err := fos.ReadObjectFile(ctx, filepath.Join(testRoot, "nonexistent.yaml"))
		assert.Error(t, err)

		// WriteBuffer inspection
		hasContent := fos.HasWriteBufferContent()
		assert.False(t, hasContent)

		op, data, pending := fos.GetWriteBufferObjectPending("backlog_item", "ITEM-1")
		assert.False(t, pending)
		assert.Empty(t, op)
		assert.Nil(t, data)

		// StreamStorageEnabledForKind
		streamEnabled := fos.StreamStorageEnabledForKind(objects.KindAuditEvent)
		_ = streamEnabled

		// RuntimeDelta
		deltaEnabled := fos.RuntimeDeltaEnabledForKind(testRoot, "backlog_item")
		assert.False(t, deltaEnabled)

		deltaState := fos.ReadRuntimeDeltaCurrentState(testRoot, "backlog_item", "ITEM-1")
		assert.Nil(t, deltaState)

		// Path inspection
		isStream := fos.IsStreamCurrentPath(testRoot, "/some/path")
		assert.False(t, isStream)

		isDraft := fos.IsObjectDraftPlanePath(testRoot, "/some/path")
		assert.False(t, isDraft)

		// StreamPathAndOffset
		p, offset, ok := fos.StreamPathAndOffset("/var/stream.json::42")
		assert.True(t, ok)
		assert.Equal(t, "/var/stream.json", p)
		assert.Equal(t, int64(42), offset)

		// ReadRecordAt (nonexistent)
		_, err = fos.ReadRecordAt("/nonexistent/file.json", 0)
		assert.Error(t, err)

		// CachedLivePath (uncached path returns empty string)
		cached := fos.CachedLivePath("/tmp/foo.yaml")
		assert.Equal(t, "", cached)

		// Overlays
		base := map[string]any{"id": "ITEM-1", "title": "Old"}
		overlay := fos.ApplyRuntimeDeltaOverlay(testRoot, "backlog_item", "ITEM-1", base)
		assert.NotNil(t, overlay)

		mat := fos.MaterializeCasYAMLMapAfterLoad(testRoot, "backlog_item", "ITEM-1", base)
		assert.NotNil(t, mat)

		// LiveCASBlobUnreadable
		assert.False(t, LiveCASBlobUnreadable(nil))
		assert.True(t, LiveCASBlobUnreadable(errors.New("content hash mismatch: abc vs def")))
	})
}

func TestStorageExtended_CriteriaHelpers_and_FileLock(t *testing.T) {
	ctx := context.Background()

	t.Run("NewCriteriaLookupFunc", func(t *testing.T) {
		mockStorage := &mockStorageWave17{}
		fn := NewCriteriaLookupFunc(mockStorage, false)
		require.NotNil(t, fn)

		// Known checklist item (traceability optional, returns empty string when not found)
		id, err := fn(ctx, "security")
		assert.NoError(t, err)
		_ = id

		// Unknown checklist item returns error
		_, err = fn(ctx, "completely_unknown_item_xyz")
		assert.Error(t, err)
	})

	t.Run("NewFileLockWithConfig", func(t *testing.T) {
		tempDir := t.TempDir()

		lockFile := filepath.Join(tempDir, "test.lock")
		cfg := FileLockConfig{
			EarlyBailoutThreshold: 2 * time.Second,
			EnableMetrics:         true,
		}
		fl, err := NewFileLockWithConfig(lockFile, cfg)
		require.NoError(t, err)
		require.NotNil(t, fl)
	})
}

func TestStorageExtended_Governor_and_GraphIndexes(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	conn := newInMemoryGraphConnWave13()
	graphStorage, err := NewGraphObjectStorage(conn, "test_project")
	require.NoError(t, err)
	require.NotNil(t, graphStorage)

	t.Run("CreateModifiedEdge_and_VerifyProvenance", func(t *testing.T) {
		err := graphStorage.CreateModifiedEdge(ctx, "EVT-1", "CODE-1")
		require.NoError(t, err)

		// VerifyProvenance: in-memory mock returns empty rows by default
		approved, err := graphStorage.VerifyProvenance(ctx, secCtx, "EVT-1")
		assert.NoError(t, err)
		assert.False(t, approved)
	})

	t.Run("EnsureSpecDerivedIndexes", func(t *testing.T) {
		// GraphObjectStorage
		err := graphStorage.EnsureSpecDerivedIndexes(ctx)
		assert.NoError(t, err)

		var nilGOS *GraphObjectStorage
		assert.NoError(t, nilGOS.EnsureSpecDerivedIndexes(ctx))

		// PoolAwareGraphStorage
		var nilPool *PoolAwareGraphStorage
		assert.NoError(t, nilPool.EnsureSpecDerivedIndexes(ctx))

		poolStorage := &PoolAwareGraphStorage{
			pool: nil,
		}
		assert.NoError(t, poolStorage.EnsureSpecDerivedIndexes(ctx))
	})
}

type mockPoolProviderWave18 struct {
	mockStorageWave17
	pool provider.ConnectionPool
}

func (m *mockPoolProviderWave18) GetPool() provider.ConnectionPool {
	return m.pool
}

func TestStorageExtended_BatchingAndCompressedSnapshot(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("BatchingObjectStorage_GetPool_and_shouldBypassBatching", func(t *testing.T) {
		// With pool
		mockPool := &mockPoolProviderWave18{}
		batcher1 := NewBatchingObjectStorage(mockPool)
		assert.Nil(t, batcher1.GetPool()) // pool field is nil

		// Without pool
		plainMock := &mockStorageWave17{}
		batcher2 := NewBatchingObjectStorage(plainMock)
		assert.Nil(t, batcher2.GetPool())

		// shouldBypassBatching
		assert.False(t, batcher2.shouldBypassBatching(ctx, secCtx))

		skipCtx := WithSkipWriteBehind(ctx)
		assert.True(t, batcher2.shouldBypassBatching(skipCtx, secCtx))

		promoteCtx := pkgctx.WithPromoteOnCreate(ctx)
		assert.True(t, batcher2.shouldBypassBatching(promoteCtx, secCtx))
	})

	t.Run("byCount_SortInterface", func(t *testing.T) {
		items := byCount{
			{key: "a", count: 10},
			{key: "b", count: 30},
			{key: "c", count: 20},
		}

		assert.Equal(t, 3, items.Len())
		assert.True(t, items.Less(1, 0)) // 30 > 10 (descending order)
		assert.False(t, items.Less(0, 1))

		items.Swap(0, 1)
		assert.Equal(t, "b", items[0].key)
		assert.Equal(t, "a", items[1].key)

		sort.Sort(items)
		assert.Equal(t, "b", items[0].key)
		assert.Equal(t, "c", items[1].key)
		assert.Equal(t, "a", items[2].key)
	})
}
