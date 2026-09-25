package mutation

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ShockwaveListener receives shockwave events emitted on transition checks.
type ShockwaveListener func(event ShockwaveEvent)

// GateEngine enforces strict per-kind lifecycle state transition matrices
// and emits shockwaves on transitions or rollbacks.
type GateEngine struct {
	mu           sync.RWMutex
	matrices     map[ObjectKind]map[ObjectStatus]map[ObjectStatus]bool
	listeners    []ShockwaveListener
	eventHistory []ShockwaveEvent
}

// NewGateEngine initializes the engine with canonical per-kind transition matrices.
func NewGateEngine() *GateEngine {
	ge := &GateEngine{
		matrices:     make(map[ObjectKind]map[ObjectStatus]map[ObjectStatus]bool),
		eventHistory: make([]ShockwaveEvent, 0),
	}
	ge.initMatrices()
	return ge
}

// Subscribe registers a shockwave listener.
func (ge *GateEngine) Subscribe(listener ShockwaveListener) {
	ge.mu.Lock()
	defer ge.mu.Unlock()
	ge.listeners = append(ge.listeners, listener)
}

// History returns a copy of emitted shockwave events.
func (ge *GateEngine) History() []ShockwaveEvent {
	ge.mu.RLock()
	defer ge.mu.RUnlock()
	out := make([]ShockwaveEvent, len(ge.eventHistory))
	copy(out, ge.eventHistory)
	return out
}

func (ge *GateEngine) initMatrices() {
	// Backlog Item Lifecycle:
	// originated -> planned -> in_progress -> complete / archived
	// Check-valve: in_progress cannot jump back to planned or originated
	ge.matrices[KindBacklogItem] = map[ObjectStatus]map[ObjectStatus]bool{
		StatusConceptual: {StatusOriginated: true},
		StatusOriginated: {StatusPlanned: true, StatusArchived: true},
		StatusPlanned:    {StatusInProgress: true, StatusBlocked: true, StatusArchived: true},
		StatusInProgress: {StatusComplete: true, StatusBlocked: true, StatusPaused: true, StatusArchived: true},
		StatusBlocked:    {StatusInProgress: true, StatusPlanned: true, StatusArchived: true},
		StatusPaused:     {StatusInProgress: true, StatusArchived: true},
		StatusComplete:   {StatusArchived: true},
	}

	// Priority Plan Lifecycle:
	// originated -> grooming -> active -> in_progress -> complete / archived
	ge.matrices[KindPriorityPlan] = map[ObjectStatus]map[ObjectStatus]bool{
		StatusConceptual: {StatusOriginated: true},
		StatusOriginated: {StatusGrooming: true, StatusActive: true, StatusArchived: true},
		StatusGrooming:   {StatusActive: true, StatusArchived: true},
		StatusActive:     {StatusInProgress: true, StatusPaused: true, StatusBlocked: true, StatusArchived: true},
		StatusInProgress: {StatusComplete: true, StatusPaused: true, StatusBlocked: true, StatusArchived: true},
		StatusComplete:   {StatusArchived: true},
	}

	// Requirement Lifecycle:
	ge.matrices[KindRequirement] = map[ObjectStatus]map[ObjectStatus]bool{
		StatusConceptual: {StatusOriginated: true},
		StatusOriginated: {StatusActive: true, StatusArchived: true},
		StatusActive:     {StatusComplete: true, StatusArchived: true},
	}
}

// EvaluateMutation validates the mutation request against lifecycle transition matrices and lineage invariants.
func (ge *GateEngine) EvaluateMutation(ctx context.Context, req KernelMutationRequest) MutationGateResult {
	event := ShockwaveEvent{
		EventID:        fmt.Sprintf("SW-%d", time.Now().UnixNano()),
		ObjectID:       req.ObjectID,
		Kind:           req.Kind,
		TransitionFrom: req.CurrentStatus,
		TransitionTo:   req.TargetStatus,
		Timestamp:      time.Now(),
	}

	// 1. Static Floor: Per-Kind State Transition Matrix
	validTransitions, kindExists := ge.matrices[req.Kind]
	if kindExists {
		allowedTargets, currentExists := validTransitions[req.CurrentStatus]
		if !currentExists || !allowedTargets[req.TargetStatus] {
			err := fmt.Errorf("%w: cannot transition %s from '%s' to '%s'",
				ErrInvalidStatusTransition, req.Kind, req.CurrentStatus, req.TargetStatus)
			event.Accepted = false
			event.RollbackNeeded = true
			event.Reason = err.Error()
			ge.emit(event)
			return MutationGateResult{Allowed: false, Error: err, Shockwave: event}
		}
	}

	// 2. Lineage Invariant: Objects transitioning to planned or active must have valid parent refs
	if req.TargetStatus == StatusPlanned || req.TargetStatus == StatusActive {
		if req.Kind == KindBacklogItem {
			if len(req.MilestoneRefs) == 0 {
				err := fmt.Errorf("%w: backlog_item must reference at least one milestone before advancing to %s",
					ErrIncompleteLineage, req.TargetStatus)
				event.Accepted = false
				event.RollbackNeeded = true
				event.Reason = err.Error()
				ge.emit(event)
				return MutationGateResult{Allowed: false, Error: err, Shockwave: event}
			}
		}
	}

	// 3. Completion Invariant: Transition to complete requires valid VDS and criteria satisfaction
	if req.TargetStatus == StatusComplete {
		if req.Kind == KindBacklogItem {
			if !req.CriteriaComplete {
				err := fmt.Errorf("%w: backlog_item cannot complete with unsatisfied or unbound criteria",
					ErrUnboundCriteria)
				event.Accepted = false
				event.RollbackNeeded = true
				event.Reason = err.Error()
				ge.emit(event)
				return MutationGateResult{Allowed: false, Error: err, Shockwave: event}
			}
		}
	}

	// Passed all gates
	event.Accepted = true
	event.RollbackNeeded = false
	ge.emit(event)

	return MutationGateResult{
		Allowed:   true,
		Error:     nil,
		Shockwave: event,
	}
}

func (ge *GateEngine) emit(event ShockwaveEvent) {
	ge.mu.Lock()
	ge.eventHistory = append(ge.eventHistory, event)
	listeners := make([]ShockwaveListener, len(ge.listeners))
	copy(listeners, ge.listeners)
	ge.mu.Unlock()

	for _, l := range listeners {
		l(event)
	}
}
