package community

import (
	"fmt"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testClusterID    = "cluster-168"
	testLeaderID     = "leader-node"
	testFollowerA    = "follower-a"
	testFollowerB    = "follower-b"
	testReplicaSolo  = "solo-replica"
	testKeyAlpha     = "key-alpha"
	testKeyBeta      = "key-beta"
	testKeyGamma     = "key-gamma"
	testValOne       = "value-one"
	testValTwo       = "value-two"
	testValThree     = "val-three"
	testValCorrupted = "corrupted-val"
	testSnapID       = "snap-manual-1"
)

func mustReplicationConfig(tailWindow, threshold int) ReplicationConfig {
	return ReplicationConfig{
		ClusterID:           testClusterID,
		TailWindow:          tailWindow,
		CompactionThreshold: threshold,
	}
}

func mustNewReplica(t *testing.T, nodeID string, cfg ReplicationConfig) *StateReplica {
	t.Helper()
	replica, err := NewStateReplica(nodeID, cfg)
	if err != nil {
		t.Fatalf("NewStateReplica(%s) failed: %v", nodeID, err)
	}
	return replica
}

func mustNewCluster(t *testing.T, cfg ReplicationConfig) *ReplicationCluster {
	t.Helper()
	cluster, err := NewReplicationCluster(cfg, testLeaderID)
	if err != nil {
		t.Fatalf("NewReplicationCluster failed: %v", err)
	}
	return cluster
}

func mustNewLogEntry(t *testing.T, seq uint64, op TransitionOp, key, val string) LogEntry {
	t.Helper()
	entry, err := NewLogEntry(seq, op, key, val)
	if err != nil {
		t.Fatalf("NewLogEntry(seq=%d) failed: %v", seq, err)
	}
	return entry
}

func mustApplyEntry(t *testing.T, replica *StateReplica, entry LogEntry, wantApplied bool) {
	t.Helper()
	applied, err := replica.Apply(entry)
	if err != nil {
		t.Fatalf("Apply(seq=%d) failed: %v", entry.Sequence, err)
	}
	if applied != wantApplied {
		t.Fatalf("Apply(seq=%d) applied=%v, want %v", entry.Sequence, applied, wantApplied)
	}
}

func TestStateReplication_ConfigAndSchemaValidation(t *testing.T) {
	badConfigs := []ReplicationConfig{
		{ClusterID: "  ", TailWindow: 2, CompactionThreshold: 4},
		{ClusterID: testClusterID, TailWindow: 0, CompactionThreshold: 4},
		{ClusterID: testClusterID, TailWindow: 5, CompactionThreshold: 3},
	}
	for i, cfg := range badConfigs {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("case %d: expected config validation error", i)
		}
	}

	validCfg := mustReplicationConfig(2, 4)
	if _, err := NewStateReplica(" ", validCfg); err == nil {
		t.Fatal("expected error for blank replica nodeID")
	}
	if _, err := NewReplicationCluster(validCfg, " "); err == nil {
		t.Fatal("expected error for blank leaderID")
	}

	if _, err := NewLogEntry(0, OpSet, testKeyAlpha, testValOne); err == nil {
		t.Fatal("expected error for sequence 0")
	}
	if _, err := NewLogEntry(1, TransitionOp("noop"), testKeyAlpha, testValOne); err == nil {
		t.Fatal("expected error for unsupported op")
	}
	if _, err := NewLogEntry(1, OpSet, "  ", testValOne); err == nil {
		t.Fatal("expected error for blank key")
	}
}

func TestStateReplication_MonotonicAndIdempotentApply(t *testing.T) {
	replica := mustNewReplica(t, testReplicaSolo, mustReplicationConfig(4, 8))

	e1 := mustNewLogEntry(t, 1, OpSet, testKeyAlpha, testValOne)
	e2 := mustNewLogEntry(t, 2, OpSet, testKeyBeta, testValTwo)
	e3 := mustNewLogEntry(t, 3, OpDelete, testKeyAlpha, "")

	mustApplyEntry(t, replica, e1, true)
	mustApplyEntry(t, replica, e1, false)
	mustApplyEntry(t, replica, e2, true)
	mustApplyEntry(t, replica, e3, true)

	if replica.AppliedSequence() != 3 {
		t.Fatalf("AppliedSequence=%d, want 3", replica.AppliedSequence())
	}
	if val, ok := replica.Get(testKeyAlpha); ok {
		t.Fatalf("expected %s deleted, got %q", testKeyAlpha, val)
	}
	if val, ok := replica.Get(testKeyBeta); !ok || val != testValTwo {
		t.Fatalf("Get(%s)=(%q, %v), want (%q, true)", testKeyBeta, val, ok, testValTwo)
	}

	conflicting := mustNewLogEntry(t, 2, OpSet, testKeyBeta, testValCorrupted)
	applied, err := replica.Apply(conflicting)
	if err == nil || applied {
		t.Fatalf("expected digest divergence error at seq 2, got applied=%v err=%v", applied, err)
	}
}

func TestStateReplication_DeterministicMerkleRoot(t *testing.T) {
	stateA := map[string]string{
		testKeyAlpha: testValOne,
		testKeyBeta:  testValTwo,
		testKeyGamma: testValThree,
	}
	stateB := map[string]string{
		testKeyGamma: testValThree,
		testKeyAlpha: testValOne,
		testKeyBeta:  testValTwo,
	}
	rootA := ComputeMerkleRoot(stateA)
	rootB := ComputeMerkleRoot(stateB)
	if rootA != rootB {
		t.Fatalf("expected identical merkle roots, got %s vs %s", rootA, rootB)
	}

	stateC := map[string]string{
		testKeyAlpha: testValOne,
		testKeyBeta:  testValCorrupted,
		testKeyGamma: testValThree,
	}
	if ComputeMerkleRoot(stateC) == rootA {
		t.Fatal("expected distinct merkle root when value changes")
	}
	if ComputeMerkleRoot(nil) == rootA {
		t.Fatal("expected non-empty state merkle root to differ from empty root")
	}
}

func mustRegisterReplica(t *testing.T, cluster *ReplicationCluster, nodeID string) *StateReplica {
	t.Helper()
	r, err := cluster.RegisterReplica(nodeID)
	if err != nil {
		t.Fatalf("RegisterReplica(%s) failed: %v", nodeID, err)
	}
	return r
}

func TestStateReplication_SlidingWindowCompactionAndCatchUp(t *testing.T) {
	cfg := mustReplicationConfig(3, 5)
	cluster := mustNewCluster(t, cfg)
	followerA := mustRegisterReplica(t, cluster, testFollowerA)

	for i := 1; i <= 8; i++ {
		if _, err := cluster.ProposeAndReplicate(OpSet, fmt.Sprintf("k-%02d", i), fmt.Sprintf("v-%02d", i)); err != nil {
			t.Fatalf("ProposeAndReplicate(%d) failed: %v", i, err)
		}
	}

	reports, err := cluster.CompactAll()
	if err != nil {
		t.Fatalf("CompactAll failed: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 compaction reports, got %d", len(reports))
	}
	if followerA.LogLen() != 3 || followerA.CompactedUpTo() != 5 {
		t.Fatalf("followerA logLen=%d compactedUpTo=%d, want 3 and 5", followerA.LogLen(), followerA.CompactedUpTo())
	}

	lagging := mustRegisterReplica(t, cluster, testFollowerB)
	lagging.MarkLagging()
	if err := cluster.CatchUpReplica(testFollowerB); err != nil {
		t.Fatalf("CatchUpReplica(%s) failed: %v", testFollowerB, err)
	}

	leader, err := cluster.GetReplica(testLeaderID)
	if err != nil {
		t.Fatalf("GetReplica(%s) failed: %v", testLeaderID, err)
	}
	if lagging.AppliedSequence() != leader.AppliedSequence() || lagging.MerkleRoot() != leader.MerkleRoot() {
		t.Fatalf("lagging replica did not converge: seq %d/%d root %s/%s",
			lagging.AppliedSequence(), leader.AppliedSequence(), lagging.MerkleRoot(), leader.MerkleRoot())
	}
}

func TestStateReplication_ConcurrentReplication(t *testing.T) {
	cfg := mustReplicationConfig(4, 16)
	cluster := mustNewCluster(t, cfg)
	followerA := mustRegisterReplica(t, cluster, testFollowerA)
	followerB := mustRegisterReplica(t, cluster, testFollowerB)

	const workers = 4
	const opsPerWorker = 10
	errCh := make(chan error, workers*opsPerWorker)

	wg := goroutinelabels.StartTestGoroutines(t, workers, "state_repl_worker", "concurrent replication", func(id int) {
		for j := 0; j < opsPerWorker; j++ {
			key := fmt.Sprintf("w%d-k%d", id, j)
			val := fmt.Sprintf("w%d-v%d", id, j)
			if _, propErr := cluster.ProposeAndReplicate(OpSet, key, val); propErr != nil {
				errCh <- propErr
			}
		}
	})
	wg.Wait()
	close(errCh)

	for propErr := range errCh {
		t.Fatalf("concurrent ProposeAndReplicate error: %v", propErr)
	}

	leader, err := cluster.GetReplica(testLeaderID)
	if err != nil {
		t.Fatalf("GetReplica(%s) failed: %v", testLeaderID, err)
	}
	wantSeq := uint64(workers * opsPerWorker)
	for _, rep := range []*StateReplica{leader, followerA, followerB} {
		if rep.AppliedSequence() != wantSeq {
			t.Fatalf("replica %s seq=%d, want %d", rep.NodeID(), rep.AppliedSequence(), wantSeq)
		}
		if rep.MerkleRoot() != leader.MerkleRoot() {
			t.Fatalf("replica %s root mismatch", rep.NodeID())
		}
	}
}

func TestStateReplication_CorruptedAndStaleSnapshotRejection(t *testing.T) {
	replica := mustNewReplica(t, testReplicaSolo, mustReplicationConfig(2, 4))
	mustApplyEntry(t, replica, mustNewLogEntry(t, 1, OpSet, testKeyAlpha, testValOne), true)
	mustApplyEntry(t, replica, mustNewLogEntry(t, 2, OpSet, testKeyBeta, testValTwo), true)

	snap, err := replica.ExportSnapshot()
	if err != nil {
		t.Fatalf("ExportSnapshot failed: %v", err)
	}

	corruptedRootSnap := snap
	corruptedRootSnap.MerkleRoot = "deadbeef"
	if err := replica.InstallSnapshot(corruptedRootSnap); err == nil {
		t.Fatal("expected error for corrupted snapshot merkle root")
	}

	corruptedStateSnap := snap
	corruptedStateSnap.State = map[string]string{testKeyAlpha: testValCorrupted}
	if err := replica.InstallSnapshot(corruptedStateSnap); err == nil {
		t.Fatal("expected error when snapshot state does not match merkle root")
	}

	staleState := map[string]string{testKeyAlpha: testValOne}
	staleSnap := MerkleSnapshot{
		SnapshotID:    testSnapID,
		Sequence:      1,
		State:         staleState,
		MerkleRoot:    ComputeMerkleRoot(staleState),
		SourceReplica: testLeaderID,
	}
	if err := replica.InstallSnapshot(staleSnap); err == nil {
		t.Fatal("expected error for stale snapshot sequence")
	}
}

func TestStateReplication_DivergenceDetectionAndQuarantine(t *testing.T) {
	cluster := mustNewCluster(t, mustReplicationConfig(2, 4))
	diverging, err := cluster.RegisterReplica(testFollowerA)
	if err != nil {
		t.Fatalf("RegisterReplica(%s) failed: %v", testFollowerA, err)
	}

	evilEntry := mustNewLogEntry(t, 1, OpSet, testKeyAlpha, testValCorrupted)
	mustApplyEntry(t, diverging, evilEntry, true)

	goodEntry := mustNewLogEntry(t, 1, OpSet, testKeyAlpha, testValOne)
	leader, err := cluster.GetReplica(testLeaderID)
	if err != nil {
		t.Fatalf("GetReplica(%s) failed: %v", testLeaderID, err)
	}
	mustApplyEntry(t, leader, goodEntry, true)

	reports := cluster.VerifyConsistency()
	if len(reports) != 1 || reports[0].ReplicaID != testFollowerA {
		t.Fatalf("expected 1 divergence report for %s, got %+v", testFollowerA, reports)
	}
	if diverging.Status() != ReplicaQuarantined {
		t.Fatalf("expected status %s, got %s", ReplicaQuarantined, diverging.Status())
	}
	if len(cluster.QuarantinedReports()) != 1 {
		t.Fatalf("expected 1 quarantined report in cluster")
	}
	if _, err := diverging.Apply(mustNewLogEntry(t, 2, OpSet, testKeyBeta, testValTwo)); err == nil {
		t.Fatal("expected quarantined replica to reject Apply")
	}
}

func TestStateReplication_GapAndCompactedRangeRejection(t *testing.T) {
	replica := mustNewReplica(t, testReplicaSolo, mustReplicationConfig(2, 3))
	if _, err := replica.Apply(mustNewLogEntry(t, 2, OpSet, testKeyAlpha, testValOne)); err == nil {
		t.Fatal("expected error for gap sequence 2 when appliedSeq is 0")
	}

	for i := uint64(1); i <= 4; i++ {
		mustApplyEntry(t, replica, mustNewLogEntry(t, i, OpSet, fmt.Sprintf("k-%d", i), testValOne), true)
	}
	report, err := replica.Compact()
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	if report.CompactedUpTo != 2 {
		t.Fatalf("CompactedUpTo=%d, want 2", report.CompactedUpTo)
	}
	if _, err := replica.Apply(mustNewLogEntry(t, 2, OpSet, "k-2", testValOne)); err == nil {
		t.Fatal("expected error for sequence within compacted range")
	}
}
