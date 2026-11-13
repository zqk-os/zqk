package community

import (
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrRunwayExhausted is returned when remaining token runway is below the minimum threshold.
	ErrRunwayExhausted = errors.New("governor: runway token budget exhausted")
	// ErrAdmissionShed is returned when queue backlog exceeds capacity and load is shed.
	ErrAdmissionShed = errors.New("governor: admission shed due to maximum capacity saturation")
	// ErrInvalidPolicy is returned when RunwayCapacityPolicy violates boundary invariants.
	ErrInvalidPolicy = errors.New("governor: invalid runway capacity policy")
	// ErrNegativeCost is returned when a task has a negative cost.
	ErrNegativeCost = errors.New("governor: task token cost cannot be negative")
)

// RunwayCapacityPolicy defines limits and scaling parameters for swarm execution.
type RunwayCapacityPolicy struct {
	MinWorkers           int `json:"min_workers"`
	MaxWorkers           int `json:"max_workers"`
	TargetTasksPerWorker int `json:"target_tasks_per_worker"`
	MinRunwayTokens      int `json:"min_runway_tokens"`
	MaxQueueDepth        int `json:"max_queue_depth"`
}

// Validate checks policy invariant boundaries.
func (p *RunwayCapacityPolicy) Validate() error {
	if p == nil {
		return fmt.Errorf("%w: policy cannot be nil", ErrInvalidPolicy)
	}
	if p.MinWorkers <= 0 {
		return fmt.Errorf("%w: min_workers must be positive", ErrInvalidPolicy)
	}
	if p.MaxWorkers < p.MinWorkers {
		return fmt.Errorf("%w: max_workers cannot be less than min_workers", ErrInvalidPolicy)
	}
	if p.TargetTasksPerWorker <= 0 {
		return fmt.Errorf("%w: target_tasks_per_worker must be positive", ErrInvalidPolicy)
	}
	if p.MinRunwayTokens < 0 {
		return fmt.Errorf("%w: min_runway_tokens cannot be negative", ErrInvalidPolicy)
	}
	if p.MaxQueueDepth <= 0 {
		return fmt.Errorf("%w: max_queue_depth must be positive", ErrInvalidPolicy)
	}
	return nil
}

// RunwayGovernor manages dynamic worker pool scaling and admission throttling against runway tokens.
type RunwayGovernor struct {
	mu            sync.RWMutex
	policy        RunwayCapacityPolicy
	activeWorkers int
	runwayTokens  int
	queueDepth    int
}

// NewRunwayGovernor initializes a governor with the provided policy and starting token budget.
func NewRunwayGovernor(policy RunwayCapacityPolicy, initialTokens int) (*RunwayGovernor, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if initialTokens < 0 {
		initialTokens = 0
	}
	return &RunwayGovernor{
		policy:        policy,
		activeWorkers: policy.MinWorkers,
		runwayTokens:  initialTokens,
		queueDepth:    0,
	}, nil
}

// AdmitTask evaluates whether a new task can be admitted without breaching runway or queue limits.
func (g *RunwayGovernor) AdmitTask(estimatedCost int) error {
	if estimatedCost < 0 {
		return ErrNegativeCost
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Check runway token threshold
	if g.runwayTokens-estimatedCost < g.policy.MinRunwayTokens {
		return ErrRunwayExhausted
	}

	// Check queue saturation threshold
	if g.queueDepth >= g.policy.MaxQueueDepth {
		return ErrAdmissionShed
	}

	g.queueDepth++
	g.runwayTokens -= estimatedCost
	return nil
}

// CompleteTask decrements active queue depth and completes a task.
func (g *RunwayGovernor) CompleteTask() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.queueDepth > 0 {
		g.queueDepth--
	}
}

// ReplenishRunway deposits additional execution tokens into the governor runway.
func (g *RunwayGovernor) ReplenishRunway(tokens int) {
	if tokens <= 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.runwayTokens += tokens
}

// EvaluateScaling determines the target worker pool size based on backlog pressure and runway headroom.
func (g *RunwayGovernor) EvaluateScaling() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.runwayTokens <= g.policy.MinRunwayTokens {
		// Conserve runway: clamp to min workers
		g.activeWorkers = g.policy.MinWorkers
		return g.activeWorkers
	}

	// Desired workers = ceil(queueDepth / targetPerWorker)
	desired := (g.queueDepth + g.policy.TargetTasksPerWorker - 1) / g.policy.TargetTasksPerWorker
	if desired < g.policy.MinWorkers {
		desired = g.policy.MinWorkers
	}
	if desired > g.policy.MaxWorkers {
		desired = g.policy.MaxWorkers
	}

	g.activeWorkers = desired
	return g.activeWorkers
}

// GetMetrics returns a snapshot of the current governor status.
func (g *RunwayGovernor) GetMetrics() (activeWorkers, queueDepth, runwayTokens int) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.activeWorkers, g.queueDepth, g.runwayTokens
}
