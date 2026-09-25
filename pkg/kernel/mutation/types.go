package mutation

import (
	"errors"
	"time"
)

// Lifecycle errors.
var (
	ErrInvalidStatusTransition  = errors.New("mutation gate: illegal status transition jump")
	ErrIncompleteLineage        = errors.New("mutation gate: incomplete lineage; missing required milestone, workstream, or goal refs")
	ErrUnboundCriteria          = errors.New("mutation gate: promotion rejected; object has unbound criteria")
	ErrMissingRequiredField     = errors.New("mutation gate: required schema field missing")
	ErrAtomicRollbackTriggered  = errors.New("mutation gate: shockwave rollback triggered")
)

// ObjectKind defines supported kernel object kinds.
type ObjectKind string

const (
	KindGoal         ObjectKind = "goal"
	KindMilestone    ObjectKind = "milestone"
	KindRequirement  ObjectKind = "requirement"
	KindCriteria     ObjectKind = "criteria"
	KindTestCase     ObjectKind = "test_case"
	KindPriorityPlan ObjectKind = "priority_plan"
	KindBacklogItem  ObjectKind = "backlog_item"
)

// ObjectStatus defines standard lifecycle states.
type ObjectStatus string

const (
	StatusConceptual  ObjectStatus = "conceptual"
	StatusOriginated  ObjectStatus = "originated"
	StatusGrooming    ObjectStatus = "grooming"
	StatusPlanned     ObjectStatus = "planned"
	StatusActive      ObjectStatus = "active"
	StatusInProgress  ObjectStatus = "in_progress"
	StatusComplete    ObjectStatus = "complete"
	StatusArchived    ObjectStatus = "archived"
	StatusBlocked     ObjectStatus = "blocked"
	StatusPaused      ObjectStatus = "paused"
)

// KernelMutationRequest represents an intent to transition an object's lifecycle status.
type KernelMutationRequest struct {
	ObjectID         string
	Kind             ObjectKind
	CurrentStatus    ObjectStatus
	TargetStatus     ObjectStatus
	GoalRefs         []string
	MilestoneRefs    []string
	WorkstreamRefs   []string
	CriteriaRefs     []string
	TestCaseRefs     []string
	UpdatedBy        string
	VDSVerified      bool
	CriteriaComplete bool
}

// ShockwaveEvent describes the event emitted during a state transition check or rollback.
type ShockwaveEvent struct {
	EventID        string       `json:"event_id"`
	ObjectID       string       `json:"object_id"`
	Kind           ObjectKind   `json:"kind"`
	TransitionFrom ObjectStatus `json:"transition_from"`
	TransitionTo   ObjectStatus `json:"transition_to"`
	Accepted       bool         `json:"accepted"`
	RollbackNeeded bool         `json:"rollback_needed"`
	Reason         string       `json:"reason,omitempty"`
	Timestamp      time.Time    `json:"timestamp"`
}

// MutationGateResult holds the outcome of a mutation check.
type MutationGateResult struct {
	Allowed   bool
	Error     error
	Shockwave ShockwaveEvent
}
