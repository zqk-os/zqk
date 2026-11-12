package community

import (
	"errors"
	"sort"
	"testing"
)

// TestCRDT_StaticFloorInvariant verifies CRIT-1794506482224793000-2ab4e1d6.
func TestCRDT_StaticFloorInvariant(t *testing.T) {
	doc, err := NewCRDTDocument("doc-cluster-alpha")
	if err != nil {
		t.Fatalf("expected NewCRDTDocument to succeed, got: %v", err)
	}
	if doc.ID() != "doc-cluster-alpha" {
		t.Errorf("expected doc ID doc-cluster-alpha, got: %s", doc.ID())
	}

	err = doc.SetRegister("cluster_state", "active", 100, "node-1")
	if err != nil {
		t.Fatalf("expected SetRegister to succeed, got: %v", err)
	}

	reg, ok := doc.GetRegister("cluster_state")
	if !ok {
		t.Fatalf("expected register cluster_state to exist")
	}
	if reg.Value != "active" || reg.Timestamp != 100 || reg.WriterID != "node-1" {
		t.Errorf("unexpected register value: %+v", reg)
	}

	err = doc.AddElement("task-101", 100, "node-1")
	if err != nil {
		t.Fatalf("expected AddElement to succeed, got: %v", err)
	}
	if !doc.HasElement("task-101") {
		t.Errorf("expected task-101 to be present in document")
	}
}

// TestCRDT_OperationalDynamicProof verifies CRIT-1794506489562327000-e050dc64.
func TestCRDT_OperationalDynamicProof(t *testing.T) {
	// Simulate two divergent cluster replicas A and B
	docA, err := NewCRDTDocument("doc-sync-mesh")
	if err != nil {
		t.Fatalf("failed creating docA: %v", err)
	}
	docB, err := NewCRDTDocument("doc-sync-mesh")
	if err != nil {
		t.Fatalf("failed creating docB: %v", err)
	}

	// Cluster A performs operations
	_ = docA.SetRegister("leader", "node-A", 150, "node-A")
	_ = docA.AddElement("service-storage", 100, "node-A")
	_ = docA.AddElement("service-gateway", 120, "node-A")

	// Cluster B performs concurrent operations
	_ = docB.SetRegister("leader", "node-B", 200, "node-B") // higher timestamp wins
	_ = docB.AddElement("service-storage", 110, "node-B")
	_ = docB.RemoveElement("service-gateway", 130, "node-B") // removal timestamp > add timestamp
	_ = docB.AddElement("service-telemetry", 140, "node-B")

	// Create clones to verify commutativity: Merge(A, B) vs Merge(B, A)
	docMergedAB, _ := NewCRDTDocument("doc-sync-mesh")
	_ = docMergedAB.Merge(docA)
	_ = docMergedAB.Merge(docB)

	docMergedBA, _ := NewCRDTDocument("doc-sync-mesh")
	_ = docMergedBA.Merge(docB)
	_ = docMergedBA.Merge(docA)

	// Invariant: Both must deterministically converge to identical state
	regAB, _ := docMergedAB.GetRegister("leader")
	regBA, _ := docMergedBA.GetRegister("leader")
	if regAB.Value != "node-B" || regBA.Value != "node-B" {
		t.Errorf("expected leader to be node-B, got AB=%s, BA=%s", regAB.Value, regBA.Value)
	}

	elemsAB := docMergedAB.Elements()
	elemsBA := docMergedBA.Elements()
	sort.Strings(elemsAB)
	sort.Strings(elemsBA)

	if len(elemsAB) != len(elemsBA) {
		t.Fatalf("element count mismatch: AB=%v, BA=%v", elemsAB, elemsBA)
	}
	for i := range elemsAB {
		if elemsAB[i] != elemsBA[i] {
			t.Errorf("element mismatch at %d: %s vs %s", i, elemsAB[i], elemsBA[i])
		}
	}

	// Verify gateway was removed (t=130 remove > t=120 add)
	if docMergedAB.HasElement("service-gateway") {
		t.Errorf("expected service-gateway to be removed by higher timestamp tombstone")
	}
	// Verify storage and telemetry remain
	if !docMergedAB.HasElement("service-storage") || !docMergedAB.HasElement("service-telemetry") {
		t.Errorf("expected service-storage and service-telemetry to be present")
	}

	// Test incremental Delta sync
	delta := &CRDTDelta{
		DocumentID: "doc-sync-mesh",
		Registers: map[string]LWWRegister{
			"epoch": {Value: "42", Timestamp: 300, WriterID: "node-B"},
		},
		SenderID:  "node-B",
		Timestamp: 300,
	}
	if err := docMergedAB.ApplyDelta(delta); err != nil {
		t.Fatalf("failed applying delta: %v", err)
	}
	epochReg, ok := docMergedAB.GetRegister("epoch")
	if !ok || epochReg.Value != "42" {
		t.Errorf("expected epoch register to be 42, got %+v", epochReg)
	}
}

// TestCRDT_NegativeAdversarialBoundary verifies CRIT-1794506496249715000-25164b8d.
func TestCRDT_NegativeAdversarialBoundary(t *testing.T) {
	// Blank document ID rejection
	_, err := NewCRDTDocument("   ")
	if !errors.Is(err, ErrBlankID) {
		t.Errorf("expected ErrBlankID for empty document id, got: %v", err)
	}

	doc, err := NewCRDTDocument("doc-boundary")
	if err != nil {
		t.Fatalf("failed creating doc: %v", err)
	}

	// Blank register key rejection
	err = doc.SetRegister("", "value", 100, "node-1")
	if !errors.Is(err, ErrBlankID) {
		t.Errorf("expected ErrBlankID for blank register key, got: %v", err)
	}

	// Invalid timestamp rejection
	err = doc.SetRegister("key", "value", -5, "node-1")
	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf("expected ErrInvalidTimestamp for negative timestamp, got: %v", err)
	}

	// Nil delta rejection
	err = doc.ApplyDelta(nil)
	if !errors.Is(err, ErrNilDelta) {
		t.Errorf("expected ErrNilDelta for nil delta, got: %v", err)
	}

	// Delta with mismatched document ID rejection
	mismatchedDelta := &CRDTDelta{
		DocumentID: "doc-other",
		SenderID:   "node-1",
		Timestamp:  100,
	}
	err = doc.ApplyDelta(mismatchedDelta)
	if !errors.Is(err, ErrCorruptPayload) {
		t.Errorf("expected ErrCorruptPayload for mismatched doc ID, got: %v", err)
	}

	// Merging document with different ID rejection
	foreignDoc, _ := NewCRDTDocument("doc-foreign")
	err = doc.Merge(foreignDoc)
	if !errors.Is(err, ErrCorruptPayload) {
		t.Errorf("expected ErrCorruptPayload when merging different doc IDs, got: %v", err)
	}
}
