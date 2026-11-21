package community

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func newTestMerkleDAGNode(regionID, nodeID string, seq uint64, parents []string, payload string) MerkleDAGNode {
	contentHash := ComputeSHA256Hex([]byte(payload))
	merkleHash := ComputeMerkleNodeHash(nodeID, regionID, seq, contentHash, parents)
	return MerkleDAGNode{
		NodeID:       nodeID,
		RegionID:     regionID,
		Sequence:     seq,
		ParentHashes: parents,
		ContentHash:  contentHash,
		MerkleHash:   merkleHash,
		Payload:      []byte(payload),
		CreatedAt:    time.Date(2026, 11, 20, 12, 0, int(seq), 0, time.UTC),
	}
}

func assertMerkleErrorIs(t *testing.T, err, target error, label string) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s: expected error %v, got %v", label, target, err)
	}
}

func assertRegionQuarantined(t *testing.T, syncer *MerkleDAGSynchronizer, regionID string) {
	t.Helper()
	st, ok := syncer.RegionState(regionID)
	if !ok || !st.Quarantined {
		t.Fatalf("expected region %s to be quarantined, state=%+v ok=%v", regionID, st, ok)
	}
}

// TestMerkleDAGSync_StaticFloorInvariant verifies schema validation for MerkleDAGNode,
// RangeProof, and MerkleDAGSynchronizer construction.
func TestMerkleDAGSync_StaticFloorInvariant(t *testing.T) {
	if _, err := NewMerkleDAGSynchronizer("   "); !errors.Is(err, ErrBlankRegionID) {
		t.Fatalf("expected ErrBlankRegionID for blank local region, got %v", err)
	}

	validNode := newTestMerkleDAGNode("us-west-1", "node-1", 1, nil, "genesis-state")
	if err := validNode.Validate(); err != nil {
		t.Fatalf("expected valid node to pass Validate, got %v", err)
	}

	badRegion := validNode
	badRegion.RegionID = " \t "
	assertMerkleErrorIs(t, badRegion.Validate(), ErrBlankRegionID, "blank node region")

	badNodeID := validNode
	badNodeID.NodeID = ""
	assertMerkleErrorIs(t, badNodeID.Validate(), ErrBlankMerkleNodeID, "blank node ID")

	badContent := validNode
	badContent.ContentHash = "not-a-sha256"
	assertMerkleErrorIs(t, badContent.Validate(), ErrInvalidSHA256Digest, "invalid content_hash")

	badParent := validNode
	badParent.ParentHashes = []string{"short"}
	assertMerkleErrorIs(t, badParent.Validate(), ErrInvalidSHA256Digest, "invalid parent_hash")

	proof, err := BuildRangeProof("us-west-1", "a", "z", []string{validNode.MerkleHash})
	if err != nil {
		t.Fatalf("BuildRangeProof failed: %v", err)
	}
	if err := VerifyRangeProof(proof); err != nil {
		t.Fatalf("VerifyRangeProof failed: %v", err)
	}

	if _, err := BuildRangeProof("us-west-1", "z", "a", []string{validNode.MerkleHash}); !errors.Is(err, ErrInvalidKeyRange) {
		t.Fatalf("expected ErrInvalidKeyRange for inverted bounds, got %v", err)
	}
	if _, err := BuildRangeProof("us-west-1", " ", "z", []string{validNode.MerkleHash}); !errors.Is(err, ErrInvalidKeyRange) {
		t.Fatalf("expected ErrInvalidKeyRange for blank start key, got %v", err)
	}
	if _, err := BuildRangeProof("us-west-1", "a", "z", nil); !errors.Is(err, ErrInvalidRangeProof) {
		t.Fatalf("expected ErrInvalidRangeProof for empty leaves, got %v", err)
	}
}

// TestMerkleDAGSync_OperationalDynamicProof verifies multi-region differential traversal,
// out-of-order causal batch resolution, deterministic re-rooting, and concurrent region syncs.
func TestMerkleDAGSync_OperationalDynamicProof(t *testing.T) {
	syncA, err := NewMerkleDAGSynchronizer("us-west-1")
	if err != nil {
		t.Fatalf("NewMerkleDAGSynchronizer failed: %v", err)
	}
	syncB, err := NewMerkleDAGSynchronizer("eu-central-1")
	if err != nil {
		t.Fatalf("NewMerkleDAGSynchronizer failed: %v", err)
	}

	n1 := newTestMerkleDAGNode("eu-central-1", "n1", 1, nil, "state-1")
	n2 := newTestMerkleDAGNode("eu-central-1", "n2", 2, []string{n1.MerkleHash}, "state-2")
	n3a := newTestMerkleDAGNode("eu-central-1", "n3a", 3, []string{n2.MerkleHash}, "branch-a")
	n3b := newTestMerkleDAGNode("eu-central-1", "n3b", 3, []string{n2.MerkleHash}, "branch-b")

	// Insert in forward order on syncB.
	for _, n := range []MerkleDAGNode{n1, n2, n3a, n3b} {
		if err := syncB.AppendNode(n); err != nil {
			t.Fatalf("AppendNode %s failed: %v", n.NodeID, err)
		}
	}

	// Compute differential nodes from syncB assuming syncA knows n1.
	known := map[string]struct{}{n1.MerkleHash: {}}
	delta, err := syncB.ComputeDifferentialNodes("eu-central-1", known)
	if err != nil {
		t.Fatalf("ComputeDifferentialNodes failed: %v", err)
	}
	if len(delta) != 3 {
		t.Fatalf("expected 3 differential nodes, got %d", len(delta))
	}

	// Deliver all 4 nodes to syncA in reverse order; SyncDelta must resolve causal order.
	proof, err := BuildRangeProof("eu-central-1", "n1", "n3b", []string{
		n1.MerkleHash, n2.MerkleHash, n3a.MerkleHash, n3b.MerkleHash,
	})
	if err != nil {
		t.Fatalf("BuildRangeProof failed: %v", err)
	}

	applied, err := syncA.SyncDelta("eu-central-1", []MerkleDAGNode{n3b, n3a, n2, n1}, proof)
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}
	if len(applied) != 4 {
		t.Fatalf("expected 4 applied nodes, got %d", len(applied))
	}

	stateA, okA := syncA.RegionState("eu-central-1")
	stateB, okB := syncB.RegionState("eu-central-1")
	if !okA || !okB {
		t.Fatalf("expected region state in both synchronizers: okA=%v okB=%v", okA, okB)
	}
	if stateA.RootHash != stateB.RootHash {
		t.Fatalf("deterministic re-root mismatch: syncA=%s syncB=%s", stateA.RootHash, stateB.RootHash)
	}

	// Concurrent multi-region synchronization stress test using goroutinelabels.StartTestGoroutines.
	var errCount atomic.Int64
	wg := goroutinelabels.StartTestGoroutines(t, 6, "merkle_dag_region_sync", "concurrent region sync", func(id int) {
		region := fmt.Sprintf("ap-region-%d", id)
		g := newTestMerkleDAGNode(region, fmt.Sprintf("g-%d", id), 1, nil, fmt.Sprintf("genesis-%d", id))
		c := newTestMerkleDAGNode(region, fmt.Sprintf("c-%d", id), 2, []string{g.MerkleHash}, fmt.Sprintf("child-%d", id))
		p, pErr := BuildRangeProof(region, "c", "g", []string{g.MerkleHash, c.MerkleHash})
		if pErr != nil {
			errCount.Add(1)
			return
		}
		if _, sErr := syncA.SyncDelta(region, []MerkleDAGNode{c, g}, p); sErr != nil {
			errCount.Add(1)
		}
	})
	wg.Wait()

	if errCount.Load() != 0 {
		t.Fatalf("expected 0 errors during concurrent region sync, got %d", errCount.Load())
	}
}

// TestMerkleDAGSync_NegativeAdversarialBoundary verifies rejection and immediate quarantine
// for invalid range proofs, tampered Merkle roots, and missing causal ancestors.
func TestMerkleDAGSync_NegativeAdversarialBoundary(t *testing.T) {
	syncer, err := NewMerkleDAGSynchronizer("us-east-1")
	if err != nil {
		t.Fatalf("NewMerkleDAGSynchronizer failed: %v", err)
	}

	// 1. Tampered range proof root -> ErrTamperedMerkleRoot + quarantine.
	n1 := newTestMerkleDAGNode("rogue-region-1", "n1", 1, nil, "payload-1")
	badProof, err := BuildRangeProof("rogue-region-1", "a", "z", []string{n1.MerkleHash})
	if err != nil {
		t.Fatalf("BuildRangeProof failed: %v", err)
	}
	badProof.ExpectedRoot = ComputeSHA256Hex([]byte("forged-root"))

	_, err = syncer.SyncDelta("rogue-region-1", []MerkleDAGNode{n1}, badProof)
	assertMerkleErrorIs(t, err, ErrTamperedMerkleRoot, "tampered range proof root")
	assertRegionQuarantined(t, syncer, "rogue-region-1")

	// Subsequent sync or append on quarantined region must fail immediately with ErrNodeQuarantined.
	_, err = syncer.SyncDelta("rogue-region-1", []MerkleDAGNode{n1}, badProof)
	assertMerkleErrorIs(t, err, ErrNodeQuarantined, "quarantined region re-sync")
	assertMerkleErrorIs(t, syncer.AppendNode(n1), ErrNodeQuarantined, "quarantined region append")

	// 2. Tampered node MerkleHash -> ErrTamperedMerkleRoot + quarantine.
	tamperedNode := newTestMerkleDAGNode("rogue-region-2", "n1", 1, nil, "clean")
	tamperedNode.ContentHash = ComputeSHA256Hex([]byte("tampered-content"))
	validProof2, err := BuildRangeProof("rogue-region-2", "a", "z", []string{tamperedNode.MerkleHash})
	if err != nil {
		t.Fatalf("BuildRangeProof failed: %v", err)
	}
	_, err = syncer.SyncDelta("rogue-region-2", []MerkleDAGNode{tamperedNode}, validProof2)
	assertMerkleErrorIs(t, err, ErrTamperedMerkleRoot, "tampered node digest")
	assertRegionQuarantined(t, syncer, "rogue-region-2")

	// 3. Missing causal ancestor -> ErrMissingCausalAncestor + quarantine.
	missingParentHash := ComputeSHA256Hex([]byte("non-existent-parent"))
	orphan := newTestMerkleDAGNode("rogue-region-3", "orphan", 2, []string{missingParentHash}, "orphan-data")
	validProof3, err := BuildRangeProof("rogue-region-3", "a", "z", []string{orphan.MerkleHash})
	if err != nil {
		t.Fatalf("BuildRangeProof failed: %v", err)
	}
	_, err = syncer.SyncDelta("rogue-region-3", []MerkleDAGNode{orphan}, validProof3)
	assertMerkleErrorIs(t, err, ErrMissingCausalAncestor, "missing causal ancestor")
	assertRegionQuarantined(t, syncer, "rogue-region-3")

	// 4. Forged boundary tag -> ErrInvalidRangeProof + quarantine.
	n4 := newTestMerkleDAGNode("rogue-region-4", "n1", 1, nil, "data")
	forgedProof, err := BuildRangeProof("rogue-region-4", "a", "z", []string{n4.MerkleHash})
	if err != nil {
		t.Fatalf("BuildRangeProof failed: %v", err)
	}
	forgedProof.BoundaryPath[0] = ComputeSHA256Hex([]byte("wrong-boundary"))
	_, err = syncer.SyncDelta("rogue-region-4", []MerkleDAGNode{n4}, forgedProof)
	assertMerkleErrorIs(t, err, ErrInvalidRangeProof, "forged boundary path")
	assertRegionQuarantined(t, syncer, "rogue-region-4")
}
