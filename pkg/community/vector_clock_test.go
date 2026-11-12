package community

import (
	"testing"
)

// TestVectorClock_StaticFloorInvariant verifies CRIT-1794505640259509000-8316a038:
// static floor invariants, blank validation, clock initialization, serialization round-trip, and partial ordering.
func TestVectorClock_StaticFloorInvariant(t *testing.T) {
	vc := NewVectorClock()
	if vc == nil {
		t.Fatal("expected non-nil vector clock")
	}

	// 1. Rejects blank node ID on Tick and Set
	if err := vc.Tick(""); err == nil {
		t.Error("expected error ticking blank node ID")
	}
	if err := vc.Tick("   "); err == nil {
		t.Error("expected error ticking whitespace node ID")
	}
	if err := vc.Set("", 5); err == nil {
		t.Error("expected error setting blank node ID")
	}

	// 2. Monotonic increments
	if err := vc.Tick("node-1"); err != nil {
		t.Fatalf("unexpected error ticking node-1: %v", err)
	}
	if val := vc.Get("node-1"); val != 1 {
		t.Errorf("expected node-1 clock 1, got %d", val)
	}
	if err := vc.Tick("node-1"); err != nil {
		t.Fatalf("unexpected error ticking node-1 again: %v", err)
	}
	if val := vc.Get("node-1"); val != 2 {
		t.Errorf("expected node-1 clock 2, got %d", val)
	}

	// 3. Regressive Set rejection
	if err := vc.Set("node-1", 1); err == nil {
		t.Error("expected error attempting to regress clock from 2 to 1")
	}

	// 4. Serialization roundtrip
	data, err := vc.Serialize()
	if err != nil {
		t.Fatalf("unexpected error serializing vector clock: %v", err)
	}
	restored, err := DeserializeVectorClock(data)
	if err != nil {
		t.Fatalf("unexpected error deserializing vector clock: %v", err)
	}
	if restored.Get("node-1") != 2 {
		t.Errorf("expected restored clock to have 2 for node-1, got %d", restored.Get("node-1"))
	}

	// 5. Identity equality
	if rel := vc.Compare(restored); rel != RelationEqual {
		t.Errorf("expected identical clocks to compare RelationEqual, got %s", rel)
	}
}

// TestVectorClock_OperationalDynamicProof verifies CRIT-1794505646342295000-fabbb0a4:
// concurrent shockwave settlement, monotonic convergence, and causal progression across nodes.
func TestVectorClock_OperationalDynamicProof(t *testing.T) {
	settler, err := NewShockwaveSettler("leader-node", 500)
	if err != nil {
		t.Fatalf("unexpected error creating settler: %v", err)
	}

	c1 := NewVectorClock()
	_ = c1.Tick("node-A")

	// 1. Initial shockwave event accepted
	res1, err := settler.Settle(ShockwaveEvent{
		EventID:     "evt-1",
		OriginNode:  "node-A",
		EntityID:    "PRI-CYCLE-140",
		TargetState: "in_progress",
		Clock:       c1,
	})
	if err != nil {
		t.Fatalf("unexpected error settling evt-1: %v", err)
	}
	if res1.Action != ActionAccepted || res1.FinalState != "in_progress" {
		t.Errorf("expected evt-1 accepted with in_progress, got action=%s state=%s", res1.Action, res1.FinalState)
	}

	// 2. Causal successor from node-A accepted
	c2 := c1.Clone()
	_ = c2.Tick("node-A")
	res2, err := settler.Settle(ShockwaveEvent{
		EventID:     "evt-2",
		OriginNode:  "node-A",
		EntityID:    "PRI-CYCLE-140",
		TargetState: "complete",
		Clock:       c2,
	})
	if err != nil {
		t.Fatalf("unexpected error settling evt-2: %v", err)
	}
	if res2.Action != ActionAccepted || res2.FinalState != "complete" {
		t.Errorf("expected evt-2 accepted with complete, got action=%s state=%s", res2.Action, res2.FinalState)
	}

	// 3. Concurrent shockwaves from node-B merged and settled deterministically
	cB := c1.Clone() // branched from c1, concurrent with c2
	_ = cB.Tick("node-B")
	res3, err := settler.Settle(ShockwaveEvent{
		EventID:     "evt-3",
		OriginNode:  "node-B",
		EntityID:    "PRI-CYCLE-140",
		TargetState: "halted",
		Clock:       cB,
	})
	if err != nil {
		t.Fatalf("unexpected error settling concurrent evt-3: %v", err)
	}
	if res3.Action != ActionConflict {
		t.Errorf("expected concurrent conflict resolution, got %s", res3.Action)
	}

	// Verify settled state has merged vector clock
	st, exists := settler.GetEntity("PRI-CYCLE-140")
	if !exists {
		t.Fatal("expected entity PRI-CYCLE-140 to exist")
	}
	if st.Clock.Get("node-A") != 2 || st.Clock.Get("node-B") != 1 {
		t.Errorf("expected merged vector clock [node-A:2, node-B:1], got %v", st.Clock.Map())
	}
}

// TestVectorClock_NegativeAdversarialBoundary verifies CRIT-1794505651569453000-33bbe267:
// stale, duplicate, or clock-skewed shockwave events are safely detected and fenced without corrupting state.
func TestVectorClock_NegativeAdversarialBoundary(t *testing.T) {
	settler, err := NewShockwaveSettler("leader-node", 100)
	if err != nil {
		t.Fatalf("unexpected error creating settler: %v", err)
	}

	// 1. Reject invalid constructor
	if _, err := NewShockwaveSettler("", 100); err == nil {
		t.Error("expected error creating settler with blank localNodeID")
	}

	// 2. Reject nil event clock
	if _, err := settler.Settle(ShockwaveEvent{
		EventID:     "evt-bad",
		OriginNode:  "node-A",
		EntityID:    "E-1",
		TargetState: "active",
		Clock:       nil,
	}); err == nil {
		t.Error("expected error settling event with nil clock")
	}

	// 3. Reject excessive clock skew on initial event
	skewedClock := NewVectorClock()
	_ = skewedClock.Set("bad-node", 9999)
	if _, err := settler.Settle(ShockwaveEvent{
		EventID:     "evt-skew",
		OriginNode:  "bad-node",
		EntityID:    "E-SKEW",
		TargetState: "active",
		Clock:       skewedClock,
	}); err == nil {
		t.Error("expected error settling event with excessive clock skew")
	}

	// 4. Initial valid state
	validClock := NewVectorClock()
	_ = validClock.Set("node-A", 10)
	_, _ = settler.Settle(ShockwaveEvent{
		EventID:     "evt-valid",
		OriginNode:  "node-A",
		EntityID:    "E-1",
		TargetState: "active",
		Clock:       validClock,
	})

	// 5. Stale event (clock regression) dropped
	staleClock := NewVectorClock()
	_ = staleClock.Set("node-A", 5)
	resStale, err := settler.Settle(ShockwaveEvent{
		EventID:     "evt-stale",
		OriginNode:  "node-A",
		EntityID:    "E-1",
		TargetState: "obsolete",
		Clock:       staleClock,
	})
	if err != nil {
		t.Fatalf("unexpected error settling stale event: %v", err)
	}
	if resStale.Action != ActionDropped || resStale.Relation != RelationBefore {
		t.Errorf("expected stale event dropped with RelationBefore, got %s / %s", resStale.Action, resStale.Relation)
	}

	// 6. Duplicate event dropped
	resDup, err := settler.Settle(ShockwaveEvent{
		EventID:     "evt-dup",
		OriginNode:  "node-A",
		EntityID:    "E-1",
		TargetState: "active",
		Clock:       validClock,
	})
	if err != nil {
		t.Fatalf("unexpected error settling dup event: %v", err)
	}
	if resDup.Action != ActionDropped || resDup.Relation != RelationEqual {
		t.Errorf("expected duplicate event dropped with RelationEqual, got %s / %s", resDup.Action, resDup.Relation)
	}

	// Confirm entity state remained untampered
	st, _ := settler.GetEntity("E-1")
	if st.State != "active" {
		t.Errorf("expected entity state active, got %s", st.State)
	}
}
