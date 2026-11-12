package community

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPeerGossip_StaticFloorInvariant verifies initialization defaults and descriptor validation.
func TestPeerGossip_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cfg := PeerGossipConfig{
		LocalPeerID: "peer-local-01",
		ClusterID:   "cluster-prod-01",
	}

	engine, err := NewPeerGossipSyncEngine(cfg)
	require.NoError(t, err)
	require.NotNil(t, engine)

	assert.Equal(t, 15*time.Second, engine.cfg.SuspectTimeout)

	// Local peer automatically registered
	desc, err := engine.GetPeer(cfg.LocalPeerID)
	require.NoError(t, err)
	assert.Equal(t, PeerStatusActive, desc.Status)
	assert.Equal(t, uint64(1), desc.Epoch)
}

// TestPeerGossip_OperationalDynamicProof verifies multi-node peer discovery, digest exchange,
// state updates with epoch advancing, and pruning.
func TestPeerGossip_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	clusterID := "cluster-swarms-east"
	engineA, err := NewPeerGossipSyncEngine(PeerGossipConfig{
		LocalPeerID:    "node-a",
		ClusterID:      clusterID,
		SuspectTimeout: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	// Register remote node B on node A
	nodeB := PeerDescriptor{
		PeerID:       "node-b",
		ClusterID:    clusterID,
		Address:      "10.0.0.2:9000",
		Epoch:        1,
		LastObserved: time.Now(),
		Status:       PeerStatusActive,
	}
	require.NoError(t, engineA.RegisterPeer(nodeB))

	// Digest generated on A reflects node A and node B
	digestA := engineA.GenerateDigest()
	assert.Equal(t, clusterID, digestA.ClusterID)
	assert.Equal(t, uint64(1), digestA.Epochs["node-a"])
	assert.Equal(t, uint64(1), digestA.Epochs["node-b"])

	// Remote node B sends updated epoch 3
	remoteDigest := GossipDigest{
		ClusterID: clusterID,
		Epochs: map[string]uint64{
			"node-a": 1,
			"node-b": 3,
			"node-c": 1, // new node discovered
		},
	}

	staleLocals, err := engineA.ReconcileDigest(remoteDigest)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"node-b", "node-c"}, staleLocals)

	// Apply node B update with higher epoch
	require.NoError(t, engineA.ApplyPeerUpdate(PeerDescriptor{
		PeerID:    "node-b",
		ClusterID: clusterID,
		Epoch:     3,
		Status:    PeerStatusActive,
	}))

	descB, err := engineA.GetPeer("node-b")
	require.NoError(t, err)
	assert.Equal(t, uint64(3), descB.Epoch)

	// Mark suspect and prune
	pastTime := time.Now().Add(-100 * time.Millisecond)
	require.NoError(t, engineA.MarkSuspect("node-b"))
	engineA.mu.Lock()
	mutated := engineA.peers["node-b"]
	mutated.LastObserved = pastTime
	engineA.peers["node-b"] = mutated
	engineA.mu.Unlock()

	pruned := engineA.PruneDeadPeers(time.Now())
	assert.Equal(t, 1, pruned)

	_, err = engineA.GetPeer("node-b")
	assert.ErrorIs(t, err, ErrPeerNotFound)
}

// TestPeerGossip_NegativeAdversarialBoundary verifies rejection of blank IDs, cluster mismatch,
// duplicate registration, and stale epoch rejections.
func TestPeerGossip_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	// Blank config validation
	_, err := NewPeerGossipSyncEngine(PeerGossipConfig{ClusterID: "prod"})
	assert.ErrorIs(t, err, ErrBlankPeerID)

	_, err = NewPeerGossipSyncEngine(PeerGossipConfig{LocalPeerID: "node-1"})
	assert.ErrorIs(t, err, ErrBlankClusterID)

	engine, err := NewPeerGossipSyncEngine(PeerGossipConfig{
		LocalPeerID: "node-neg-local",
		ClusterID:   "cluster-alpha",
	})
	require.NoError(t, err)

	// Blank input validation
	assert.ErrorIs(t, engine.RegisterPeer(PeerDescriptor{}), ErrBlankPeerID)
	assert.ErrorIs(t, engine.RegisterPeer(PeerDescriptor{PeerID: "p1"}), ErrBlankClusterID)

	_, err = engine.GetPeer("")
	assert.ErrorIs(t, err, ErrBlankPeerID)

	assert.ErrorIs(t, engine.MarkSuspect(""), ErrBlankPeerID)
	assert.ErrorIs(t, engine.MarkSuspect("nonexistent"), ErrPeerNotFound)

	// Cluster mismatch rejection
	assert.ErrorIs(t, engine.RegisterPeer(PeerDescriptor{
		PeerID:    "foreign-peer",
		ClusterID: "cluster-beta",
	}), ErrClusterMismatch)

	_, err = engine.ReconcileDigest(GossipDigest{ClusterID: "cluster-beta"})
	assert.ErrorIs(t, err, ErrClusterMismatch)

	// Stale epoch rejection on update
	validPeer := PeerDescriptor{
		PeerID:    "peer-epoch-test",
		ClusterID: "cluster-alpha",
		Epoch:     5,
		Status:    PeerStatusActive,
	}
	require.NoError(t, engine.RegisterPeer(validPeer))

	// Re-register duplicate
	assert.ErrorIs(t, engine.RegisterPeer(validPeer), ErrPeerAlreadyRegistered)

	// Stale epoch
	stalePeer := validPeer
	stalePeer.Epoch = 4
	assert.ErrorIs(t, engine.ApplyPeerUpdate(stalePeer), ErrStaleEpoch)

	// Equal epoch
	stalePeer.Epoch = 5
	assert.ErrorIs(t, engine.ApplyPeerUpdate(stalePeer), ErrStaleEpoch)
}
