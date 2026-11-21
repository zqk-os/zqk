package community

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testCRDTClusterAlpha   = "cluster-alpha"
	testCRDTClusterBeta    = "cluster-beta"
	testCRDTDocSyncMesh    = "doc-sync-mesh"
	testCRDTDocBoundary    = "doc-boundary"
	testCRDTDocForeign     = "doc-foreign"
	testCRDTNodeAlpha      = "node-alpha"
	testCRDTNodeBeta       = "node-beta"
	testCRDTKeyLeader      = "leader"
	testCRDTKeyCluster     = "cluster_state"
	testCRDTSvcStorage     = "service-storage"
	testCRDTSvcGateway     = "service-gateway"
	testCRDTSvcTelemetry   = "service-telemetry"
	testCRDTValueActive    = "active"
	testCRDTValueTieAlpha  = "tie-alpha"
	testCRDTValueTieZulu   = "tie-zulu"
	testCRDTBlankSpaces    = "   "
	testCRDTForgedDigest   = "deadbeef00000000"
	testCRDTConcurrentDesc = "concurrent crdt delta and register convergence"
)

func mustNewTestCRDTDocument(t *testing.T, clusterID, docID string) *CRDTDocument {
	t.Helper()
	doc, err := NewCRDTDocument(clusterID, docID)
	require.NoError(t, err)
	require.NotNil(t, doc)
	return doc
}

func mustBuildSealedCRDTDelta(t *testing.T, clusterID, docID, senderID string, ts int64, regs map[string]LWWRegister) *CRDTDelta {
	t.Helper()
	delta := &CRDTDelta{
		ClusterID:  clusterID,
		DocumentID: docID,
		SenderID:   senderID,
		Timestamp:  ts,
		Registers:  regs,
	}
	delta.Seal()
	return delta
}

// TestCRDTConvergence_StaticFloorInvariant validates document schema, LWW register
// construction, vector clock tracking, and deterministic SHA-256 state digests.
func TestCRDTConvergence_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	doc := mustNewTestCRDTDocument(t, testCRDTClusterAlpha, testCRDTDocSyncMesh)
	assert.Equal(t, testCRDTClusterAlpha, doc.ClusterID())
	assert.Equal(t, testCRDTDocSyncMesh, doc.ID())

	require.NoError(t, doc.SetRegister(testCRDTKeyCluster, testCRDTValueActive, 100, testCRDTNodeAlpha))
	reg, ok := doc.GetRegister(testCRDTKeyCluster)
	require.True(t, ok)
	assert.Equal(t, testCRDTValueActive, reg.Value)
	assert.Equal(t, int64(100), reg.Timestamp)
	assert.Equal(t, testCRDTNodeAlpha, reg.WriterID)
	assert.Len(t, reg.ValueDigest, 64)

	require.NoError(t, doc.AddElement(testCRDTSvcStorage, 100, testCRDTNodeAlpha))
	assert.True(t, doc.HasElement(testCRDTSvcStorage))
	assert.Equal(t, []string{testCRDTSvcStorage}, doc.Elements())

	vc := doc.VectorClock()
	assert.Equal(t, uint64(2), vc.Get(testCRDTNodeAlpha))

	digestA := doc.StateDigest()
	assert.Len(t, digestA, 64)
	assert.Equal(t, digestA, doc.StateDigest())

	delta, err := doc.BuildDelta(testCRDTNodeAlpha, 120)
	require.NoError(t, err)
	require.NoError(t, delta.Validate())
}

// TestCRDTConvergence_OperationalDynamicProof verifies commutative, associative,
// idempotent merging, deterministic tie-breaking, and concurrent thread-safe convergence.
func TestCRDTConvergence_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	docA := mustNewTestCRDTDocument(t, testCRDTClusterAlpha, testCRDTDocSyncMesh)
	docB := mustNewTestCRDTDocument(t, testCRDTClusterAlpha, testCRDTDocSyncMesh)

	require.NoError(t, docA.SetRegister(testCRDTKeyLeader, testCRDTNodeAlpha, 150, testCRDTNodeAlpha))
	require.NoError(t, docA.AddElement(testCRDTSvcStorage, 100, testCRDTNodeAlpha))
	require.NoError(t, docA.AddElement(testCRDTSvcGateway, 120, testCRDTNodeAlpha))

	require.NoError(t, docB.SetRegister(testCRDTKeyLeader, testCRDTNodeBeta, 200, testCRDTNodeBeta))
	require.NoError(t, docB.AddElement(testCRDTSvcStorage, 110, testCRDTNodeBeta))
	require.NoError(t, docB.RemoveElement(testCRDTSvcGateway, 130, testCRDTNodeBeta))
	require.NoError(t, docB.AddElement(testCRDTSvcTelemetry, 140, testCRDTNodeBeta))

	// Same timestamp and writer ID tie-break deterministically on valueDigest.
	regTie1, err := NewLWWRegister(testCRDTValueTieAlpha, 500, testCRDTNodeAlpha)
	require.NoError(t, err)
	regTie2, err := NewLWWRegister(testCRDTValueTieZulu, 500, testCRDTNodeAlpha)
	require.NoError(t, err)
	assert.Equal(t, regTie1.Merge(regTie2), regTie2.Merge(regTie1))

	mergedAB := mustNewTestCRDTDocument(t, testCRDTClusterAlpha, testCRDTDocSyncMesh)
	require.NoError(t, mergedAB.Merge(docA))
	require.NoError(t, mergedAB.Merge(docB))

	mergedBA := mustNewTestCRDTDocument(t, testCRDTClusterAlpha, testCRDTDocSyncMesh)
	require.NoError(t, mergedBA.Merge(docB))
	require.NoError(t, mergedBA.Merge(docA))

	assert.Equal(t, mergedAB.StateDigest(), mergedBA.StateDigest())
	assert.Equal(t, mergedAB.Elements(), mergedBA.Elements())
	assert.False(t, mergedAB.HasElement(testCRDTSvcGateway))
	assert.True(t, mergedAB.HasElement(testCRDTSvcStorage))
	assert.True(t, mergedAB.HasElement(testCRDTSvcTelemetry))

	// Concurrent delta applications and reads must remain race-free and convergent.
	concurrentDoc := mustNewTestCRDTDocument(t, testCRDTClusterAlpha, testCRDTDocSyncMesh)
	wg := goroutinelabels.StartTestGoroutines(t, 6, "crdt_worker", testCRDTConcurrentDesc, func(id int) {
		nodeID := fmt.Sprintf("worker-%d", id)
		reg, regErr := NewLWWRegister(nodeID, int64(300+id), nodeID)
		assert.NoError(t, regErr)
		delta := mustBuildSealedCRDTDelta(t, testCRDTClusterAlpha, testCRDTDocSyncMesh, nodeID, int64(300+id), map[string]LWWRegister{
			testCRDTKeyLeader: reg,
		})
		assert.NoError(t, concurrentDoc.ApplyDelta(delta))
		digest := concurrentDoc.StateDigest()
		assert.Len(t, digest, 64)
	})
	wg.Wait()

	finalLeader, ok := concurrentDoc.GetRegister(testCRDTKeyLeader)
	require.True(t, ok)
	assert.Equal(t, int64(305), finalLeader.Timestamp)
	assert.Equal(t, "worker-5", finalLeader.WriterID)
}

// TestCRDTConvergence_NegativeAdversarialBoundary verifies strict rejection of
// malformed inputs, cluster mismatches, and forged digests without state mutation.
func TestCRDTConvergence_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	badClusterDoc, err := NewCRDTDocument(testCRDTBlankSpaces, testCRDTDocBoundary)
	assert.Nil(t, badClusterDoc)
	assert.ErrorIs(t, err, ErrBlankClusterID)

	badIDDoc, err := NewCRDTDocument(testCRDTClusterAlpha, testCRDTBlankSpaces)
	assert.Nil(t, badIDDoc)
	assert.ErrorIs(t, err, ErrBlankID)

	doc := mustNewTestCRDTDocument(t, testCRDTClusterAlpha, testCRDTDocBoundary)
	require.NoError(t, doc.SetRegister(testCRDTKeyCluster, testCRDTValueActive, 100, testCRDTNodeAlpha))
	baselineDigest := doc.StateDigest()

	assert.ErrorIs(t, doc.SetRegister(testCRDTBlankSpaces, testCRDTValueActive, 110, testCRDTNodeAlpha), ErrBlankID)
	assert.ErrorIs(t, doc.SetRegister(testCRDTKeyCluster, testCRDTValueActive, 0, testCRDTNodeAlpha), ErrInvalidTimestamp)
	assert.ErrorIs(t, doc.SetRegister(testCRDTKeyCluster, testCRDTValueActive, -5, testCRDTNodeAlpha), ErrInvalidTimestamp)
	assert.ErrorIs(t, doc.ApplyDelta(nil), ErrNilDelta)

	forgedDelta := mustBuildSealedCRDTDelta(t, testCRDTClusterAlpha, testCRDTDocBoundary, testCRDTNodeBeta, 200, map[string]LWWRegister{
		testCRDTKeyLeader: {Value: testCRDTNodeBeta, Timestamp: 200, WriterID: testCRDTNodeBeta},
	})
	forgedDelta.Digest = testCRDTForgedDigest

	cases := []struct {
		name    string
		delta   *CRDTDelta
		wantErr error
	}{
		{
			name:    "cluster mismatch",
			delta:   mustBuildSealedCRDTDelta(t, testCRDTClusterBeta, testCRDTDocBoundary, testCRDTNodeBeta, 200, nil),
			wantErr: ErrClusterMismatch,
		},
		{
			name:    "document mismatch",
			delta:   mustBuildSealedCRDTDelta(t, testCRDTClusterAlpha, testCRDTDocForeign, testCRDTNodeBeta, 200, nil),
			wantErr: ErrCorruptPayload,
		},
		{
			name:    "forged digest",
			delta:   forgedDelta,
			wantErr: ErrDigestMismatch,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, doc.ApplyDelta(tc.delta), tc.wantErr)
			assert.Equal(t, baselineDigest, doc.StateDigest())
		})
	}

	foreignClusterDoc := mustNewTestCRDTDocument(t, testCRDTClusterBeta, testCRDTDocBoundary)
	assert.ErrorIs(t, doc.Merge(foreignClusterDoc), ErrClusterMismatch)

	foreignIDDoc := mustNewTestCRDTDocument(t, testCRDTClusterAlpha, testCRDTDocForeign)
	assert.ErrorIs(t, doc.Merge(foreignIDDoc), ErrCorruptPayload)
	assert.Equal(t, baselineDigest, doc.StateDigest())
}
