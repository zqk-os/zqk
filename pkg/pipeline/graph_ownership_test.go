package pipeline

import (
	"testing"
)

func TestTypedGraph_OwnershipLifecycle(t *testing.T) {
	tg := NewTypedGraph()

	ownerA := Owner{Type: "backlog_item", SubjectID: "BLI-101"}
	ownerB := Owner{Type: "backlog_item", SubjectID: "BLI-102"}
	ownerC := Owner{Type: "requirement", SubjectID: "REQ-201"}

	// Initially empty
	if tg.Owns(ownerA.Type, ownerA.SubjectID) {
		t.Fatalf("expected unowned initially")
	}

	// Claim A and C
	tg.Claim(ownerA)
	tg.Claim(ownerC)

	if !tg.Owns(ownerA.Type, ownerA.SubjectID) {
		t.Errorf("expected ownerA to be claimed")
	}
	if !tg.Owns(ownerC.Type, ownerC.SubjectID) {
		t.Errorf("expected ownerC to be claimed")
	}
	if tg.Owns(ownerB.Type, ownerB.SubjectID) {
		t.Errorf("ownerB was not claimed")
	}

	// Invalidate A
	tg.Invalidate(ownerA)
	if tg.Owns(ownerA.Type, ownerA.SubjectID) {
		t.Errorf("expected ownerA to be invalidated")
	}
	if !tg.Owns(ownerC.Type, ownerC.SubjectID) {
		t.Errorf("ownerC should remain claimed")
	}
}
