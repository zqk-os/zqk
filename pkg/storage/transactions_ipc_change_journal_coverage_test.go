package storage

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// mockPrivilegedWriter implements PrivilegedWriter for testing IPC
type mockPrivilegedWriter struct {
	writes  []string
	deletes []string
	renames []string
}

func (m *mockPrivilegedWriter) WriteObject(ctx context.Context, id, kind string, payload []byte, isDraft bool) error {
	m.writes = append(m.writes, id)
	return nil
}

func (m *mockPrivilegedWriter) DeleteObject(ctx context.Context, id, kind string) error {
	m.deletes = append(m.deletes, id)
	return nil
}

func (m *mockPrivilegedWriter) RenameObject(ctx context.Context, oldID, newID, kind string) error {
	m.renames = append(m.renames, oldID+"->"+newID)
	return nil
}

func TestStorageExtended_TransactionsAndAdapters(t *testing.T) {
	ctx := context.Background()

	t.Run("FileSystemAdapter_Lifecycle_and_Operations", func(t *testing.T) {
		testRoot, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

		adapter := NewFileSystemAdapter(testRoot, fos)
		require.NotNil(t, adapter)

		tx := adapter.BeginTransaction(ctx)
		require.NotNil(t, tx)

		// AddFile deduplication
		fPath1 := filepath.Join(testRoot, "file1.yaml")
		tx.AddFile(fPath1)
		tx.AddFile(fPath1)
		assert.Equal(t, 1, len(tx.filePaths))

		// Create validation (empty kind/id error)
		err := tx.Create(ctx, secCtx, map[string]any{"id": "foo"})
		assert.Error(t, err)

		// Pre-create object in storage
		initialObj := map[string]any{
			objects.FieldKeyKind:        "backlog_item",
			objects.FieldKeyID:          "BLI-TX-1",
			"title":                     "Tx Item 1",
			"status":                    "planned",
			objects.FieldKeyDescription: "Tx Item 1 Description",
		}
		require.NoError(t, fos.Create(ctx, secCtx, initialObj))

		// First transaction: Update and Commit
		err = tx.Update(ctx, secCtx, "BLI-TX-1", map[string]any{"title": "Updated Title"})
		assert.NoError(t, err)

		// Commit transaction
		err = tx.Commit(ctx, secCtx)
		assert.NoError(t, err)

		// Operations fail on committed tx
		err = tx.Create(ctx, secCtx, initialObj)
		assert.Error(t, err)
		err = tx.Update(ctx, secCtx, "BLI-TX-1", map[string]any{"title": "Updated Again"})
		assert.Error(t, err)
		err = tx.Delete(ctx, secCtx, "BLI-TX-1")
		assert.Error(t, err)
		err = tx.Commit(ctx, secCtx)
		assert.Error(t, err)
		err = tx.Rollback(ctx)
		assert.Error(t, err)

		// Second transaction with Delete and Rollback
		tx2 := adapter.BeginTransaction(ctx)
		err = tx2.Delete(ctx, secCtx, "BLI-TX-1")
		assert.NoError(t, err)

		// Rollback tx2
		err = tx2.Rollback(ctx)
		assert.NoError(t, err)
		// Operations fail on rolled-back tx
		err = tx2.Commit(ctx, secCtx)
		assert.Error(t, err)
		err = tx2.Create(ctx, secCtx, initialObj)
		assert.Error(t, err)
	})

	t.Run("DSIATransaction_and_Provider", func(t *testing.T) {
		tmpDir := t.TempDir()
		provider := NewDSIAStorageProvider(tmpDir)
		require.NotNil(t, provider)

		// Verify Shutdown is safe
		err := provider.Shutdown(ctx)
		assert.NoError(t, err)

		// BeginTransaction
		txObj, err := provider.BeginTransaction(ctx)
		require.NoError(t, err)
		dsiaTx, ok := txObj.(*DSIATransaction)
		require.True(t, ok)

		// Staged Create
		secCtx := pkgctx.NewSystemSecurityContext()
		err = dsiaTx.Create(ctx, secCtx, map[string]any{
			objects.FieldKeyKind: "backlog_item",
			objects.FieldKeyID:   "DSIA-1",
			"title":              "DSIA Title",
		})
		assert.NoError(t, err)

		// Read from staged
		readStaged, err := dsiaTx.Read(ctx, secCtx, "DSIA-1")
		assert.NoError(t, err)
		assert.Equal(t, "DSIA Title", readStaged["title"])

		// Update staged
		err = dsiaTx.Update(ctx, secCtx, "DSIA-1", map[string]any{
			objects.FieldKeyKind: "backlog_item",
			objects.FieldKeyID:   "DSIA-1",
			"title":              "DSIA Updated Title",
		})
		assert.NoError(t, err)

		// Commit staged
		err = dsiaTx.Commit(ctx)
		assert.NoError(t, err)

		// Read committed from provider
		committedObj, err := provider.Read(ctx, secCtx, "DSIA-1")
		assert.NoError(t, err)
		assert.Equal(t, "DSIA Updated Title", committedObj["title"])

		// Query GetPath & GetNeighbors
		path, err := provider.GetPath(ctx, secCtx, "DSIA-1", "DSIA-1")
		assert.NoError(t, err)
		assert.Equal(t, 2, len(path))

		neighbors, err := provider.GetNeighbors(ctx, secCtx, "DSIA-1", "both")
		assert.NoError(t, err)
		assert.NotNil(t, neighbors)

		// Provider direct Update
		err = provider.Update(ctx, secCtx, "DSIA-1", map[string]any{
			objects.FieldKeyKind: "backlog_item",
			objects.FieldKeyID:   "DSIA-1",
			"title":              "DSIA Direct Update",
		})
		assert.NoError(t, err)

		// Rollback test
		txRoll, err := provider.BeginTransaction(ctx)
		require.NoError(t, err)
		dsiaRoll, _ := txRoll.(*DSIATransaction)
		err = dsiaRoll.Delete(ctx, secCtx, "DSIA-1", false)
		assert.NoError(t, err)
		err = dsiaRoll.Rollback(ctx)
		assert.NoError(t, err)
		// Post-rollback error conditions
		assert.Error(t, dsiaRoll.Create(ctx, secCtx, map[string]any{"id": "X"}))
		assert.Error(t, dsiaRoll.Update(ctx, secCtx, "X", nil))
		assert.Error(t, dsiaRoll.Delete(ctx, secCtx, "X", false))
		assert.Error(t, dsiaRoll.Commit(ctx))
		assert.Error(t, dsiaRoll.Rollback(ctx))
	})
}

func TestStorageExtended_HashRegistryAndQueueShutdown(t *testing.T) {
	ctx := context.Background()

	t.Run("HashRegistryManager_ShutdownHandler", func(t *testing.T) {
		manager := &HashRegistryManager{
			registries: make(map[string]*HashRegistry),
		}
		require.NotNil(t, manager)

		assert.Equal(t, ConstMiscHashRegistryManager, manager.GetName())
		assert.True(t, manager.IsCritical())
		assert.True(t, manager.IsDrained())
		assert.Equal(t, int64(0), manager.GetPendingCount())

		err := manager.InitiateShutdown()
		assert.NoError(t, err)

		err = manager.Drain(ctx)
		assert.NoError(t, err)

		// ValidateRegistryLocation & GetRegistryDirectoryForFile
		err = ValidateRegistryLocation("/repo/backlog", "", "backlog_item", false)
		assert.NoError(t, err)
		err = ValidateRegistryLocation("/repo/audit/2026-01", "/repo/audit/2026-01/evt.yaml", "audit_event", true)
		assert.NoError(t, err)
		err = ValidateRegistryLocation("/wrong/dir", "/repo/audit/2026-01/evt.yaml", "audit_event", true)
		assert.Error(t, err)
		err = ValidateRegistryLocation("/repo/backlog", "/repo/backlog/item.yaml", "backlog_item", false)
		assert.NoError(t, err)
		err = ValidateRegistryLocation("/wrong/dir", "/repo/backlog/item.yaml", "backlog_item", false)
		assert.Error(t, err)

		dirB := GetRegistryDirectoryForFile("/repo/audit/2026-01/evt.yaml", true)
		assert.Equal(t, "/repo/audit/2026-01", dirB)
		dirNB := GetRegistryDirectoryForFile("/repo/backlog/item.yaml", false)
		assert.Equal(t, "/repo/backlog", dirNB)
	})

	t.Run("HashRegistry_HasHash_and_GetAllHashes", func(t *testing.T) {
		tmpDir := t.TempDir()
		hr := NewHashRegistry(ctx, "backlog_item", tmpDir)
		require.NotNil(t, hr)

		assert.False(t, hr.HasHash("item.yaml"))
		hr.SetHash("item.yaml", "sha256:abc123")
		assert.True(t, hr.HasHash("item.yaml"))

		hashes := hr.GetAllHashes()
		assert.Equal(t, "sha256:abc123", hashes["item.yaml"])

		assert.True(t, hr.IsCritical())
		assert.Equal(t, "hash_registry_backlog_item", hr.GetName())
		assert.True(t, hr.IsDrained())
		assert.Equal(t, int64(0), hr.GetPendingCount())

		// SaveAsync & SaveWithContext
		hr.SaveAsync()
		err := hr.SaveWithContext(ctx)
		assert.NoError(t, err)

		err = hr.InitiateShutdown()
		assert.NoError(t, err)
		err = hr.Drain(ctx)
		assert.NoError(t, err)
	})

	t.Run("IOQueueManager_Shutdown_and_SetConfig", func(t *testing.T) {
		// Create an isolated IOQueueManager so global shutdown isn't affected
		mgr := &IOQueueManager{
			queues: make([]*ioQueue, 0),
			config: DefaultIOQueueConfig(),
			ctx:    ctx,
		}
		cfg := DefaultIOQueueConfig()
		cfg.MaxQueues = 4
		mgr.SetConfig(cfg)
		assert.Equal(t, ConstMiscIoQueueManager, mgr.GetName())
		assert.True(t, mgr.IsCritical())
		assert.True(t, mgr.IsDrained())
		assert.Equal(t, int64(0), mgr.GetPendingCount())

		// selectQueueGrouped
		q1 := mgr.selectQueueGrouped("group-A")
		assert.NotNil(t, q1)
		q2 := mgr.selectQueueGrouped("group-A")
		assert.Equal(t, q1, q2)

		err := mgr.Drain(ctx)
		assert.NoError(t, err)
	})
}

func TestStorageExtended_IPCServerAndWriter(t *testing.T) {
	ctx := context.Background()
	// macOS UNIX socket path length must be < 104 characters
	socketPath := filepath.Join(os.TempDir(), "pw_test.sock")
	_ = os.Remove(socketPath)
	defer os.Remove(socketPath)

	mockWriter := &mockPrivilegedWriter{}
	daemon := NewPrivilegedWriterDaemon(mockWriter)
	require.NotNil(t, daemon)

	listener, err := StartIPCServer(socketPath, daemon)
	require.NoError(t, err)
	defer listener.Close()

	// Wait for listener to accept connections
	for i := 0; i < 20; i++ {
		conn, dialErr := net.Dial("unix", socketPath)
		if dialErr == nil {
			conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	client, err := NewIPCWriter(socketPath)
	require.NoError(t, err)
	defer client.Close()

	err = client.WriteObject(ctx, "OBJ-1", "backlog_item", []byte("data"), false)
	assert.NoError(t, err)
	assert.Contains(t, mockWriter.writes, "OBJ-1")

	err = client.RenameObject(ctx, "OBJ-1", "OBJ-2", "backlog_item")
	assert.NoError(t, err)
	assert.Contains(t, mockWriter.renames, "OBJ-1->OBJ-2")

	err = client.DeleteObject(ctx, "OBJ-2", "backlog_item")
	assert.NoError(t, err)
	assert.Contains(t, mockWriter.deletes, "OBJ-2")
}

func TestStorageExtended_StorageFileMethods_and_ChangeJournal(t *testing.T) {
	ctx := context.Background()

	t.Run("FileObjectStorage_Helpers", func(t *testing.T) {
		testRoot, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

		// DiscoverObjectKinds
		kinds := fos.DiscoverObjectKinds()
		assert.NotNil(t, kinds)

		// RemoveEmptyBucketDirectories
		removed, err := fos.RemoveEmptyBucketDirectories("audit_event")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, removed, 0)

		// ClearCASCaches & InvalidateCASCacheForKind
		fos.ClearCASCaches()
		fos.InvalidateCASCacheForKind("backlog_item")

		// GetProcessDir & GetLogger & ValidateObject
		pDir := fos.GetProcessDir()
		assert.NotEmpty(t, pDir)
		logger := fos.GetLogger()
		assert.NotNil(t, logger)

		// SetKindProcessorRegistry
		kRegistry := NewKindProcessorRegistry()
		fos.SetKindProcessorRegistry(kRegistry)
		assert.Equal(t, kRegistry, fos.GetKindProcessors())

		// EnsureTestIdentityCacheHandler
		EnsureTestIdentityCacheHandler()

		// SetChangeNotificationHandler
		called := false
		SetChangeNotificationHandler(func(c context.Context, op, k, id string, data map[string]any) error {
			called = true
			return nil
		})
		executeChangeNotification(ctx, "create", "backlog_item", "TEST-HOOK-1", map[string]any{"id": "TEST-HOOK-1"})
		time.Sleep(50 * time.Millisecond)
		assert.True(t, called)
		SetChangeNotificationHandler(nil)

		// EnsureBucketStrategyRegistryReady
		fos.EnsureBucketStrategyRegistryReady(ctx)

		// hasDateSubdirectories & isBucketInTimeRange
		assert.False(t, fos.hasDateSubdirectories(filepath.Join(testRoot, "nonexistent")))
		tr := &timeRange{
			start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC),
		}
		assert.True(t, fos.isBucketInTimeRange("2026-01", tr))
		assert.False(t, fos.isBucketInTimeRange("2025-12", tr))
		assert.True(t, fos.isBucketInTimeRange("categorical", tr))

		// countFiles
		count, err := fos.countFiles(ctx, fos.GetKindDir("backlog_item"), false)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, 0)

		// EmitListCountWaitProgress
		EmitListCountWaitProgress(ctx)

		// UsesContentAddressableStorage & WriteObjectToStorage
		assert.True(t, fos.UsesContentAddressableStorage("backlog_item"))
		bliFile := filepath.Join(testRoot, ".zqk", "process", "backlog_items", "BLI-MIG-1.yaml")
		_ = fileutil.MkdirAll(filepath.Dir(bliFile), 0755)
		err = fos.WriteObjectToStorage(ctx, "BLI-MIG-1", "backlog_item", bliFile, []byte("kind: backlog_item\nid: BLI-MIG-1\nstatus: todo\ntitle: Migrated Item\ndescription: Migrated Item Description\n"), secCtx, false)
		assert.NoError(t, err)

		// PutStreamSegment
		hashes, err := fos.PutStreamSegment("audit_event", "SEG-1", []byte("stream segment test payload"))
		assert.NoError(t, err)
		assert.NotEmpty(t, hashes)
	})

	t.Run("HighVolumeCache_and_Invalidation", func(t *testing.T) {
		workers := getHighVolumeCacheBuildWorkers()
		assert.GreaterOrEqual(t, workers, 4)

		cache := NewHighVolumeEventCache()
		assert.False(t, cache.HasStatus())
		cache.InvalidateForProject("/some/project/root")

		// Invalidation shockwave bus testing using an isolated bus instance
		testBus := NewInvalidationShockwaveBus()
		assert.NotNil(t, testBus)
		testBus.ClearSubscribers()
		assert.Equal(t, 0, testBus.SubscribersCount())

		var received []MutationEvent
		sub := InvalidationSubscriberFunc(func(c context.Context, ev MutationEvent) error {
			received = append(received, ev)
			return nil
		})
		testBus.Subscribe(sub)
		assert.Equal(t, 1, testBus.SubscribersCount())

		testBus.Broadcast(ctx, MutationEvent{Kind: "backlog_item", ID: "BLI-SUB-1", Op: MutationOpPut})
		time.Sleep(20 * time.Millisecond)
		assert.Equal(t, 1, len(received))

		testBus.Unsubscribe(sub)
		assert.Equal(t, 0, testBus.SubscribersCount())

		// Ensure global bus has default parse cache subscriber restored
		globalBus := GetGlobalInvalidationBus()
		assert.NotNil(t, globalBus)
		globalBus.Subscribe(newParseCacheSubscriber(GetGlobalParseCache()))
	})

	t.Run("ChangeJournal_Reconstruction_and_Metrics", func(t *testing.T) {
		// deepCopySlice
		copied := deepCopySlice([]any{"a", map[string]any{"key": "val"}, []any{1, 2}})
		assert.Equal(t, 3, len(copied))
		assert.Nil(t, deepCopySlice(nil))

		// getString
		assert.Equal(t, "hello", getString(map[string]any{"k": "hello"}, "k"))
		assert.Equal(t, "", getString(map[string]any{"k": "hello"}, "nonexistent"))

		// ExtractObjectRef
		k, id, err := ExtractObjectRef("backlog_item:BLI-123")
		assert.NoError(t, err)
		assert.Equal(t, "backlog_item", k)
		assert.Equal(t, "BLI-123", id)

		_, _, err = ExtractObjectRef("invalid-ref")
		assert.Error(t, err)

		// buildAuditEventValidationMetric
		metricsrecording.EnterAllowRecording()
		defer metricsrecording.LeaveAllowRecording()
		bTrue := buildAuditEventValidationMetric(true)
		assert.NotNil(t, bTrue)
		bFalse := buildAuditEventValidationMetric(false)
		assert.NotNil(t, bFalse)
	})
}
