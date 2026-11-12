package community

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSwarmPartition_StaticFloorInvariant verifies default configurations, quorum calculation,
// and zero-value structure safety.
func TestSwarmPartition_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cfg := SwarmPartitionConfig{
		NodeID:            "node-1",
		ClusterID:         "cluster-1",
		TotalClusterNodes: 5,
	}

	detector, err := NewSwarmPartitionDetector(cfg)
	require.NoError(t, err)
	require.NotNil(t, detector)

	// In a 5-node cluster, strict majority is (5/2)+1 = 3
	assert.Equal(t, 3, detector.majorityCount)
	assert.Equal(t, 5*time.Second, detector.cfg.LeaseDuration)

	// Even cluster size: 4 nodes -> majority is (4/2)+1 = 3
	cfg4 := SwarmPartitionConfig{
		NodeID:            "node-1",
		ClusterID:         "cluster-1",
		TotalClusterNodes: 4,
	}
	detector4, err := NewSwarmPartitionDetector(cfg4)
	require.NoError(t, err)
	assert.Equal(t, 3, detector4.majorityCount)
}

// TestSwarmPartition_OperationalDynamicProof verifies quorum maintenance with active heartbeats,
// lease acquisition/renewal, write permissions, and partition degradation.
func TestSwarmPartition_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	now := time.Now()
	detector, err := NewSwarmPartitionDetector(SwarmPartitionConfig{
		NodeID:            "node-1",
		ClusterID:         "cluster-prod",
		TotalClusterNodes: 5,
		LeaseDuration:     1 * time.Second,
	})
	require.NoError(t, err)

	// Initially, only self is active (1 node < 3 majority)
	status, count := detector.EvaluateQuorum(now)
	assert.Equal(t, PartitionStatusMinorityFenced, status)
	assert.Equal(t, 1, count)
	assert.False(t, detector.IsWritePermitted(now))

	// Record heartbeats from node-2 and node-3 (activeCount = 3 == majority)
	require.NoError(t, detector.RecordHeartbeat("node-2", now))
	require.NoError(t, detector.RecordHeartbeat("node-3", now))

	status, count = detector.EvaluateQuorum(now)
	assert.Equal(t, PartitionStatusQuorumHealthy, status)
	assert.Equal(t, 3, count)

	// Acquire lease
	lease, err := detector.AcquireOrRenewLease(now)
	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.Equal(t, uint64(1), lease.LeaseEpoch)
	assert.True(t, detector.IsWritePermitted(now))

	// Verify unexpired lease
	assert.NoError(t, detector.VerifyLease(now.Add(500*time.Millisecond)))

	// Advance time past lease duration without new heartbeats -> network partition simulated
	futureTime := now.Add(2 * time.Second)
	status, count = detector.EvaluateQuorum(futureTime)
	assert.Equal(t, PartitionStatusMinorityFenced, status)
	assert.Equal(t, 1, count)
	assert.False(t, detector.IsWritePermitted(futureTime))
	assert.ErrorIs(t, detector.VerifyLease(futureTime), ErrLeaseExpired)
}

// TestSwarmPartition_NegativeAdversarialBoundary validates error handling, zero/negative cluster sizes,
// blank IDs, expired lease revocations, and lease acquisition rejection when in minority partition.
func TestSwarmPartition_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	// Blank input validation
	_, err := NewSwarmPartitionDetector(SwarmPartitionConfig{ClusterID: "c1", TotalClusterNodes: 3})
	assert.ErrorIs(t, err, ErrBlankPartitionNodeID)

	_, err = NewSwarmPartitionDetector(SwarmPartitionConfig{NodeID: "n1", TotalClusterNodes: 3})
	assert.ErrorIs(t, err, ErrBlankPartitionClusterID)

	_, err = NewSwarmPartitionDetector(SwarmPartitionConfig{
		NodeID:            "n1",
		ClusterID:         "c1",
		TotalClusterNodes: 0,
	})
	assert.ErrorIs(t, err, ErrInvalidClusterSize)

	detector, err := NewSwarmPartitionDetector(SwarmPartitionConfig{
		NodeID:            "n1",
		ClusterID:         "c1",
		TotalClusterNodes: 3,
	})
	require.NoError(t, err)

	assert.ErrorIs(t, detector.RecordHeartbeat("", time.Now()), ErrBlankPartitionNodeID)
	assert.ErrorIs(t, detector.RecordHeartbeat("   ", time.Now()), ErrBlankPartitionNodeID)

	// Attempting lease acquisition in minority partition
	_, err = detector.AcquireOrRenewLease(time.Now())
	assert.ErrorIs(t, err, ErrQuorumNotReached)

	// Lease never acquired -> VerifyLease fails
	assert.ErrorIs(t, detector.VerifyLease(time.Now()), ErrLeaseExpired)
}
