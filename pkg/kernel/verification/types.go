package verification

import (
	"context"
	"errors"
	"time"
)

// TopologyMode defines the verification execution topology.
type TopologyMode string

const (
	// TopologySequential runs stages in linear sequence.
	TopologySequential TopologyMode = "sequential"
	// TopologyConcurrent executes independent stages in parallel.
	TopologyConcurrent TopologyMode = "concurrent"
	// TopologyHybridDAG executes stages according to dependency edges with barrier synchronization.
	TopologyHybridDAG TopologyMode = "hybrid_dag"
)

// StageCategory classifies the criterion proof role.
type StageCategory string

const (
	CategoryStaticFloor       StageCategory = "static_floor"
	CategoryOperationalProof  StageCategory = "operational_proof"
	CategoryNegativeInvariant StageCategory = "negative_invariant"
)

// Verification errors.
var (
	ErrNilOrganizer              = errors.New("verification: execution organizer cannot be nil")
	ErrInsufficientCriteriaRatio = errors.New("verification: test case must bind at least 2 criteria (CRIT:TST >= 2:1); 1:1 single-verification test cases prohibited")
	ErrCircularDependency        = errors.New("verification: circular dependency detected in stage topology")
	ErrStagePanic                = errors.New("verification: stage panicked during execution")
)

// VerifyFunc defines the execution signature for a single criterion verification stage.
type VerifyFunc func(ctx context.Context) error

// VerificationStage represents a single verification step bound to a criterion.
type VerificationStage struct {
	ID        string
	Category  StageCategory
	Verify    VerifyFunc
	DependsOn []string
	Timeout   time.Duration
}

// ExecutionOrganizer defines the composite verification topology for a test case.
type ExecutionOrganizer struct {
	TestCaseID string
	Mode       TopologyMode
	Stages     []VerificationStage
}

// StageResult captures the outcome of an individual verification stage.
type StageResult struct {
	StageID  string
	Category StageCategory
	Passed   bool
	Error    error
	Panicked bool
	Duration time.Duration
}

// VerificationReport contains aggregated diagnostics for the composite test case.
type VerificationReport struct {
	TestCaseID    string
	Mode          TopologyMode
	Passed        bool
	StageResults  map[string]*StageResult
	PanicsCaught  int
	TotalDuration time.Duration
}
