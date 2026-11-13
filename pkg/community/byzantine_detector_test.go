package community

import (
	"strings"
	"testing"
	"time"
)

// TestByzantineDetector_StaticFloorInvariant verifies state machine definitions and valid initialization.
func TestByzantineDetector_StaticFloorInvariant(t *testing.T) {
	// Verify zero-threshold rejection
	_, err := NewByzantineDetector(0)
	if err == nil {
		t.Fatalf("expected error for witnessThreshold=0, got nil")
	}

	detector, err := NewByzantineDetector(2)
	if err != nil {
		t.Fatalf("unexpected error creating detector: %v", err)
	}

	if detector.witnessThreshold != 2 {
		t.Fatalf("expected witness threshold 2, got %d", detector.witnessThreshold)
	}

	// Register healthy node
	if err := detector.RegisterNode("agent-node-01"); err != nil {
		t.Fatalf("unexpected error registering node: %v", err)
	}

	if state := detector.GetNodeState("agent-node-01"); state != NodeStateHealthy {
		t.Fatalf("expected state %s, got %s", NodeStateHealthy, state)
	}

	// Blank registration rejection
	if err := detector.RegisterNode("   "); err == nil {
		t.Fatalf("expected error on blank node ID, got nil")
	}
}

// TestByzantineDetector_OperationalDynamicProof verifies detection of equivocation and quarantine propagation.
func TestByzantineDetector_OperationalDynamicProof(t *testing.T) {
	detector, err := NewByzantineDetector(2)
	if err != nil {
		t.Fatalf("unexpected error creating detector: %v", err)
	}

	nodeID := "malicious-agent-node-09"
	_ = detector.RegisterNode(nodeID)

	// First valid assertion
	a1 := SignedAssertion{
		NodeID:    nodeID,
		View:      1,
		Sequence:  42,
		Digest:    "hash_commit_alpha",
		Signature: "sig_node09_a",
		Timestamp: time.Now().UTC(),
	}

	notice, err := detector.ProcessAssertion(a1)
	if err != nil {
		t.Fatalf("unexpected error processing assertion 1: %v", err)
	}
	if notice != nil {
		t.Fatalf("expected no quarantine notice for first assertion, got %+v", notice)
	}

	if state := detector.GetNodeState(nodeID); state != NodeStateHealthy {
		t.Fatalf("expected node to remain healthy, got %s", state)
	}

	// Second conflicting assertion for identical sequence and view (Equivocation!)
	a2 := SignedAssertion{
		NodeID:    nodeID,
		View:      1,
		Sequence:  42,
		Digest:    "hash_commit_beta", // divergent payload digest
		Signature: "sig_node09_b",
		Timestamp: time.Now().UTC(),
	}

	notice, err = detector.ProcessAssertion(a2)
	if err != nil {
		t.Fatalf("unexpected error processing equivocation: %v", err)
	}
	if notice == nil {
		t.Fatalf("expected quarantine notice for equivocation, got nil")
	}

	if notice.Reason != FaultEquivocation {
		t.Fatalf("expected reason %s, got %s", FaultEquivocation, notice.Reason)
	}
	if notice.NewState != NodeStateQuarantined {
		t.Fatalf("expected new state %s, got %s", NodeStateQuarantined, notice.NewState)
	}

	if state := detector.GetNodeState(nodeID); state != NodeStateQuarantined {
		t.Fatalf("expected node state to be quarantined, got %s", state)
	}

	history := detector.ExportQuarantineNotices()
	if len(history) != 1 {
		t.Fatalf("expected 1 quarantine notice in history, got %d", len(history))
	}
}

// TestByzantineDetector_NegativeAdversarialBoundary verifies rejection of false accusations and spoofed evidence.
func TestByzantineDetector_NegativeAdversarialBoundary(t *testing.T) {
	detector, err := NewByzantineDetector(2)
	if err != nil {
		t.Fatalf("unexpected error creating detector: %v", err)
	}

	targetNode := "honest-node-03"
	_ = detector.RegisterNode(targetNode)

	// Case 1: Self-reporting accusation rejected
	selfEvidence := FaultEvidence{
		FaultType:         FaultEquivocation,
		AccusedNode:       targetNode,
		ReporterNode:      targetNode,
		WitnessSignatures: []string{"w1", "w2"},
		AssertionA:        SignedAssertion{Digest: "d1", Signature: "s1"},
		AssertionB:        SignedAssertion{Digest: "d2", Signature: "s2"},
	}
	if _, err := detector.ReportFault(selfEvidence); err == nil || !strings.Contains(err.Error(), "self-reporting") {
		t.Fatalf("expected self-reporting error, got: %v", err)
	}

	// Case 2: Insufficient witness signatures rejected (only 1 witness provided when 2 required)
	lowWitnessEvidence := FaultEvidence{
		FaultType:         FaultEquivocation,
		AccusedNode:       targetNode,
		ReporterNode:      "rival-node-07",
		WitnessSignatures: []string{"w1"},
		AssertionA:        SignedAssertion{Digest: "d1", Signature: "s1"},
		AssertionB:        SignedAssertion{Digest: "d2", Signature: "s2"},
	}
	if _, err := detector.ReportFault(lowWitnessEvidence); err == nil || !strings.Contains(err.Error(), "insufficient witness") {
		t.Fatalf("expected insufficient witness error, got: %v", err)
	}

	// Case 3: Bogus equivocation evidence where assertions are identical
	identicalEvidence := FaultEvidence{
		FaultType:         FaultEquivocation,
		AccusedNode:       targetNode,
		ReporterNode:      "rival-node-07",
		WitnessSignatures: []string{"w1", "w2"},
		AssertionA:        SignedAssertion{Digest: "same_digest", Signature: "same_sig"},
		AssertionB:        SignedAssertion{Digest: "same_digest", Signature: "same_sig"},
	}
	if _, err := detector.ReportFault(identicalEvidence); err == nil || !strings.Contains(err.Error(), "assertions are identical") {
		t.Fatalf("expected error for identical assertions in equivocation, got: %v", err)
	}

	// Ensure target node remains unpenalized and healthy
	if state := detector.GetNodeState(targetNode); state != NodeStateHealthy {
		t.Fatalf("expected honest node to remain healthy despite rejected accusations, got %s", state)
	}
}
