package community

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func newTestCASCompactor(t *testing.T, retainGens uint64) *CASCompactor {
	t.Helper()
	compactor, err := NewCASCompactor(CompactionConfig{
		MinPageSize:       4,
		MaxPageSize:       4096,
		RetainGenerations: retainGens,
	})
	require.NoError(t, err)
	require.NotNil(t, compactor)
	return compactor
}

func makeTestPage(pageID, bucket, objectKey string, gen uint64, payload string) StoragePage {
	return StoragePage{
		PageID:     pageID,
		Bucket:     bucket,
		ObjectKey:  objectKey,
		Generation: gen,
		Payload:    []byte(payload),
	}
}

func assertRetrievedObject(t *testing.T, compactor *CASCompactor, bucket, key, expectedPayload string) {
	t.Helper()
	data, checksum, err := compactor.RetrieveObject(bucket, key)
	require.NoError(t, err)
	assert.Equal(t, []byte(expectedPayload), data)

	sum := sha256.Sum256([]byte(expectedPayload))
	expectedHex := hex.EncodeToString(sum[:])
	assert.Equal(t, expectedHex, checksum)

	verifiedHex, err := compactor.VerifyChecksum(bucket, key)
	require.NoError(t, err)
	assert.Equal(t, expectedHex, verifiedHex)
}

// TestCASCompactor_StaticFloorInvariant verifies configuration bounds, non-blank identifiers,
// payload size limits, and initial snapshot invariants.
func TestCASCompactor_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	invalidConfigs := []struct {
		name string
		cfg  CompactionConfig
	}{
		{"zero min page size", CompactionConfig{MinPageSize: 0, MaxPageSize: 1024}},
		{"negative min page size", CompactionConfig{MinPageSize: -8, MaxPageSize: 1024}},
		{"max smaller than min", CompactionConfig{MinPageSize: 64, MaxPageSize: 32}},
	}
	for _, tc := range invalidConfigs {
		_, err := NewCASCompactor(tc.cfg)
		assert.ErrorIs(t, err, ErrInvalidCompactionConfig, tc.name)
	}

	compactor := newTestCASCompactor(t, 1)
	snap := compactor.Snapshot()
	assert.Equal(t, uint64(0), snap.Generation)
	assert.Equal(t, 0, snap.ActivePages)
	assert.Equal(t, int64(0), snap.TotalBytes)

	require.NoError(t, compactor.PutPage(makeTestPage("pg-1", "bucket-a", "obj-1", 1, "alpha-payload")))
	assertRetrievedObject(t, compactor, "bucket-a", "obj-1", "alpha-payload")
}

// TestCASCompactor_OperationalDynamicProof verifies generational reclamation, SHA-256 re-indexing,
// zero-loss retrieval, and concurrent uncommitted write transaction shielding.
func TestCASCompactor_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	compactor := newTestCASCompactor(t, 1)

	require.NoError(t, compactor.AppendGeneration(1, []StoragePage{
		makeTestPage("pg-1a", "cas-main", "doc/readme", 1, "version-one-content"),
		makeTestPage("pg-1b", "cas-main", "doc/config", 1, "config-v1-stable"),
	}))

	require.NoError(t, compactor.AppendGeneration(2, []StoragePage{
		makeTestPage("pg-2a", "cas-main", "doc/readme", 2, "version-two-content"),
	}))

	// Start an uncommitted write transaction pinning doc/config before compaction.
	require.NoError(t, compactor.BeginWriteTx("tx-upgrade-config"))
	require.NoError(t, compactor.StageWritePage("tx-upgrade-config", makeTestPage("pg-3b", "cas-main", "doc/config", 3, "config-v3-staged")))

	// Run concurrent compaction cycles and read verifications while tx is uncommitted.
	wg := goroutinelabels.StartTestGoroutines(t, 4, "cas_compactor_worker", "concurrent compact and verify", func(id int) {
		if id%2 == 0 {
			report, compactErr := compactor.Compact()
			assert.NoError(t, compactErr)
			assert.GreaterOrEqual(t, report.VerifiedChecksums, 2)
			return
		}
		_, verifyErr := compactor.VerifyChecksum("cas-main", "doc/config")
		assert.NoError(t, verifyErr)
	})
	wg.Wait()

	// Uncommitted tx must still be shielded; doc/config remains at v1 until commit.
	assertRetrievedObject(t, compactor, "cas-main", "doc/readme", "version-two-content")
	assertRetrievedObject(t, compactor, "cas-main", "doc/config", "config-v1-stable")

	// Commit the shielded write transaction, then compact to reclaim superseded gen-1 config.
	require.NoError(t, compactor.CommitWriteTx("tx-upgrade-config"))
	assertRetrievedObject(t, compactor, "cas-main", "doc/config", "config-v3-staged")

	finalReport, err := compactor.Compact()
	require.NoError(t, err)
	assert.Equal(t, uint64(3), finalReport.ActiveGeneration)
	assert.Equal(t, 1, finalReport.ReclaimedPageCount)
	assert.Equal(t, int64(len("config-v1-stable")), finalReport.ReclaimedBytes)
	assert.Equal(t, 0, finalReport.PinnedPageCount)
	assert.Equal(t, 2, finalReport.BucketIndexCount)
	assert.Equal(t, 2, finalReport.VerifiedChecksums)

	// Abort lifecycle check: staged write is discarded and prior committed version stays intact.
	require.NoError(t, compactor.BeginWriteTx("tx-abort-me"))
	require.NoError(t, compactor.StageWritePage("tx-abort-me", makeTestPage("pg-4b", "cas-main", "doc/config", 4, "config-v4-aborted")))
	require.NoError(t, compactor.AbortWriteTx("tx-abort-me"))
	assertRetrievedObject(t, compactor, "cas-main", "doc/config", "config-v3-staged")
}

// TestCASCompactor_NegativeAdversarialBoundary verifies strict rejection of blank fields,
// out-of-bounds payloads, non-monotonic generations, corrupted checksums, and invalid tx states.
func TestCASCompactor_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	compactor := newTestCASCompactor(t, 1)
	require.NoError(t, compactor.PutPage(makeTestPage("pg-10", "cas-sec", "key-1", 10, "valid-seed-data")))

	invalidPages := []struct {
		name string
		page StoragePage
		want error
	}{
		{"blank page id", makeTestPage("   ", "cas-sec", "key-2", 11, "valid-data"), ErrInvalidStoragePage},
		{"blank bucket", makeTestPage("pg-11", "", "key-2", 11, "valid-data"), ErrInvalidStoragePage},
		{"blank object key", makeTestPage("pg-12", "cas-sec", "  ", 11, "valid-data"), ErrInvalidStoragePage},
		{"empty payload", makeTestPage("pg-13", "cas-sec", "key-2", 11, ""), ErrInvalidStoragePage},
		{"payload below min", makeTestPage("pg-14", "cas-sec", "key-2", 11, "ab"), ErrPageSizeOutOfBounds},
		{"payload above max", makeTestPage("pg-15", "cas-sec", "key-2", 11, fmt.Sprintf("%05000d", 1)), ErrPageSizeOutOfBounds},
		{"zero generation", makeTestPage("pg-16", "cas-sec", "key-2", 0, "valid-data"), ErrNonMonotonicGeneration},
		{"stale generation", makeTestPage("pg-17", "cas-sec", "key-2", 10, "valid-data"), ErrNonMonotonicGeneration},
		{"regressing generation", makeTestPage("pg-18", "cas-sec", "key-2", 5, "valid-data"), ErrNonMonotonicGeneration},
		{"tampered checksum", StoragePage{
			PageID: "pg-19", Bucket: "cas-sec", ObjectKey: "key-2", Generation: 11,
			Payload: []byte("valid-data"), Checksum: "deadbeef",
		}, ErrChecksumMismatch},
	}

	for _, tc := range invalidPages {
		err := compactor.PutPage(tc.page)
		assert.ErrorIs(t, err, tc.want, tc.name)
	}

	// Missing object lookup and blank key verification.
	_, err := compactor.VerifyChecksum("cas-sec", "non-existent")
	assert.ErrorIs(t, err, ErrObjectNotFound)
	_, _, err = compactor.RetrieveObject("", "key-1")
	assert.ErrorIs(t, err, ErrInvalidStoragePage)

	// Invalid transaction operations.
	assert.ErrorIs(t, compactor.CommitWriteTx("unknown-tx"), ErrWriteTxNotFound)
	assert.ErrorIs(t, compactor.AbortWriteTx("unknown-tx"), ErrWriteTxNotFound)
	assert.ErrorIs(t, compactor.StageWritePage("unknown-tx", makeTestPage("pg-20", "cas-sec", "key-2", 11, "valid-data")), ErrWriteTxNotFound)

	// Simulate in-memory payload corruption and verify Compact and RetrieveObject fail closed.
	compactor.mu.Lock()
	compactor.pages["pg-10"].Payload[0] ^= 0xFF
	compactor.mu.Unlock()

	_, _, err = compactor.RetrieveObject("cas-sec", "key-1")
	assert.ErrorIs(t, err, ErrChecksumMismatch)
	_, err = compactor.Compact()
	assert.ErrorIs(t, err, ErrChecksumMismatch)
}
