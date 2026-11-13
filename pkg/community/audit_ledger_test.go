package community

import (
	"strings"
	"testing"
	"time"
)

// TestAuditLedger_StaticFloorInvariant verifies schema initialization and blank error guards.
func TestAuditLedger_StaticFloorInvariant(t *testing.T) {
	ledger := NewAuditLedger()
	if count := ledger.BlockCount(); count != 0 {
		t.Fatalf("expected initial block count 0, got %d", count)
	}

	// Empty block rejected
	_, err := ledger.AppendBlock([]AuditEntry{})
	if err == nil {
		t.Fatalf("expected error appending empty block, got nil")
	}

	// Blank entry validation
	invalidEntries := []AuditEntry{
		{
			EntryID: "   ",
			ActorID: "agent-01",
			Action:  "mutate",
			Payload: "{}",
		},
	}
	_, err = ledger.AppendBlock(invalidEntries)
	if err == nil {
		t.Fatalf("expected error on blank entry ID, got nil")
	}
}

// TestAuditLedger_OperationalDynamicProof verifies sequential hash chaining and valid inclusion proofs.
func TestAuditLedger_OperationalDynamicProof(t *testing.T) {
	ledger := NewAuditLedger()

	entries1 := []AuditEntry{
		{EntryID: "evt-001", ActorID: "agent-alpha", Action: "lock:acquire", Payload: "resource:db", Timestamp: time.Now().UTC()},
		{EntryID: "evt-002", ActorID: "agent-beta", Action: "object:promote", Payload: "BLI-101", Timestamp: time.Now().UTC()},
		{EntryID: "evt-003", ActorID: "agent-gamma", Action: "gossip:sync", Payload: "cluster:west", Timestamp: time.Now().UTC()},
	}

	blk0, err := ledger.AppendBlock(entries1)
	if err != nil {
		t.Fatalf("unexpected error appending block 0: %v", err)
	}

	if blk0.Index != 0 {
		t.Fatalf("expected block 0 index 0, got %d", blk0.Index)
	}
	if blk0.MerkleRoot == "" {
		t.Fatalf("expected non-empty Merkle root for block 0")
	}

	entries2 := []AuditEntry{
		{EntryID: "evt-004", ActorID: "agent-alpha", Action: "lock:release", Payload: "resource:db", Timestamp: time.Now().UTC()},
		{EntryID: "evt-005", ActorID: "agent-delta", Action: "test:execute", Payload: "TST-500", Timestamp: time.Now().UTC()},
	}

	blk1, err := ledger.AppendBlock(entries2)
	if err != nil {
		t.Fatalf("unexpected error appending block 1: %v", err)
	}

	if blk1.Index != 1 {
		t.Fatalf("expected block 1 index 1, got %d", blk1.Index)
	}
	if blk1.PreviousHash == "" {
		t.Fatalf("expected non-empty previous hash linking to block 0")
	}

	// Verify Merkle inclusion proof for entry 1 in block 0
	proof, err := ledger.GenerateInclusionProof(0, 1)
	if err != nil {
		t.Fatalf("unexpected error generating inclusion proof: %v", err)
	}

	valid, err := VerifyInclusionProof(*proof)
	if err != nil || !valid {
		t.Fatalf("expected valid proof verification, got valid=%v, err=%v", valid, err)
	}
}

// TestAuditLedger_NegativeAdversarialBoundary verifies rejection of tampered blocks and forged proofs.
func TestAuditLedger_NegativeAdversarialBoundary(t *testing.T) {
	ledger := NewAuditLedger()

	entries := []AuditEntry{
		{EntryID: "evt-alpha", ActorID: "actor-01", Action: "transfer", Payload: "100", Timestamp: time.Now().UTC()},
		{EntryID: "evt-beta", ActorID: "actor-02", Action: "transfer", Payload: "200", Timestamp: time.Now().UTC()},
		{EntryID: "evt-gamma", ActorID: "actor-03", Action: "transfer", Payload: "300", Timestamp: time.Now().UTC()},
		{EntryID: "evt-delta", ActorID: "actor-04", Action: "transfer", Payload: "400", Timestamp: time.Now().UTC()},
	}

	_, err := ledger.AppendBlock(entries)
	if err != nil {
		t.Fatalf("unexpected error appending block: %v", err)
	}

	proof, err := ledger.GenerateInclusionProof(0, 2)
	if err != nil {
		t.Fatalf("unexpected error generating proof: %v", err)
	}

	// Case 1: Tampered Entry Hash
	tamperedProof := *proof
	tamperedProof.EntryHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // forged hash
	valid, err := VerifyInclusionProof(tamperedProof)
	if valid || err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected tamper error, got valid=%v, err=%v", valid, err)
	}

	// Case 2: Tampered Path Element
	corruptPathProof := *proof
	corruptPathProof.AuditPath = make([]AuditPathElement, len(proof.AuditPath))
	copy(corruptPathProof.AuditPath, proof.AuditPath)
	corruptPathProof.AuditPath[0].Hash = "deadbeefcafebabe12345678"
	valid, err = VerifyInclusionProof(corruptPathProof)
	if valid || err == nil {
		t.Fatalf("expected corrupted path verification error, got valid=%v, err=%v", valid, err)
	}

	// Case 3: Out-of-bounds index proof request
	_, err = ledger.GenerateInclusionProof(0, 99)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out of bounds error, got %v", err)
	}
}
