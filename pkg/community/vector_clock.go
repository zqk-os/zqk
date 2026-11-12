package community

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// CausalRelation represents the partial ordering relation between two vector clocks.
type CausalRelation string

const (
	// RelationEqual indicates both vector clocks have identical components.
	RelationEqual CausalRelation = "equal"
	// RelationBefore indicates this vector clock causally happened before the other.
	RelationBefore CausalRelation = "before"
	// RelationAfter indicates this vector clock causally happened after the other.
	RelationAfter CausalRelation = "after"
	// RelationConcurrent indicates neither clock causally dominates the other (concurrent events).
	RelationConcurrent CausalRelation = "concurrent"
)

// VectorClock represents a thread-safe causal vector clock tracking logical time across nodes.
type VectorClock struct {
	mu     sync.RWMutex
	values map[string]uint64
}

// NewVectorClock creates an empty, initialized VectorClock.
func NewVectorClock() *VectorClock {
	return &VectorClock{
		values: make(map[string]uint64),
	}
}

// NewVectorClockFromMap initializes a VectorClock with the given map.
func NewVectorClockFromMap(m map[string]uint64) *VectorClock {
	vc := NewVectorClock()
	if m != nil {
		for k, v := range m {
			if strutil.IsNonBlank(k) {
				vc.values[k] = v
			}
		}
	}
	return vc
}

// Clone returns a deep copy of the VectorClock.
func (vc *VectorClock) Clone() *VectorClock {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	cpy := NewVectorClock()
	for k, v := range vc.values {
		cpy.values[k] = v
	}
	return cpy
}

// Tick increments the logical clock component for the specified node ID.
func (vc *VectorClock) Tick(nodeID string) error {
	if err := strutil.RequireNonBlank(nodeID, "node ID"); err != nil {
		return err
	}

	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.values[nodeID]++
	return nil
}

// Get returns the logical clock value for the specified node.
func (vc *VectorClock) Get(nodeID string) uint64 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.values[nodeID]
}

// Set explicitly sets the logical clock for a node (must be >= current value).
func (vc *VectorClock) Set(nodeID string, val uint64) error {
	if err := strutil.RequireNonBlank(nodeID, "node ID"); err != nil {
		return err
	}

	vc.mu.Lock()
	defer vc.mu.Unlock()

	if val < vc.values[nodeID] {
		return fmt.Errorf("vector clock value %d cannot regress current value %d for node %s", val, vc.values[nodeID], nodeID)
	}

	vc.values[nodeID] = val
	return nil
}

// Merge takes the pairwise maximum of all components between this and another clock.
func (vc *VectorClock) Merge(other *VectorClock) error {
	if other == nil {
		return errors.New("cannot merge nil vector clock")
	}

	other.mu.RLock()
	defer other.mu.RUnlock()

	vc.mu.Lock()
	defer vc.mu.Unlock()

	for k, v := range other.values {
		if v > vc.values[k] {
			vc.values[k] = v
		}
	}
	return nil
}

// Compare evaluates the partial causal ordering relation between this clock and other.
func (vc *VectorClock) Compare(other *VectorClock) CausalRelation {
	if other == nil {
		return RelationAfter
	}

	vc.mu.RLock()
	defer vc.mu.RUnlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	allKeys := make(map[string]struct{}, len(vc.values)+len(other.values))
	for k := range vc.values {
		allKeys[k] = struct{}{}
	}
	for k := range other.values {
		allKeys[k] = struct{}{}
	}

	var hasGreater, hasLesser bool
	for k := range allKeys {
		v1 := vc.values[k]
		v2 := other.values[k]
		if v1 > v2 {
			hasGreater = true
		} else if v1 < v2 {
			hasLesser = true
		}
	}

	switch {
	case !hasGreater && !hasLesser:
		return RelationEqual
	case hasGreater && !hasLesser:
		return RelationAfter
	case !hasGreater && hasLesser:
		return RelationBefore
	default:
		return RelationConcurrent
	}
}

// Map returns a snapshot map of the vector clock components.
func (vc *VectorClock) Map() map[string]uint64 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	m := make(map[string]uint64, len(vc.values))
	for k, v := range vc.values {
		m[k] = v
	}
	return m
}

// Serialize serializes the clock to JSON bytes.
func (vc *VectorClock) Serialize() ([]byte, error) {
	return json.Marshal(vc.Map())
}

// DeserializeVectorClock parses a serialized vector clock.
func DeserializeVectorClock(data []byte) (*VectorClock, error) {
	if len(data) == 0 {
		return nil, errors.New("empty vector clock payload")
	}
	var m map[string]uint64
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal vector clock: %w", err)
	}
	return NewVectorClockFromMap(m), nil
}

// ShockwaveEvent describes an asynchronous state transition shockwave.
type ShockwaveEvent struct {
	EventID     string       `json:"event_id"`
	OriginNode  string       `json:"origin_node"`
	EntityID    string       `json:"entity_id"`
	TargetState string       `json:"target_state"`
	Clock       *VectorClock `json:"clock"`
}

// SettleAction denotes the outcome of shockwave settlement.
type SettleAction string

const (
	ActionAccepted SettleAction = "accepted"
	ActionDropped  SettleAction = "dropped"
	ActionConflict SettleAction = "conflict_resolved"
)

// SettleResult details the outcome of settling a shockwave event.
type SettleResult struct {
	Action      SettleAction   `json:"action"`
	EntityID    string         `json:"entity_id"`
	FinalState  string         `json:"final_state"`
	MergedClock *VectorClock   `json:"merged_clock"`
	Reason      string         `json:"reason"`
	Relation    CausalRelation `json:"relation"`
}

// EntityState tracks the current settled state of an entity.
type EntityState struct {
	EntityID   string       `json:"entity_id"`
	State      string       `json:"state"`
	Clock      *VectorClock `json:"clock"`
	LastOrigin string       `json:"last_origin"`
}

// ShockwaveSettler evaluates incoming shockwave events and settles them idempotently.
type ShockwaveSettler struct {
	mu           sync.RWMutex
	localNodeID  string
	maxClockSkew uint64
	entities     map[string]*EntityState
}

// NewShockwaveSettler instantiates a ShockwaveSettler with skew fencing.
func NewShockwaveSettler(localNodeID string, maxClockSkew uint64) (*ShockwaveSettler, error) {
	if err := strutil.RequireNonBlank(localNodeID, "local node ID"); err != nil {
		return nil, err
	}
	if maxClockSkew == 0 {
		maxClockSkew = 1000 // default skew ceiling
	}

	return &ShockwaveSettler{
		localNodeID:  localNodeID,
		maxClockSkew: maxClockSkew,
		entities:     make(map[string]*EntityState),
	}, nil
}

// Settle evaluates an incoming shockwave event and updates the settled entity state.
func (s *ShockwaveSettler) Settle(event ShockwaveEvent) (SettleResult, error) {
	if err := strutil.RequireNonBlank(event.EntityID, "entity ID"); err != nil {
		return SettleResult{}, err
	}
	if err := strutil.RequireNonBlank(event.OriginNode, "origin node"); err != nil {
		return SettleResult{}, err
	}
	if err := strutil.RequireNonBlank(event.TargetState, "target state"); err != nil {
		return SettleResult{}, err
	}
	if event.Clock == nil {
		return SettleResult{}, errors.New("event vector clock cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.entities[event.EntityID]
	if !exists {
		// New entity: check clock skew
		incomingVal := event.Clock.Get(event.OriginNode)
		if incomingVal > s.maxClockSkew {
			return SettleResult{
				Action:   ActionDropped,
				EntityID: event.EntityID,
				Reason:   fmt.Sprintf("excessive clock skew (%d > %d)", incomingVal, s.maxClockSkew),
			}, fmt.Errorf("clock skew exceeded ceiling: %d > %d", incomingVal, s.maxClockSkew)
		}

		newState := &EntityState{
			EntityID:   event.EntityID,
			State:      event.TargetState,
			Clock:      event.Clock.Clone(),
			LastOrigin: event.OriginNode,
		}
		s.entities[event.EntityID] = newState
		return SettleResult{
			Action:      ActionAccepted,
			EntityID:    event.EntityID,
			FinalState:  event.TargetState,
			MergedClock: newState.Clock.Clone(),
			Reason:      "initial state established",
			Relation:    RelationAfter,
		}, nil
	}

	// Compare incoming clock with current settled clock
	relation := event.Clock.Compare(current.Clock)

	switch relation {
	case RelationEqual:
		return SettleResult{
			Action:      ActionDropped,
			EntityID:    event.EntityID,
			FinalState:  current.State,
			MergedClock: current.Clock.Clone(),
			Reason:      "duplicate identical shockwave event",
			Relation:    RelationEqual,
		}, nil

	case RelationBefore:
		return SettleResult{
			Action:      ActionDropped,
			EntityID:    event.EntityID,
			FinalState:  current.State,
			MergedClock: current.Clock.Clone(),
			Reason:      "causally superseded stale shockwave event",
			Relation:    RelationBefore,
		}, nil

	case RelationAfter:
		_ = current.Clock.Merge(event.Clock)
		current.State = event.TargetState
		current.LastOrigin = event.OriginNode
		return SettleResult{
			Action:      ActionAccepted,
			EntityID:    event.EntityID,
			FinalState:  current.State,
			MergedClock: current.Clock.Clone(),
			Reason:      "causal successor accepted",
			Relation:    RelationAfter,
		}, nil

	default: // RelationConcurrent
		// Concurrent shockwaves: merge vector clocks, break ties deterministically by lexicographical node ID
		_ = current.Clock.Merge(event.Clock)
		if event.OriginNode > current.LastOrigin {
			current.State = event.TargetState
			current.LastOrigin = event.OriginNode
		}
		return SettleResult{
			Action:      ActionConflict,
			EntityID:    event.EntityID,
			FinalState:  current.State,
			MergedClock: current.Clock.Clone(),
			Reason:      "concurrent shockwaves merged with deterministic tie-break",
			Relation:    RelationConcurrent,
		}, nil
	}
}

// GetEntity returns the settled state of an entity.
func (s *ShockwaveSettler) GetEntity(entityID string) (EntityState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st, ok := s.entities[entityID]
	if !ok {
		return EntityState{}, false
	}
	return EntityState{
		EntityID:   st.EntityID,
		State:      st.State,
		Clock:      st.Clock.Clone(),
		LastOrigin: st.LastOrigin,
	}, true
}
