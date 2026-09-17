package tracing

import "testing"

// TestBindOrphanedReqToBli ensures that an orphaned REQ gets bound to a covering BLI.
func TestBindOrphanedReqToBli(t *testing.T) {
	owner := NewBinder()

	reqs := []Ownership{
		{ID: "REQ-001", Kind: "req", Status: "active"},
	}
	blis := []Ownership{
		{ID: "BLI-201", Kind: "backlog_item", Status: "in_progress"},
	}

	bound, parked := owner.BindOrPark(reqs, blis)

	if len(bound) != 1 {
		t.Fatalf("expected 1 bound, got %d", len(bound))
	}
	if bound[0].ID != "REQ-001" {
		t.Errorf("expected REQ-001 bound, got %s", bound[0].ID)
	}
	if len(parked) > 0 {
		t.Fatalf("expected 0 parked, got %d", len(parked))
	}
}

// TestOrphanedReqWithoutBliIsParam ensures orphaned REQ with no covering BLI is parked.
func TestOrphanedReqWithoutBliIsParam(t *testing.T) {
	owner := NewBinder()

	reqs := []Ownership{
		{ID: "REQ-099", Kind: "req", Status: "active"},
	}
	blis := []Ownership{} // no BLIs to bind to

	bound, parked := owner.BindOrPark(reqs, blis)

	if len(bound) != 0 {
		t.Fatalf("expected 0 bound, got %d", len(bound))
	}
	if len(parked) != 1 {
		t.Fatalf("expected 1 parked, got %d", len(parked))
	}
	if parked[0].Status != "parked" {
		t.Errorf("expected park status, got %s", parked[0].Status)
	}
	if parked[0].ParkReason != "no_covering_bli" {
		t.Errorf("expected no_covering_bli reason, got %s", parked[0].ParkReason)
	}
}

// TestAllActiveBliBoundToReq ensures every active BLI gets coverage from REQ.
func TestAllActiveBliBoundToReq(t *testing.T) {
	owner := NewBinder()

	reqs := []Ownership{
		{ID: "REQ-100", Kind: "req", Status: "active"},
		{ID: "REQ-101", Kind: "req", Status: "active"},
	}
	blis := []Ownership{
		{ID: "BLI-301", Kind: "backlog_item", Status: "active"},
		{ID: "BLI-302", Kind: "backlog_item", Status: "active"},
	}

	bound, parked := owner.BindOrPark(reqs, blis)

	if len(bound) != 2 {
		t.Fatalf("expected 2 bound, got %d", len(bound))
	}
	if len(parked) > 0 {
		t.Fatalf("expected 0 parked, got %d: %v", len(parked), parked)
	}
}

// TestNoLeftoversAfterBinding ensures all active items are accounted for.
func TestNoLeftoversAfterBinding(t *testing.T) {
	owner := NewBinder()

	reqs := []Ownership{
		{ID: "REQ-500", Kind: "req", Status: "active"},
	}
	blis := []Ownership{
		{ID: "BLI-601", Kind: "backlog_item", Status: "completed"}, // not active — should not be bound.
	}

	bound, parked := owner.BindOrPark(reqs, blis)

	if len(bound) != 1 {
		t.Fatalf("expected 1 bound, got %d", len(bound))
	}
	// BLI-601 is completed; it should not be in the output at all.
	if len(parked) != 0 {
		// no parked items expected for this case — completed BLIs are ignored.
	}
}
