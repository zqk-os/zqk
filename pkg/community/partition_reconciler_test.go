package community

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testPartitionClusterID    = "cluster-alpha"
	testPartitionForeignID    = "cluster-beta"
	testPartitionLocalNodeID  = "node-1"
	testPartitionNode2ID      = "node-2"
	testPartitionNode4ID      = "node-4"
	testPartitionBranchMain   = "branch-main"
	testPartitionBranchMinor  = "branch-minor"
	testPartitionKeyConfig    = "config/mode"
	testPartitionKeyTelemetry = "telemetry/rate"
	testPartitionKeyAudit     = "audit/trail"
	testPartitionValStrict    = "strict"
	testPartitionValConverged = "converged"
)

func newTestPartitionConfig() PartitionReconcilerConfig {
	return PartitionReconcilerConfig{
		ClusterID:       testPartitionClusterID,
		LocalNodeID:     testPartitionLocalNodeID,
		TotalValidators: 5,
		QuorumThreshold: 3,
	}
}

func makeSealedBranch(
	t *testing.T,
	branchID string,
	clusterID string,
	epoch uint64,
	clock PartitionVectorClock,
	voters []string,
	deltas []StateDelta,
) PartitionBranch {
	t.Helper()
	b := PartitionBranch{
		BranchID:             branchID,
		ClusterID:            clusterID,
		Epoch:                epoch,
		PartitionVectorClock: clock,
		VoterIDs:             voters,
		Deltas:               deltas,
	}
	require.NoError(t, b.Seal())
	return b
}

func TestPartitionReconciler_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, StrictMajorityQuorum(0))
	assert.Equal(t, 1, StrictMajorityQuorum(1))
	assert.Equal(t, 2, StrictMajorityQuorum(3))
	assert.Equal(t, 3, StrictMajorityQuorum(5))
	assert.True(t, PartitionStateHealthy.IsValid())
	assert.True(t, PartitionStateSplitBrain.IsValid())
	assert.False(t, PartitionState("unknown").IsValid())

	cases := []struct {
		name    string
		mutate  func(*PartitionReconcilerConfig)
		wantErr error
	}{
		{
			name:    "valid default majority",
			mutate:  func(c *PartitionReconcilerConfig) { c.QuorumThreshold = 0 },
			wantErr: nil,
		},
		{
			name:    "blank cluster ID",
			mutate:  func(c *PartitionReconcilerConfig) { c.ClusterID = "   " },
			wantErr: ErrBlankClusterID,
		},
		{
			name:    "blank local node ID",
			mutate:  func(c *PartitionReconcilerConfig) { c.LocalNodeID = "" },
			wantErr: ErrInvalidPartitionConfig,
		},
		{
			name:    "non-positive total validators",
			mutate:  func(c *PartitionReconcilerConfig) { c.TotalValidators = 0 },
			wantErr: ErrInvalidPartitionConfig,
		},
		{
			name:    "sub-majority quorum threshold",
			mutate:  func(c *PartitionReconcilerConfig) { c.QuorumThreshold = 2 },
			wantErr: ErrInvalidPartitionConfig,
		},
		{
			name:    "quorum exceeds total validators",
			mutate:  func(c *PartitionReconcilerConfig) { c.QuorumThreshold = 6 },
			wantErr: ErrInvalidPartitionConfig,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestPartitionConfig()
			tc.mutate(&cfg)
			rec, err := NewPartitionReconciler(cfg)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, rec)
				return
			}
			require.NoError(t, err)
			state, epoch, clock := rec.State()
			assert.Equal(t, PartitionStateHealthy, state)
			assert.Equal(t, uint64(1), epoch)
			assert.Empty(t, clock)
		})
	}
}

func TestPartitionReconciler_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	rec, err := NewPartitionReconciler(newTestPartitionConfig())
	require.NoError(t, err)

	majorityBranch := makeSealedBranch(
		t,
		testPartitionBranchMain,
		testPartitionClusterID,
		1,
		PartitionVectorClock{testPartitionLocalNodeID: 2, testPartitionNode2ID: 1},
		[]string{"v1", "v2", "v3"},
		[]StateDelta{
			{Key: testPartitionKeyConfig, Value: testPartitionValStrict, Version: 2, OriginNode: testPartitionLocalNodeID},
		},
	)
	minorityBranch := makeSealedBranch(
		t,
		testPartitionBranchMinor,
		testPartitionClusterID,
		1,
		PartitionVectorClock{testPartitionNode4ID: 3, "node-5": 1},
		[]string{"v4", "v5"},
		[]StateDelta{
			{Key: testPartitionKeyConfig, Value: "relaxed", Version: 9, OriginNode: testPartitionNode4ID},
			{Key: testPartitionKeyTelemetry, Value: "100hz", Version: 1, OriginNode: "node-5"},
		},
	)

	require.NoError(t, rec.RecordBranch(minorityBranch))
	stateAfterMinor, _, _ := rec.State()
	assert.Equal(t, PartitionStatePartitioned, stateAfterMinor)

	require.NoError(t, rec.RecordBranch(majorityBranch))
	stateAfterSplit, _, _ := rec.State()
	assert.Equal(t, PartitionStateSplitBrain, stateAfterSplit)

	report, err := rec.Reconcile([]PartitionBranch{minorityBranch, majorityBranch})
	require.NoError(t, err)
	assert.Equal(t, testPartitionBranchMain, report.WinningBranchID)
	assert.Equal(t, uint64(2), report.NewEpoch)
	assert.Equal(t, PartitionStateHealthy, report.PartitionState)
	assert.Equal(t, testPartitionValStrict, report.UnifiedState[testPartitionKeyConfig])
	assert.Equal(t, "100hz", report.UnifiedState[testPartitionKeyTelemetry])
	assert.Equal(t, uint64(2), report.UnifiedClock[testPartitionLocalNodeID])
	assert.Equal(t, uint64(3), report.UnifiedClock[testPartitionNode4ID])
	assert.Len(t, rec.History(), 3)

	runConcurrentPartitionReconciliationProof(t, rec, report.UnifiedClock)
}

func runConcurrentPartitionReconciliationProof(
	t *testing.T,
	rec *PartitionReconciler,
	baseClock PartitionVectorClock,
) {
	t.Helper()

	wg := goroutinelabels.StartTestGoroutines(t, 4, "partition_reconciler_worker", "concurrent branch and heal", func(id int) {
		if id%2 == 1 {
			snapshot := rec.SnapshotState()
			assert.Equal(t, testPartitionValStrict, snapshot[testPartitionKeyConfig])
			return
		}
		clock := baseClock.Clone()
		nodeKey := fmt.Sprintf("worker-%d", id)
		clock[nodeKey] = uint64(id + 1)
		branch := makeSealedBranch(
			t,
			fmt.Sprintf("concurrent-branch-%d", id),
			testPartitionClusterID,
			2,
			clock,
			[]string{"v1", "v2", "v3"},
			[]StateDelta{
				{Key: testPartitionKeyAudit, Value: fmt.Sprintf("ok-%d", id), Version: 1, OriginNode: nodeKey},
			},
		)
		recordErr := rec.RecordBranch(branch)
		assert.NoError(t, recordErr)
	})
	wg.Wait()

	_, currentEpoch, mergedClock := rec.State()
	finalBranch := makeSealedBranch(
		t,
		"branch-converged",
		testPartitionClusterID,
		currentEpoch,
		mergedClock.Merge(PartitionVectorClock{testPartitionNode2ID: 5}),
		[]string{"v1", "v2", "v3", "v4"},
		[]StateDelta{
			{Key: testPartitionKeyAudit, Value: testPartitionValConverged, Version: 2, OriginNode: testPartitionNode2ID},
		},
	)
	finalReport, err := rec.Reconcile([]PartitionBranch{finalBranch})
	require.NoError(t, err)
	assert.Equal(t, testPartitionValConverged, finalReport.UnifiedState[testPartitionKeyAudit])
	assert.Equal(t, uint64(3), finalReport.NewEpoch)
}

func TestPartitionReconciler_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	rec, err := NewPartitionReconciler(newTestPartitionConfig())
	require.NoError(t, err)

	seedBranch := makeSealedBranch(
		t,
		testPartitionBranchMain,
		testPartitionClusterID,
		2,
		PartitionVectorClock{testPartitionLocalNodeID: 4},
		[]string{"v1", "v2", "v3"},
		[]StateDelta{{Key: testPartitionKeyConfig, Value: "v2", Version: 2, OriginNode: testPartitionLocalNodeID}},
	)
	_, err = rec.Reconcile([]PartitionBranch{seedBranch})
	require.NoError(t, err)

	cases := []struct {
		name     string
		branches []PartitionBranch
		wantErr  error
	}{
		{
			name:     "empty branch list",
			branches: nil,
			wantErr:  ErrInvalidPartitionConfig,
		},
		{
			name: "cluster mismatch rejected",
			branches: []PartitionBranch{
				makeSealedBranch(t, "b-foreign", testPartitionForeignID, 3, PartitionVectorClock{testPartitionLocalNodeID: 5}, []string{"v1", "v2", "v3"}, nil),
			},
			wantErr: ErrClusterMismatch,
		},
		{
			name: "sub-quorum minority rejected",
			branches: []PartitionBranch{
				makeSealedBranch(t, testPartitionBranchMinor, testPartitionClusterID, 3, PartitionVectorClock{testPartitionLocalNodeID: 5}, []string{"v1", "v2"}, nil),
			},
			wantErr: ErrSubQuorumBranch,
		},
		{
			name: "ambiguous dual majority rejected",
			branches: []PartitionBranch{
				makeSealedBranch(t, "b-maj-1", testPartitionClusterID, 3, PartitionVectorClock{testPartitionLocalNodeID: 5}, []string{"v1", "v2", "v3"}, nil),
				makeSealedBranch(t, "b-maj-2", testPartitionClusterID, 3, PartitionVectorClock{testPartitionLocalNodeID: 6}, []string{"v2", "v4", "v5"}, nil),
			},
			wantErr: ErrAmbiguousQuorum,
		},
		{
			name: "stale epoch rollback rejected",
			branches: []PartitionBranch{
				makeSealedBranch(t, "b-stale-epoch", testPartitionClusterID, 2, PartitionVectorClock{testPartitionLocalNodeID: 5}, []string{"v1", "v2", "v3"}, nil),
			},
			wantErr: ErrStalePartitionRollback,
		},
		{
			name: "vector clock regression rollback rejected",
			branches: []PartitionBranch{
				makeSealedBranch(t, "b-stale-clock", testPartitionClusterID, 3, PartitionVectorClock{testPartitionLocalNodeID: 3}, []string{"v1", "v2", "v3"}, nil),
			},
			wantErr: ErrStalePartitionRollback,
		},
		{
			name: "zero vector clock counter rejected",
			branches: []PartitionBranch{
				makeSealedBranch(t, "b-zero-clock", testPartitionClusterID, 3, PartitionVectorClock{testPartitionLocalNodeID: 0}, []string{"v1", "v2", "v3"}, nil),
			},
			wantErr: ErrNonMonotonicVectorClock,
		},
		{
			name: "duplicate voter spoofing rejected",
			branches: []PartitionBranch{
				makeSealedBranch(t, "b-dup-voter", testPartitionClusterID, 3, PartitionVectorClock{testPartitionLocalNodeID: 5}, []string{"v1", "v1", "v2"}, nil),
			},
			wantErr: ErrInvalidPartitionConfig,
		},
		{
			name: "forged state digest rejected",
			branches: func() []PartitionBranch {
				b := makeSealedBranch(t, "b-forged", testPartitionClusterID, 3, PartitionVectorClock{testPartitionLocalNodeID: 5}, []string{"v1", "v2", "v3"}, nil)
				b.StateHash = "deadbeef"
				return []PartitionBranch{b}
			}(),
			wantErr: ErrForgedBranchDigest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, recErr := rec.Reconcile(tc.branches)
			require.ErrorIs(t, recErr, tc.wantErr)
		})
	}
}
