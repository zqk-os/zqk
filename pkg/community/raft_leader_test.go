package community_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/community"
)

// CRIT-1794503104846838000-7f963d0f: Raft-lite leader election rejects zero term, blank candidate ID, or insufficient peers.
func TestRaftLeaderEngine_StaticFloorInvariants(t *testing.T) {
	t.Run("rejects blank node ID", func(t *testing.T) {
		engine, err := community.NewRaftLeaderEngine(community.RaftLeaderConfig{
			NodeID:      "   ",
			ClusterID:   "cluster-alpha",
			ClusterSize: 3,
		})
		assert.ErrorIs(t, err, community.ErrBlankCandidateID)
		assert.Nil(t, engine)
	})

	t.Run("rejects blank cluster ID", func(t *testing.T) {
		engine, err := community.NewRaftLeaderEngine(community.RaftLeaderConfig{
			NodeID:      "node-1",
			ClusterID:   "",
			ClusterSize: 3,
		})
		assert.ErrorIs(t, err, community.ErrBlankRaftClusterID)
		assert.Nil(t, engine)
	})

	t.Run("rejects cluster size zero or negative", func(t *testing.T) {
		engine, err := community.NewRaftLeaderEngine(community.RaftLeaderConfig{
			NodeID:      "node-1",
			ClusterID:   "cluster-alpha",
			ClusterSize: 0,
		})
		assert.ErrorIs(t, err, community.ErrInvalidRaftClusterSize)
		assert.Nil(t, engine)
	})

	t.Run("rejects zero term in vote request", func(t *testing.T) {
		engine, err := community.NewRaftLeaderEngine(community.RaftLeaderConfig{
			NodeID:      "node-1",
			ClusterID:   "cluster-alpha",
			ClusterSize: 3,
		})
		require.NoError(t, err)

		resp, err := engine.HandleVoteRequest(community.RaftVoteRequest{
			Term:        0,
			CandidateID: "node-2",
			ClusterID:   "cluster-alpha",
		})
		assert.ErrorIs(t, err, community.ErrZeroTerm)
		assert.False(t, resp.VoteGranted)
	})

	t.Run("majority quorum calculation", func(t *testing.T) {
		cases := []struct {
			size int
			want int
		}{
			{1, 1},
			{2, 2},
			{3, 2},
			{4, 3},
			{5, 3},
			{7, 4},
		}
		for _, tc := range cases {
			engine, err := community.NewRaftLeaderEngine(community.RaftLeaderConfig{
				NodeID:      "node-1",
				ClusterID:   "cluster-alpha",
				ClusterSize: tc.size,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, engine.MajorityQuorum())
		}
	})
}

// CRIT-1794503110563636000-7620c565: Raft-lite leader election reaches majority quorum, issues fencing lease, and renews heartbeats.
func TestRaftLeaderEngine_OperationalDynamicProof(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

	engine, err := community.NewRaftLeaderEngine(community.RaftLeaderConfig{
		NodeID:        "node-1",
		ClusterID:     "cluster-alpha",
		ClusterSize:   3,
		LeaseDuration: 5 * time.Second,
	})
	require.NoError(t, err)

	// Follower starts election
	req, err := engine.StartElection()
	require.NoError(t, err)
	assert.Equal(t, uint64(2), req.Term)
	assert.Equal(t, "node-1", req.CandidateID)

	term, role, nodeID := engine.State()
	assert.Equal(t, uint64(2), term)
	assert.Equal(t, community.RoleCandidate, role)
	assert.Equal(t, "node-1", nodeID)

	// Collect majority votes (1 peer vote + 1 self vote = 2/3)
	peerVotes := []community.RaftVoteResponse{
		{
			Term:        2,
			VoteGranted: true,
			VoterID:     "node-2",
		},
	}
	lease, err := engine.ClaimLeadership(peerVotes, now)
	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.Equal(t, uint64(2), lease.Term)
	assert.Equal(t, "node-1", lease.LeaderID)
	assert.Equal(t, uint64(1), lease.FencingToken)
	assert.Equal(t, now.Add(5*time.Second), lease.ExpiresAt)

	// Verify active lease
	active, ok := engine.ActiveLease(now.Add(2 * time.Second))
	assert.True(t, ok)
	assert.Equal(t, uint64(1), active.FencingToken)

	// Renew heartbeat
	renewedLease, err := engine.RenewHeartbeat(now.Add(3*time.Second), 1)
	require.NoError(t, err)
	require.NotNil(t, renewedLease)
	assert.Equal(t, uint64(2), renewedLease.FencingToken)
	assert.Equal(t, now.Add(8*time.Second), renewedLease.ExpiresAt)
}

// CRIT-1794503115265089000-fac9f516: Raft-lite election detects split-brain step-down on observing higher term or expired lease.
func TestRaftLeaderEngine_NegativeAdversarialBoundary(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

	engine, err := community.NewRaftLeaderEngine(community.RaftLeaderConfig{
		NodeID:        "node-1",
		ClusterID:     "cluster-alpha",
		ClusterSize:   5,
		LeaseDuration: 4 * time.Second,
	})
	require.NoError(t, err)

	_, err = engine.StartElection()
	require.NoError(t, err)

	// Quorum failure: 1 peer vote in 5-node cluster (total 2/5 < 3)
	insufficientVotes := []community.RaftVoteResponse{
		{Term: 2, VoteGranted: true, VoterID: "node-2"},
	}
	lease, err := engine.ClaimLeadership(insufficientVotes, now)
	assert.Error(t, err)
	assert.Nil(t, lease)

	// Achieves quorum with 2 peer votes + self = 3/5
	sufficientVotes := []community.RaftVoteResponse{
		{Term: 2, VoteGranted: true, VoterID: "node-2"},
		{Term: 2, VoteGranted: true, VoterID: "node-3"},
	}
	leaderLease, err := engine.ClaimLeadership(sufficientVotes, now)
	require.NoError(t, err)
	require.NotNil(t, leaderLease)

	// Higher term observed during ClaimLeadership
	higherTermVotes := []community.RaftVoteResponse{
		{Term: 3, VoteGranted: false, VoterID: "node-4"},
	}
	_, err = engine.ClaimLeadership(higherTermVotes, now)
	assert.ErrorIs(t, err, community.ErrHigherTermObserved)
	term, role, _ := engine.State()
	assert.Equal(t, uint64(3), term)
	assert.Equal(t, community.RoleFollower, role)

	// Re-elect and test heartbeat quorum loss step down
	_, err = engine.StartElection() // term 4
	require.NoError(t, err)
	votes4 := []community.RaftVoteResponse{
		{Term: 4, VoteGranted: true, VoterID: "node-2"},
		{Term: 4, VoteGranted: true, VoterID: "node-3"},
	}
	_, err = engine.ClaimLeadership(votes4, now)
	require.NoError(t, err)

	// Heartbeat with only 1 healthy peer (1 peer + 1 self = 2 < 3) causes step-down
	_, err = engine.RenewHeartbeat(now, 1)
	assert.Error(t, err)
	_, roleAfterFail, _ := engine.State()
	assert.Equal(t, community.RoleFollower, roleAfterFail)

	// Lease expiry check
	_, valid := engine.ActiveLease(now.Add(10 * time.Second))
	assert.False(t, valid)
}
