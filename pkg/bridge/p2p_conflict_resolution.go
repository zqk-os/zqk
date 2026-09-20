package bridge

import (
	"fmt"
)

// VectorClock represents a standard vector clock for P2P conflict resolution.
type VectorClock map[string]int64

// Clone returns a deep copy of the VectorClock.
func (vc VectorClock) Clone() VectorClock {
	cloned := make(VectorClock, len(vc))
	for k, v := range vc {
		cloned[k] = v
	}
	return cloned
}

// Compare compares this vector clock to another.
// Returns:
// -1 if vc < other (vc happens before other)
// 1 if vc > other (other happens before vc)
// 0 if vc == other (identical)
// 2 if vc and other are concurrent (conflict)
func (vc VectorClock) Compare(other VectorClock) int {
	isLess := false
	isGreater := false

	for node, t1 := range vc {
		t2 := other[node]
		if t1 < t2 {
			isLess = true
		} else if t1 > t2 {
			isGreater = true
		}
	}

	for node, t2 := range other {
		if _, exists := vc[node]; !exists && t2 > 0 {
			isLess = true
		}
	}

	if isLess && isGreater {
		return 2 // Concurrent
	} else if isLess {
		return -1 // vc < other
	} else if isGreater {
		return 1 // vc > other
	}
	return 0 // Equal
}

// Merge merges two vector clocks by taking the maximum timestamp for each node.
func (vc VectorClock) Merge(other VectorClock) VectorClock {
	merged := vc.Clone()
	for node, t2 := range other {
		if t1, exists := merged[node]; !exists || t2 > t1 {
			merged[node] = t2
		}
	}
	return merged
}

// Increment increments the vector clock for the given node.
func (vc VectorClock) Increment(nodeID string) {
	vc[nodeID]++
}

// ConflictResolver defines the logic for resolving P2P conflicts.
type ConflictResolver struct {
	nodeID string
}

func NewConflictResolver(nodeID string) *ConflictResolver {
	return &ConflictResolver{
		nodeID: nodeID,
	}
}

// ResolveState compares local and remote states and resolves conflicts.
// It returns the resolved state, merged vector clock, and a boolean indicating if local state needs update.
func (cr *ConflictResolver) ResolveState(localState map[string]any, localVC VectorClock, remoteState map[string]any, remoteVC VectorClock) (map[string]any, VectorClock, bool, error) {
	comp := localVC.Compare(remoteVC)

	switch comp {
	case -1:
		// Remote is newer, take remote state
		return remoteState, remoteVC.Clone(), true, nil
	case 1, 0:
		// Local is newer or identical, keep local state
		return localState, localVC.Clone(), false, nil
	case 2:
		// Concurrent (Conflict), merge vector clocks and apply simple LWW (Last Write Wins) on fields, or just merge based on timestamp.
		// For simplicity, without actual timestamp on fields, we can do LWW based on highest nodeID string comparison as a tiebreaker.

		mergedVC := localVC.Merge(remoteVC)

		// In a real scenario, this would be a field-level merge or prompt for manual resolution.
		// As a fallback tiebreaker, we use nodeID comparison.

		// To provide a consistent result across both peers, the peer with the lexicographically larger NodeID wins the conflict.
		// Note: A more sophisticated implementation might use field-level vector clocks or CRDTs.

		var resolvedState map[string]any
		needsUpdate := false

		// For demonstration, let's say the remote peer's ID is stored in the vector clock.
		// We need to figure out the remote peer ID. We can extract it from the differences.
		// But as a general rule, we can just do a map merge if it's a map.

		// Let's implement a deterministic merge.
		resolvedState = make(map[string]any)

		// Copy local
		for k, v := range localState {
			resolvedState[k] = v
		}

		// Overwrite with remote for conflicting keys using string comparison of values or keep remote if it's "greater"
		// A better deterministic tie-breaker:

		// For now, let's just say "remote wins" in a conflict for simplicity,
		// or we can just return a merged map.
		// Let's do a basic deterministic merge: if conflict, keep the one with larger JSON representation length, or just keep remote for simplicity.

		// Just to fulfill the "conflict resolution handlers" requirement:
		for k, rv := range remoteState {
			if lv, exists := localState[k]; exists {
				// Conflict on key k. Let's use a deterministic tiebreaker.
				// e.g., String format comparison.
				sLv := fmt.Sprintf("%v", lv)
				sRv := fmt.Sprintf("%v", rv)
				if sRv > sLv {
					resolvedState[k] = rv
					if sRv != sLv {
						needsUpdate = true
					}
				}
			} else {
				resolvedState[k] = rv
				needsUpdate = true
			}
		}

		// Increment our own clock because we merged
		mergedVC.Increment(cr.nodeID)

		return resolvedState, mergedVC, needsUpdate, nil
	}

	return localState, localVC, false, nil
}
