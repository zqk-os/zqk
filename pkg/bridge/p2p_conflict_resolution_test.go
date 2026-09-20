package bridge

import (
	"reflect"
	"testing"
)

func TestVectorClock_Compare(t *testing.T) {
	vc1 := VectorClock{"A": 1, "B": 2}
	vc2 := VectorClock{"A": 1, "B": 2}

	if comp := vc1.Compare(vc2); comp != 0 {
		t.Errorf("Expected 0 (equal), got %d", comp)
	}

	vc3 := VectorClock{"A": 2, "B": 2}
	if comp := vc1.Compare(vc3); comp != -1 {
		t.Errorf("Expected -1 (vc1 < vc3), got %d", comp)
	}
	if comp := vc3.Compare(vc1); comp != 1 {
		t.Errorf("Expected 1 (vc3 > vc1), got %d", comp)
	}

	vc4 := VectorClock{"A": 1, "B": 3}
	if comp := vc3.Compare(vc4); comp != 2 {
		t.Errorf("Expected 2 (concurrent), got %d", comp)
	}
}

func TestVectorClock_Merge(t *testing.T) {
	vc1 := VectorClock{"A": 2, "B": 1}
	vc2 := VectorClock{"A": 1, "B": 3, "C": 1}

	merged := vc1.Merge(vc2)

	expected := VectorClock{"A": 2, "B": 3, "C": 1}
	if !reflect.DeepEqual(merged, expected) {
		t.Errorf("Expected %v, got %v", expected, merged)
	}
}

func TestVectorClock_Increment(t *testing.T) {
	vc := VectorClock{"A": 1}
	vc.Increment("A")
	vc.Increment("B")

	expected := VectorClock{"A": 2, "B": 1}
	if !reflect.DeepEqual(vc, expected) {
		t.Errorf("Expected %v, got %v", expected, vc)
	}
}

func TestConflictResolver_ResolveState(t *testing.T) {
	resolver := NewConflictResolver("NodeA")

	localVC := VectorClock{"NodeA": 2, "NodeB": 1}
	remoteVC := VectorClock{"NodeA": 2, "NodeB": 2} // Remote is strictly newer

	localState := map[string]any{"key1": "local_val"}
	remoteState := map[string]any{"key1": "remote_val"}

	// Test 1: Remote is newer
	resolvedState, resolvedVC, needsUpdate, err := resolver.ResolveState(localState, localVC, remoteState, remoteVC)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !needsUpdate {
		t.Errorf("Expected needsUpdate to be true")
	}
	if resolvedState["key1"] != "remote_val" {
		t.Errorf("Expected resolvedState to have 'remote_val', got %v", resolvedState["key1"])
	}
	if resolvedVC["NodeB"] != 2 {
		t.Errorf("Expected resolvedVC['NodeB'] to be 2, got %v", resolvedVC["NodeB"])
	}

	// Test 2: Local is newer
	localVC = VectorClock{"NodeA": 3, "NodeB": 1}
	remoteVC = VectorClock{"NodeA": 2, "NodeB": 1}
	resolvedState, resolvedVC, needsUpdate, err = resolver.ResolveState(localState, localVC, remoteState, remoteVC)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if needsUpdate {
		t.Errorf("Expected needsUpdate to be false")
	}
	if resolvedState["key1"] != "local_val" {
		t.Errorf("Expected resolvedState to have 'local_val', got %v", resolvedState["key1"])
	}
	if resolvedVC["NodeA"] != 3 {
		t.Errorf("Expected resolvedVC['NodeA'] to be 3, got %v", resolvedVC["NodeA"])
	}

	// Test 3: Concurrent (Conflict)
	localVC = VectorClock{"NodeA": 2, "NodeB": 1}
	remoteVC = VectorClock{"NodeA": 1, "NodeB": 2}
	localState = map[string]any{"keyA": "a", "keyB": "b1"}
	remoteState = map[string]any{"keyB": "b2", "keyC": "c"}

	resolvedState, resolvedVC, needsUpdate, err = resolver.ResolveState(localState, localVC, remoteState, remoteVC)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !needsUpdate {
		t.Errorf("Expected needsUpdate to be true for concurrent merge")
	}
	// "b2" > "b1", so keyB should be "b2"
	if resolvedState["keyB"] != "b2" {
		t.Errorf("Expected keyB to be 'b2', got %v", resolvedState["keyB"])
	}
	if resolvedState["keyA"] != "a" {
		t.Errorf("Expected keyA to be 'a', got %v", resolvedState["keyA"])
	}
	if resolvedState["keyC"] != "c" {
		t.Errorf("Expected keyC to be 'c', got %v", resolvedState["keyC"])
	}
	// Our NodeA should be incremented since we resolved a conflict
	if resolvedVC["NodeA"] != 3 {
		t.Errorf("Expected resolvedVC['NodeA'] to be 3, got %v", resolvedVC["NodeA"])
	}
}
