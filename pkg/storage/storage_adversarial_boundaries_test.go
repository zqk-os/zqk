package storage

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

func setupCycle30Crit3Storage(t *testing.T) (*FileObjectStorage, string) {
	t.Helper()
	tmpDir := t.TempDir()
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	fs, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, fs)

	t.Cleanup(func() {
		errShutdown := fs.Shutdown(context.Background())
		assert.NoError(t, errShutdown)
		opts := TempProjectTeardown(tmpDir, fs)
		errTeardown := RunProjectTestTeardown(opts)
		assert.NoError(t, errTeardown)
	})

	return fs, tmpDir
}

func sampleCycle30Crit3BacklogItem(id, title string) map[string]any {
	return map[string]any{
		objects.FieldKeyID:          id,
		objects.FieldKeyKind:        "backlog_item",
		objects.FieldKeyTitle:       title,
		objects.FieldKeyDescription: "Negative boundary validation payload",
		objects.FieldKeyStatus:      "originated",
	}
}

func TestCycle30_AdversarialBoundaries_CorruptedCASBlobPayloads(t *testing.T) {
	fs, tmpDir := setupCycle30Crit3Storage(t)
	ctx := context.Background()

	realData := []byte("title: Valid CAS Content\nkind: backlog_item\n")
	hasher := sha256.New()
	_, errHash := hasher.Write(realData)
	require.NoError(t, errHash)
	validHash := hex.EncodeToString(hasher.Sum(nil))

	validFilePath := filepath.Join(tmpDir, validHash+".yaml")
	errValid := fs.VerifyOrReconcileCASHash(ctx, "backlog_item", "BLI-CAS-001", validFilePath, realData)
	require.NoError(t, errValid)

	corruptedData := []byte("title: Tampered Payload\nkind: backlog_item\n")
	errCorrupted := fs.VerifyOrReconcileCASHash(ctx, "backlog_item", "BLI-CAS-001", validFilePath, corruptedData)
	require.Error(t, errCorrupted)
	assert.Contains(t, errCorrupted.Error(), "CAS hash mismatch")

	cachedBlob, okCached := filecas.LookupCASBlob("invalid-non-hex-hash-12345")
	assert.False(t, okCached)
	assert.Nil(t, cachedBlob)

	dummyHash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	filecas.StoreCASBlob(dummyHash, realData)
	retrievedBlob, okStored := filecas.LookupCASBlob(dummyHash)
	assert.True(t, okStored)
	assert.Equal(t, realData, retrievedBlob)
}

func TestCycle30_AdversarialBoundaries_TruncatedEntriesAndChecksumMismatches(t *testing.T) {
	truncatedYAML := []byte("title: \"unterminated string literal\nkind: backlog_item\n")
	_, errTrunc := unmarshalAndVerifyYAMLObject(truncatedYAML)
	require.Error(t, errTrunc)

	invalidControlChars := []byte(":\n  - : invalid: [unclosed")
	_, errInvalid := unmarshalAndVerifyYAMLObject(invalidControlChars)
	require.Error(t, errInvalid)

	bodyWithFakeChecksum := []byte("title: Checksum Item\nsha256_checksum: 0000000000000000000000000000000000000000000000000000000000000000\n")
	objMismatch := map[string]any{
		"title":           "Checksum Item",
		"sha256_checksum": "0000000000000000000000000000000000000000000000000000000000000000",
	}
	errChecksum := crud.VerifyEmbeddedChecksum(bodyWithFakeChecksum, objMismatch)
	require.Error(t, errChecksum)
	assert.Contains(t, errChecksum.Error(), "integrity verification failed")

	errNilObj := crud.VerifyEmbeddedChecksum(bodyWithFakeChecksum, nil)
	assert.NoError(t, errNilObj)
}

func TestCycle30_AdversarialBoundaries_InvalidObjectIDsAndSchemaRejection(t *testing.T) {
	fs, _ := setupCycle30Crit3Storage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	wrongPrefixObj := sampleCycle30Crit3BacklogItem("WRONGPREFIX-001", "Invalid ID Prefix")
	errBadID := fs.Create(ctx, secCtx, wrongPrefixObj)
	require.Error(t, errBadID)

	missingKindObj := map[string]any{
		objects.FieldKeyTitle: "Missing Kind Field",
	}
	errMissingKind := fs.Create(ctx, secCtx, missingKindObj)
	require.Error(t, errMissingKind)

	emptyTitleObj := sampleCycle30Crit3BacklogItem("BLI-EMPTYTITLE-001", "   ")
	errEmptyTitle := fs.Create(ctx, secCtx, emptyTitleObj)
	require.Error(t, errEmptyTitle)
	assert.Contains(t, errEmptyTitle.Error(), "invalid object title")

	multilineTitleObj := sampleCycle30Crit3BacklogItem("BLI-MULTILINE-001", "Line One\nLine Two")
	errMultiline := fs.Create(ctx, secCtx, multilineTitleObj)
	require.Error(t, errMultiline)
	assert.Contains(t, errMultiline.Error(), "invalid object title")
}

func TestCycle30_AdversarialBoundaries_WALTruncatedAndCorruptedJSONLines(t *testing.T) {
	truncatedLine := []byte(`{"seq": 42, "op": "crea`)
	_, errTrunc := parseWALLine(truncatedLine)
	require.Error(t, errTrunc)

	garbageLine := []byte("NOT_A_VALID_JSON_RECORD")
	_, errGarbage := parseWALLine(garbageLine)
	require.Error(t, errGarbage)

	validLine := []byte(`{"seq": 50, "op": "create", "kind": "backlog_item", "id": "BLI-WAL-001"}`)
	records, errValid := parseWALLine(validLine)
	require.NoError(t, errValid)
	assert.Len(t, records, 1)
	assert.Equal(t, int64(50), records[0].Seq)

	maxSeq := getMaxSeqFromLine(validLine)
	assert.Equal(t, int64(50), maxSeq)

	badMaxSeq := getMaxSeqFromLine([]byte(`{"seq": "invalid"}`))
	assert.Equal(t, int64(0), badMaxSeq)
}

func TestCycle30_AdversarialBoundaries_UncommittedTxAbort_ZeroResidualDiskArtifacts(t *testing.T) {
	fs, _ := setupCycle30Crit3Storage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	tx, errTx := fs.BeginTransaction(ctx)
	require.NoError(t, errTx)
	require.NotNil(t, tx)

	obj := sampleCycle30Crit3BacklogItem("BLI-ABORT-001", "Uncommitted Test Item")
	errQueue := tx.Create(ctx, secCtx, obj)
	require.NoError(t, errQueue)

	_, errBeforeCommit := fs.Read(ctx, secCtx, "BLI-ABORT-001")
	require.Error(t, errBeforeCommit)

	errRollback := tx.Rollback(ctx)
	require.NoError(t, errRollback)

	_, errAfterRollback := fs.Read(ctx, secCtx, "BLI-ABORT-001")
	require.Error(t, errAfterRollback)

	errPostOp := tx.Create(ctx, secCtx, obj)
	require.Error(t, errPostOp)
	assert.Contains(t, errPostOp.Error(), ConstStreamTransactionAlreadyCommittedOrRolledBack)
}

func TestCycle30_AdversarialBoundaries_DoubleRollbackAndCommittedRollbackFailClosed(t *testing.T) {
	fs, tmpDir := setupCycle30Crit3Storage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	tx1, errTx1 := fs.BeginTransaction(ctx)
	require.NoError(t, errTx1)
	obj1 := sampleCycle30Crit3BacklogItem("BLI-COMMIT-001", "Committed Item")
	errCreate1 := tx1.Create(ctx, secCtx, obj1)
	require.NoError(t, errCreate1)
	errCommit1 := tx1.Commit(ctx)
	require.NoError(t, errCommit1)

	errRollbackCommitted := tx1.Rollback(ctx)
	require.Error(t, errRollbackCommitted)
	assert.Contains(t, errRollbackCommitted.Error(), ConstStreamTransactionAlreadyCommitted)

	tx2, errTx2 := fs.BeginTransaction(ctx)
	require.NoError(t, errTx2)
	errFirstRollback := tx2.Rollback(ctx)
	require.NoError(t, errFirstRollback)
	errSecondRollback := tx2.Rollback(ctx)
	require.NoError(t, errSecondRollback)

	adapter := NewFileSystemAdapter(tmpDir, fs)
	txAdapter := adapter.BeginTransaction(ctx)
	errAdapterRollback1 := txAdapter.Rollback(ctx)
	require.NoError(t, errAdapterRollback1)
	errAdapterRollback2 := txAdapter.Rollback(ctx)
	require.NoError(t, errAdapterRollback2)

	errCommitRolledBack := txAdapter.Commit(ctx, secCtx)
	require.Error(t, errCommitRolledBack)
	assert.Contains(t, errCommitRolledBack.Error(), ConstMiscTransactionWasRolledBack)
}

func TestCycle30_AdversarialBoundaries_ConcurrentTxAbortsRace(t *testing.T) {
	fs, _ := setupCycle30Crit3Storage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	tx, errTx := fs.BeginTransaction(ctx)
	require.NoError(t, errTx)
	fileTx, okFileTx := tx.(*FileObjectTransaction)
	require.True(t, okFileTx)

	obj := sampleCycle30Crit3BacklogItem("BLI-RACE-001", "Concurrent Abort Item")
	errCreate := tx.Create(ctx, secCtx, obj)
	require.NoError(t, errCreate)

	concurrencyCount := 8
	var wg sync.WaitGroup
	wg.Add(concurrencyCount)

	for i := 0; i < concurrencyCount; i++ {
		goroutinelabels.NewGoroutine("storage_test", "concurrent transaction abort race").StartSimple(func() {
			defer wg.Done()
			errAbort := tx.Rollback(ctx)
			assert.NoError(t, errAbort)
		})
	}

	wg.Wait()
	assert.True(t, fileTx.rolledBack)
	assert.Nil(t, fileTx.ops)
}

func TestCycle30_AdversarialBoundaries_PartialWriteFailure_AutomaticWALRollback(t *testing.T) {
	fs, _ := setupCycle30Crit3Storage(t)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	originalObj := sampleCycle30Crit3BacklogItem("BLI-RESTORE-001", "Original Pre-Tx Title")
	errInit := fs.Create(ctx, secCtx, originalObj)
	require.NoError(t, errInit)

	tx, errTx := fs.BeginTransaction(ctx)
	require.NoError(t, errTx)
	fileTx, okFileTx := tx.(*FileObjectTransaction)
	require.True(t, okFileTx)

	errUpdate := tx.Update(ctx, secCtx, "BLI-RESTORE-001", map[string]any{
		objects.FieldKeyTitle: "Mutated Title Before Crash",
	})
	require.NoError(t, errUpdate)

	fileTx.ops = append(fileTx.ops, fileTransactionOp{
		opType: OpCreate,
		id:     "BLI-FAIL-001",
		kind:   "nonexistent_invalid_kind_for_rollback_test",
		obj: map[string]any{
			objects.FieldKeyID:   "BLI-FAIL-001",
			objects.FieldKeyKind: "nonexistent_invalid_kind_for_rollback_test",
		},
	})

	_, errApply := fileTx.commitStageApplyPerOpMixed(ctx, secCtx)
	require.Error(t, errApply)

	restoredObj, errRead := fs.Read(ctx, secCtx, "BLI-RESTORE-001")
	require.NoError(t, errRead)
	assert.Equal(t, "Original Pre-Tx Title", restoredObj[objects.FieldKeyTitle])

	_, errPhantom := fs.Read(ctx, secCtx, "BLI-FAIL-001")
	require.Error(t, errPhantom)
}

func TestCycle30_AdversarialBoundaries_CanceledContextGracefulFailure(t *testing.T) {
	fs, _ := setupCycle30Crit3Storage(t)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	secCtx := pkgctx.NewSystemSecurityContext()
	obj := sampleCycle30Crit3BacklogItem("BLI-CANCEL-001", "Canceled Context Item")

	errRetry := ExecuteSimpleRetry(canceledCtx, &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      50 * time.Millisecond,
		BackoffFactor: 2.0,
	}, func() error {
		return fs.Create(canceledCtx, secCtx, obj)
	}, isRetryableError)
	require.Error(t, errRetry)
	assert.True(t, errors.Is(errRetry, context.Canceled))

	_, errTimeout := ExecuteWithConfig[int](&OperationConfig{
		Timeout: 1 * time.Nanosecond,
		RetryConfig: &RetryConfig{
			MaxAttempts: 1,
		},
	}, func(ctx context.Context) (int, error) {
		time.Sleep(5 * time.Millisecond)
		return 42, ctx.Err()
	})
	require.Error(t, errTimeout)
	assert.True(t, errors.Is(errTimeout, context.DeadlineExceeded) || strings.Contains(errTimeout.Error(), "context deadline exceeded"))

	listRes, errList := fs.List(canceledCtx, secCtx, nil, ListFilter{Kind: "backlog_item"})
	assert.True(t, errList == nil || errors.Is(errList, context.Canceled))
	assert.True(t, listRes == nil || len(listRes.Objects) == 0)
}

func TestCycle30_AdversarialBoundaries_ContendedLockTimeoutAndDeadlockPrevention(t *testing.T) {
	tmpDir := t.TempDir()
	coordA := NewFileTransactionCoordinator(tmpDir, NewAutoCleanupStrategy())
	coordB := NewFileTransactionCoordinator(tmpDir, NewAutoCleanupStrategy())

	file1 := filepath.Join(tmpDir, "adversarial_file_1.yaml")
	file2 := filepath.Join(tmpDir, "adversarial_file_2.yaml")
	ctx := context.Background()

	errLockA := coordA.AcquireLocks(ctx, []string{file2, file1}, 2*time.Second)
	require.NoError(t, errLockA)
	assert.True(t, coordA.IsLocked(file1))
	assert.True(t, coordA.IsLocked(file2))

	errLockB := coordB.AcquireLocks(ctx, []string{file1, file2}, 50*time.Millisecond)
	require.Error(t, errLockB)
	assert.Empty(t, coordB.GetLockedFiles())

	errReleaseA := coordA.ReleaseLocks(ctx)
	require.NoError(t, errReleaseA)
	assert.False(t, coordA.IsLocked(file1))

	errLockBRetry := coordB.AcquireLocks(ctx, []string{file1, file2}, 1*time.Second)
	require.NoError(t, errLockBRetry)
	assert.True(t, coordB.IsLocked(file1))

	errReleaseB := coordB.ReleaseLocks(ctx)
	require.NoError(t, errReleaseB)
}

func TestCycle30_AdversarialBoundaries_WALReplayFailureAndRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	walObj, errWAL := AcquireObjectWAL(tmpDir)
	require.NoError(t, errWAL)
	require.NotNil(t, walObj)

	rec1 := &WALRecord{
		Kind:    "backlog_item",
		ID:      "BLI-WAL-REC-001",
		Op:      "create",
		DataB64: base64.StdEncoding.EncodeToString([]byte("title: Rec 1\n")),
	}
	errAppend := walObj.AppendWithSync(rec1)
	require.NoError(t, errAppend)

	errSimulated := errors.New("simulated unrecoverable parse failure during replay")
	_, _, errReplay := ReplayWALChunk(tmpDir, 0, 10, func(rec *WALRecord) error {
		return errSimulated
	})
	require.Error(t, errReplay)
	assert.True(t, errors.Is(errReplay, errSimulated))

	errRelease := ReleaseObjectWAL(tmpDir, walObj)
	require.NoError(t, errRelease)
}
