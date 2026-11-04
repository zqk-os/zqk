package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupCycle12TestStorage(t *testing.T) (*FileObjectStorage, string) {
	t.Helper()
	tmpDir := t.TempDir()
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	fs, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, fs)

	t.Cleanup(func() {
		_ = fs.Shutdown(context.Background())
		opts := TempProjectTeardown(tmpDir, fs)
		_ = RunProjectTestTeardown(opts)
	})

	return fs, tmpDir
}

func sampleCycle12MaturationReport(id, title string, score float64) map[string]any {
	return map[string]any{
		objects.FieldKeyID:                  id,
		objects.FieldKeyKind:                "maturation_report",
		objects.FieldKeyTitle:               title,
		objects.FieldKeyDescription:         "Maturation report description for cycle 12 verification",
		objects.FieldKeyFitnessScore:        score,
		objects.FieldKeyComponentID:         "COMP-CYCLE12",
		objects.FieldKeyObservationDuration: "14d",
		objects.FieldKeyGraduationStatus:    objects.ObjectStatusPromoted,
	}
}

func TestCycle12_FileObjectTransaction_Lifecycle_And_Guards(t *testing.T) {
	fs, _ := setupCycle12TestStorage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	tx, err := fs.BeginTransaction(ctx)
	require.NoError(t, err)
	require.NotNil(t, tx)

	fileTx, ok := tx.(*FileObjectTransaction)
	require.True(t, ok)

	// Guard: Create with empty Kind or ID
	err = tx.Create(ctx, secCtx, map[string]any{objects.FieldKeyKind: "", objects.FieldKeyID: "MAT-1001"})
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamObjectMustHaveKindAndId)

	err = tx.Create(ctx, secCtx, map[string]any{objects.FieldKeyKind: "maturation_report", objects.FieldKeyID: ""})
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamObjectMustHaveKindAndId)

	// Guard: Update non-existent object fails during readKind
	err = tx.Update(ctx, secCtx, "MAT-NONEXISTENT", map[string]any{"title": "Fail"})
	require.Error(t, err)

	// Guard: Delete non-existent object fails during readKind
	err = tx.Delete(ctx, secCtx, "MAT-NONEXISTENT", false)
	require.Error(t, err)

	// Guard: DeleteByIDAndKind with empty parameters
	err = fileTx.DeleteByIDAndKind(ctx, "", "maturation_report")
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamIdAndKindRequiredForDeletebyidandkind)

	err = fileTx.DeleteByIDAndKind(ctx, "MAT-1001", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamIdAndKindRequiredForDeletebyidandkind)

	// Valid Create queueing
	obj1 := sampleCycle12MaturationReport("MAT-10001", "Report 1", 75.0)
	err = tx.Create(ctx, secCtx, obj1)
	require.NoError(t, err)

	// Read delegates to storage
	_, err = tx.Read(ctx, secCtx, "MAT-10001")
	require.Error(t, err) // Not committed yet

	// Rollback active transaction
	err = tx.Rollback(ctx)
	require.NoError(t, err)
	require.True(t, fileTx.rolledBack)

	// EnsureActive guards after rollback
	err = tx.Create(ctx, secCtx, obj1)
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamTransactionAlreadyCommittedOrRolledBack)

	err = tx.Update(ctx, secCtx, "MAT-10001", map[string]any{"title": "New"})
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamTransactionAlreadyCommittedOrRolledBack)

	err = tx.Delete(ctx, secCtx, "MAT-10001", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamTransactionAlreadyCommittedOrRolledBack)

	err = fileTx.DeleteByIDAndKind(ctx, "MAT-10001", "maturation_report")
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamTransactionAlreadyCommittedOrRolledBack)

	// Commit after rollback must fail
	err = tx.Commit(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamTransactionWasRolledBack)

	// Secondary rollback is safe
	err = tx.Rollback(ctx)
	require.NoError(t, err)
}

func TestCycle12_FileObjectTransaction_AtomicRollback_OnFailure(t *testing.T) {
	fs, _ := setupCycle12TestStorage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Pre-create an object
	objA := sampleCycle12MaturationReport("MAT-20001", "Original Title A", 80.0)
	err := fs.Create(ctx, secCtx, objA)
	require.NoError(t, err)

	// 2. Begin transaction
	tx, err := fs.BeginTransaction(ctx)
	require.NoError(t, err)
	fileTx := tx.(*FileObjectTransaction)

	// Queue update for objA
	err = tx.Update(ctx, secCtx, "MAT-20001", map[string]any{objects.FieldKeyTitle: "Mutated Title A"})
	require.NoError(t, err)

	// Queue create for objB
	objB := sampleCycle12MaturationReport("MAT-20002", "Created Title B", 90.0)
	err = tx.Create(ctx, secCtx, objB)
	require.NoError(t, err)

	// Queue a failing op that causes mixed execution to fail during Create
	fileTx.ops = append(fileTx.ops, fileTransactionOp{
		opType: OpCreate,
		id:     "MAT-FAIL-001",
		kind:   "unregistered_invalid_kind_fails_create",
		obj: map[string]any{
			objects.FieldKeyID:   "MAT-FAIL-001",
			objects.FieldKeyKind: "unregistered_invalid_kind_fails_create",
		},
	})

	// 3. Attempt to apply through commitStageApplyPerOpMixed - should fail and trigger atomic rollback
	_, applyErr := fileTx.commitStageApplyPerOpMixed(ctx, secCtx)
	require.Error(t, applyErr)

	// Verify objA was restored to original state
	restoredA, err := fs.Read(ctx, secCtx, "MAT-20001")
	require.NoError(t, err)
	require.Equal(t, "Original Title A", restoredA[objects.FieldKeyTitle])

	// Verify objB was cleaned up
	_, err = fs.Read(ctx, secCtx, "MAT-20002")
	require.Error(t, err)

	// Verify helpers
	require.False(t, allTransactionOpsAreDeletes(fileTx.ops))
	require.False(t, allTransactionOpsAreDeletes(nil))
	require.True(t, allTransactionOpsAreDeletes([]fileTransactionOp{
		{opType: OpDelete, id: "MAT-1"},
		{opType: OpDelete, id: "MAT-2"},
	}))
}

func TestCycle12_FileObjectTransaction_BatchDeletes_And_PurgeDraft(t *testing.T) {
	fs, _ := setupCycle12TestStorage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Pre-create 3 objects
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("MAT-3000%d", i)
		obj := sampleCycle12MaturationReport(id, fmt.Sprintf("Report %d", i), 85.0)
		err := fs.Create(ctx, secCtx, obj)
		require.NoError(t, err)
	}

	// Begin transaction and queue deletes using DeleteByIDAndKind
	tx, err := fs.BeginTransaction(ctx)
	require.NoError(t, err)
	fileTx := tx.(*FileObjectTransaction)

	// Invariant proof: hard delete is refused on core kind without test permission
	err = fileTx.DeleteByIDAndKind(ctx, "MAT-30001", "maturation_report")
	require.Error(t, err)
	require.Contains(t, err.Error(), "hard delete refused")

	// With test hard delete enabled in context, queuing succeeds
	testCtx := WithTestHardDelete(ctx)
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("MAT-3000%d", i)
		err = fileTx.DeleteByIDAndKind(testCtx, id, "maturation_report")
		require.NoError(t, err)
	}

	require.True(t, allTransactionOpsAreDeletes(fileTx.ops))

	// Commit batch deletes
	err = tx.Commit(testCtx)
	require.NoError(t, err)
	require.True(t, fileTx.committed)

	// Attempting commit again returns error
	err = tx.Commit(testCtx)
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstStreamTransactionAlreadyCommitted)

	// Verify all 3 objects deleted
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("MAT-3000%d", i)
		_, err = fs.Read(ctx, secCtx, id)
		require.Error(t, err)
	}

	// Test purgeDraftPlaneDeletes handles nil storage safely
	fileTx.purgeDraftPlaneDeletes(nil)
	fileTx.purgeDraftPlaneDeletes(fs)
}

func TestCycle12_FileObjectTransaction_WriteBehindBatchWAL_EdgeCases(t *testing.T) {
	fs, _ := setupCycle12TestStorage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	tx := &FileObjectTransaction{
		storage: fs,
		ctx:     ctx,
		ops:     nil,
	}

	// Case 1: Empty ops -> done=false
	done, err := tx.commitStageWriteBehindBatchWAL(ctx, secCtx, fs, false)
	require.False(t, done)
	require.NoError(t, err)

	// Case 2: allDeletes == true -> done=false
	tx.ops = []fileTransactionOp{{opType: OpDelete, id: "MAT-40001", kind: "maturation_report"}}
	done, err = tx.commitStageWriteBehindBatchWAL(ctx, secCtx, fs, true)
	require.False(t, done)
	require.NoError(t, err)

	// Case 3: nil storage WAL or writeBuf -> done=false
	nilWALStorage := &FileObjectStorage{}
	done, err = tx.commitStageWriteBehindBatchWAL(ctx, secCtx, nilWALStorage, false)
	require.False(t, done)
	require.NoError(t, err)

	// Finalize with errors logs without panic
	tx.commitStageFinalize(nil)
	require.True(t, tx.committed)
}

func TestCycle12_FileTransactionCoordinator_TwoPhaseLocking(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Initial coordinator with default auto-cleanup strategy
	coord := NewFileTransactionCoordinator(tmpDir, nil)
	require.NotNil(t, coord)
	require.NotNil(t, coord.lockStrategy)

	ctx := context.Background()
	fileA := filepath.Join(tmpDir, "fileA.yaml")
	fileB := filepath.Join(tmpDir, "fileB.yaml")
	fileC := filepath.Join(tmpDir, "fileC.yaml")

	// 2. Acquire locks in unordered list (coordinator sorts internally)
	pathsToLock := []string{fileC, fileA, fileB}
	err := coord.AcquireLocks(ctx, pathsToLock, 2*time.Second)
	require.NoError(t, err)

	// 3. Verify locks are held
	require.True(t, coord.IsLocked(fileA))
	require.True(t, coord.IsLocked(fileB))
	require.True(t, coord.IsLocked(fileC))
	require.False(t, coord.IsLocked(filepath.Join(tmpDir, "unlocked.yaml")))

	lockedFiles := coord.GetLockedFiles()
	require.Len(t, lockedFiles, 3)
	require.Contains(t, lockedFiles, fileA)
	require.Contains(t, lockedFiles, fileB)
	require.Contains(t, lockedFiles, fileC)

	// 4. Re-acquiring already held locks is idempotent and non-blocking
	err = coord.AcquireLocks(ctx, []string{fileA}, 1*time.Second)
	require.NoError(t, err)

	// 5. Test lock contention from a separate coordinator:
	coord2 := NewFileTransactionCoordinator(tmpDir, NewAutoCleanupStrategy())
	// Try to acquire fileA which is already held -> should fail fast with timeout
	errContention := coord2.AcquireLocks(ctx, []string{fileA}, 50*time.Millisecond)
	require.Error(t, errContention)

	// 6. Release specific locks
	coord.releaseLocks([]string{fileB})
	require.False(t, coord.IsLocked(fileB))
	require.True(t, coord.IsLocked(fileA))

	// 7. Release all remaining locks
	err = coord.ReleaseLocks(ctx)
	require.NoError(t, err)
	require.False(t, coord.IsLocked(fileA))
	require.Empty(t, coord.GetLockedFiles())
}

func TestCycle12_FileSystemAdapter_TransactionCommit_And_Rollback(t *testing.T) {
	fs, tmpDir := setupCycle12TestStorage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	adapter := NewFileSystemAdapter(tmpDir, fs)
	require.NotNil(t, adapter)
	require.NotNil(t, adapter.coordinator)

	// 1. Begin enhanced transaction
	tx := adapter.BeginTransaction(ctx)
	require.NotNil(t, tx)

	// AddFile deduplication
	fileX := filepath.Join(tmpDir, "x.yaml")
	tx.AddFile(fileX)
	tx.AddFile(fileX)
	require.Len(t, tx.filePaths, 1)

	// Guard: Create with missing Kind or ID
	err := tx.Create(ctx, secCtx, map[string]any{objects.FieldKeyKind: ""})
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstMiscObjectMustHaveKindAndId)

	// Guard: Create on non-existent unindexed object path fails path resolution
	err = tx.Create(ctx, secCtx, sampleCycle12MaturationReport("MAT-50002", "Unindexed Path", 80.0))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to determine file path")

	// 2. Pre-create objects via storage
	obj1 := sampleCycle12MaturationReport("MAT-50001", "Pre-existing 1", 70.0)
	err = fs.Create(ctx, secCtx, obj1)
	require.NoError(t, err)

	obj2 := sampleCycle12MaturationReport("MAT-50002", "Pre-existing 2", 82.0)
	err = fs.Create(ctx, secCtx, obj2)
	require.NoError(t, err)

	// Queue update for obj1
	err = tx.Update(ctx, secCtx, "MAT-50001", map[string]any{objects.FieldKeyTitle: "Enhanced Update 1"})
	require.NoError(t, err)

	// Queue delete for obj2 with test hard delete allowed
	testCtx := WithTestHardDelete(ctx)
	err = tx.Delete(testCtx, secCtx, "MAT-50002")
	require.NoError(t, err)

	// 3. Commit transaction
	err = tx.Commit(testCtx, secCtx)
	require.NoError(t, err)
	require.True(t, tx.committed)

	// Verify obj1 updated
	read1, err := fs.Read(ctx, secCtx, "MAT-50001")
	require.NoError(t, err)
	require.Equal(t, "Enhanced Update 1", read1[objects.FieldKeyTitle])

	// Verify obj2 deleted
	_, err = fs.Read(ctx, secCtx, "MAT-50002")
	require.Error(t, err)

	// Guard: Commit on already committed tx
	err = tx.Commit(ctx, secCtx)
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstMiscTransactionAlreadyCommitted)

	// Guard: Rollback on already committed tx
	err = tx.Rollback(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstMiscTransactionAlreadyCommitted)

	// 4. Test Rollback mechanics on a fresh transaction
	tx2 := adapter.BeginTransaction(ctx)
	err = tx2.Update(ctx, secCtx, "MAT-50001", map[string]any{objects.FieldKeyTitle: "Will Rollback"})
	require.NoError(t, err)

	err = tx2.Rollback(ctx)
	require.NoError(t, err)
	require.True(t, tx2.rolledBack)
	require.Empty(t, tx2.operations)
	require.Empty(t, tx2.filePaths)

	// Commit on rolled back tx
	err = tx2.Commit(ctx, secCtx)
	require.Error(t, err)
	require.Contains(t, err.Error(), ConstMiscTransactionWasRolledBack)

	// Also verify FileObjectStorage.BeginEnhancedTransaction
	enhTx, err := fs.BeginEnhancedTransaction(ctx)
	require.NoError(t, err)
	require.NotNil(t, enhTx)
}

func TestCycle12_CAS_VerifyOrReconcileHash_And_Corruption(t *testing.T) {
	fs, tmpDir := setupCycle12TestStorage(t)
	ctx := context.Background()

	data := []byte("title: Verification Content\nstatus: active\n")
	hasher := sha256.New()
	hasher.Write(data)
	correctHash := hex.EncodeToString(hasher.Sum(nil))

	// 1. Hash-named file matching hash
	hashFilePath := filepath.Join(tmpDir, correctHash+".yaml")
	err := fs.VerifyOrReconcileCASHash(ctx, "maturation_report", "MAT-60001", hashFilePath, data)
	require.NoError(t, err)

	// 2. Hash-named file mismatch
	corruptHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	corruptFilePath := filepath.Join(tmpDir, corruptHash+".yaml")
	err = fs.VerifyOrReconcileCASHash(ctx, "maturation_report", "MAT-60001", corruptFilePath, data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "CAS hash mismatch")

	// 3. ID-based filename (reconciles with registry)
	idFilePath := filepath.Join(tmpDir, "MAT-60001.yaml")
	err = fs.VerifyOrReconcileCASHash(ctx, "maturation_report", "MAT-60001", idFilePath, data)
	require.NoError(t, err)

	// 4. Cache manipulation and CAS instance retrieval
	fs.ClearCASCaches()
	fs.InvalidateCASCacheForKind("maturation_report")

	casInst, err := fs.GetContentAddressableStorage("maturation_report")
	require.NoError(t, err)
	require.NotNil(t, casInst)
}

func TestCycle12_ScanIDBasedFiles_And_DiscoverKinds(t *testing.T) {
	fs, _ := setupCycle12TestStorage(t)

	kindDir := fs.GetKindDir("maturation_report")
	err := fileutil.MkdirAll(kindDir, paths.FilePerm755)
	require.NoError(t, err)

	// Write ID-based file
	idFile := filepath.Join(kindDir, "MAT-70001.yaml")
	err = fileutil.WriteFile(idFile, []byte("id: MAT-70001\n"), paths.FilePerm644)
	require.NoError(t, err)

	// Write hash-based file (should be skipped by scan)
	hashFile := filepath.Join(kindDir, "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef.yaml")
	err = fileutil.WriteFile(hashFile, []byte("id: HASH-1\n"), paths.FilePerm644)
	require.NoError(t, err)

	// Write dotfile (should be skipped)
	dotFile := filepath.Join(kindDir, ".index.lock")
	err = fileutil.WriteFile(dotFile, []byte(""), paths.FilePerm644)
	require.NoError(t, err)

	// Test ScanIDBasedFilesWithPaths
	results := fs.ScanIDBasedFilesWithPaths(kindDir, "maturation_report")
	require.Contains(t, results, "MAT-70001")
	require.NotContains(t, results, "HASH-1")
	require.NotContains(t, results, ".index.lock")

	// Test ScanIDBasedFilesRecursive (uses cache)
	recursiveResults := fs.ScanIDBasedFilesRecursive(kindDir, "maturation_report")
	require.Contains(t, recursiveResults, "MAT-70001")

	// Test DiscoverObjectKinds
	kinds := fs.DiscoverObjectKinds()
	require.NotEmpty(t, kinds)
}

func TestCycle12_FileObjectStorage_ConcurrentReadsWrites(t *testing.T) {
	fs, _ := setupCycle12TestStorage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	concurrencyCount := 10
	var wg sync.WaitGroup
	wg.Add(concurrencyCount)

	for i := 0; i < concurrencyCount; i++ {
		idx := i
		goroutinelabels.NewGoroutine("storage_test", "concurrent reads and writes").StartSimple(func() {
			defer wg.Done()
			id := fmt.Sprintf("MAT-800%02d", idx)
			obj := sampleCycle12MaturationReport(id, fmt.Sprintf("Concurrent Report %d", idx), float64(70+idx))

			// Write
			err := fs.Create(ctx, secCtx, obj)
			assert.NoError(t, err)

			// Read
			readObj, err := fs.Read(ctx, secCtx, id)
			assert.NoError(t, err)
			assert.NotNil(t, readObj)
			assert.Equal(t, fmt.Sprintf("Concurrent Report %d", idx), readObj[objects.FieldKeyTitle])
		})
	}

	wg.Wait()
}

func TestCycle12_FileObjectStorage_UpdatePrepare_ErrorPaths(t *testing.T) {
	fs, _ := setupCycle12TestStorage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Error path 1: Cannot infer kind from unrecognized ID
	_, err := fs.prepareFileObjectUpdate(ctx, secCtx, "UNRECOGNIZED-99999", map[string]any{"title": "Fail"})
	require.Error(t, err)

	// Error path 2: Target object does not exist
	_, err = fs.prepareFileObjectUpdate(ctx, secCtx, "MAT-99999", map[string]any{"title": "Fail"})
	require.Error(t, err)

	// Error path 3: Invalid lifecycle status transition
	// Create initial object
	initObj := sampleCycle12MaturationReport("MAT-90001", "Lifecycle Check", 85.0)
	err = fs.Create(ctx, secCtx, initObj)
	require.NoError(t, err)

	// Try invalid lifecycle transition if restricted
	_, err = fs.prepareFileObjectUpdate(ctx, secCtx, "MAT-90001", map[string]any{
		objects.FieldKeyStatus: "invalid_nonexistent_status_xyz",
	})
	require.Error(t, err)
}
