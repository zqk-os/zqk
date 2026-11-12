package community

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

// TestSnapshotSync_StaticFloorInvariant verifies CRIT-1794505972597665000-0b146d99:
// static floor invariants, blank parameter validation, non-zero epoch floor, and SHA256 verification.
func TestSnapshotSync_StaticFloorInvariant(t *testing.T) {
	// 1. Controller constructor rejects blank values
	if _, err := NewSnapshotSyncController("", "node-1"); err == nil {
		t.Error("expected error with blank clusterID")
	}
	if _, err := NewSnapshotSyncController("cluster-1", ""); err == nil {
		t.Error("expected error with blank nodeID")
	}

	// 2. SnapshotHeader validation
	h := SnapshotHeader{
		ClusterID:     "cluster-1",
		NodeID:        "node-1",
		Epoch:         1,
		CreatedAt:     time.Now().UTC(),
		PayloadSHA256: "some-sha",
		EntityCount:   1,
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	// 3. Rejects zero epoch
	hZero := h
	hZero.Epoch = 0
	if err := hZero.Validate(); err == nil {
		t.Error("expected error with epoch 0")
	}

	// 4. Rejects zero timestamp
	hNoTime := h
	hNoTime.CreatedAt = time.Time{}
	if err := hNoTime.Validate(); err == nil {
		t.Error("expected error with zero CreatedAt")
	}

	// 5. Correct SHA256 checksum verification
	payload := []byte(`{"goal-1":"complete","goal-2":"in_progress"}`)
	sum := sha256.Sum256(payload)
	validChecksum := hex.EncodeToString(sum[:])

	env := SnapshotEnvelope{
		Header: SnapshotHeader{
			ClusterID:     "cluster-1",
			NodeID:        "node-1",
			Epoch:         10,
			CreatedAt:     time.Now().UTC(),
			PayloadSHA256: validChecksum,
			EntityCount:   2,
		},
		Payload: payload,
	}
	if err := env.VerifyChecksum(); err != nil {
		t.Fatalf("unexpected checksum verification error: %v", err)
	}
}

// TestSnapshotSync_OperationalDynamicProof verifies CRIT-1794505980902621000-31077b96:
// operational snapshot capture, remote node restoration, and deterministic state convergence.
func TestSnapshotSync_OperationalDynamicProof(t *testing.T) {
	primary, err := NewSnapshotSyncController("cluster-prod", "primary-node")
	if err != nil {
		t.Fatalf("unexpected error creating primary: %v", err)
	}

	entities := map[string]string{
		"PRI-140": "complete",
		"PRI-141": "in_progress",
		"GOAL-10": "active",
	}

	// 1. Capture snapshot at epoch 1
	snap, err := primary.CaptureSnapshot(1, entities)
	if err != nil {
		t.Fatalf("unexpected error capturing snapshot: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot envelope")
	}
	if snap.Header.EntityCount != 3 {
		t.Errorf("expected 3 entities, got %d", snap.Header.EntityCount)
	}

	// 2. Restore onto new worker node
	worker, err := NewSnapshotSyncController("cluster-prod", "worker-node-1")
	if err != nil {
		t.Fatalf("unexpected error creating worker: %v", err)
	}

	if err := worker.RestoreSnapshot(snap); err != nil {
		t.Fatalf("unexpected error restoring snapshot on worker: %v", err)
	}

	if worker.CurrentEpoch() != 1 {
		t.Errorf("expected worker epoch 1, got %d", worker.CurrentEpoch())
	}

	// Verify all entities exist on worker with identical values
	for k, expectedVal := range entities {
		actualVal, exists := worker.GetEntity(k)
		if !exists {
			t.Errorf("expected entity %s to exist on worker", k)
		}
		if actualVal != expectedVal {
			t.Errorf("expected entity %s value %s, got %s", k, expectedVal, actualVal)
		}
	}

	// 3. Monotonic epoch advancement
	entities["PRI-141"] = "complete"
	entities["PRI-142"] = "active"
	snap2, err := primary.CaptureSnapshot(2, entities)
	if err != nil {
		t.Fatalf("unexpected error capturing epoch 2: %v", err)
	}
	if err := worker.RestoreSnapshot(snap2); err != nil {
		t.Fatalf("unexpected error advancing worker to epoch 2: %v", err)
	}
	if worker.CurrentEpoch() != 2 {
		t.Errorf("expected worker epoch 2, got %d", worker.CurrentEpoch())
	}
	val, _ := worker.GetEntity("PRI-141")
	if val != "complete" {
		t.Errorf("expected updated PRI-141 to be complete, got %s", val)
	}
}

// TestSnapshotSync_NegativeAdversarialBoundary verifies CRIT-1794505988420521000-cf8aaab5:
// checksum tamper detection, corrupted payload rejection, and pre-epoch WAL replay fencing.
func TestSnapshotSync_NegativeAdversarialBoundary(t *testing.T) {
	node, err := NewSnapshotSyncController("cluster-edge", "edge-node")
	if err != nil {
		t.Fatalf("unexpected error creating node: %v", err)
	}

	// 1. Rejects zero epoch on capture
	if _, err := node.CaptureSnapshot(0, nil); err == nil {
		t.Error("expected error capturing snapshot at epoch 0")
	}

	// 2. Establish baseline at epoch 5
	snap, err := node.CaptureSnapshot(5, map[string]string{"K1": "V1"})
	if err != nil {
		t.Fatalf("unexpected error capturing baseline snapshot: %v", err)
	}

	// 3. Reject non-monotonic capture (same or lesser epoch)
	if _, err := node.CaptureSnapshot(5, nil); err == nil {
		t.Error("expected error capturing with duplicate epoch 5")
	}
	if _, err := node.CaptureSnapshot(3, nil); err == nil {
		t.Error("expected error capturing with regressive epoch 3")
	}

	// 4. Reject tampered payload bytes
	tamperedSnap := &SnapshotEnvelope{
		Header:  snap.Header,
		Payload: []byte(`{"K1":"TAMPERED_BYZANTINE_VALUE"}`),
	}
	if err := node.RestoreSnapshot(tamperedSnap); err == nil {
		t.Error("expected error restoring tampered snapshot with mismatched checksum")
	}

	// 5. Pre-epoch WAL replay fencing
	if node.CanReplayWALEntry(3) {
		t.Error("expected WAL entry at epoch 3 to be fenced when current epoch is 5")
	}
	if node.CanReplayWALEntry(5) {
		t.Error("expected WAL entry at epoch 5 to be fenced when current epoch is 5")
	}
	if !node.CanReplayWALEntry(6) {
		t.Error("expected future WAL entry at epoch 6 to be replayable")
	}

	// 6. Reject regressive restore
	staleEnv := &SnapshotEnvelope{
		Header: SnapshotHeader{
			ClusterID:     "cluster-edge",
			NodeID:        "other-node",
			Epoch:         2,
			CreatedAt:     time.Now().UTC(),
			PayloadSHA256: "",
			EntityCount:   0,
		},
		Payload: []byte(`{}`),
	}
	sum := sha256.Sum256(staleEnv.Payload)
	staleEnv.Header.PayloadSHA256 = hex.EncodeToString(sum[:])
	if err := node.RestoreSnapshot(staleEnv); err == nil {
		t.Error("expected error restoring regressive epoch 2 when current epoch is 5")
	}
}
