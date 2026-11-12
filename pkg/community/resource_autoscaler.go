package community

import (
	"errors"
	"math"
	"sync"
)

var (
	// ErrInvalidAutoscalingPolicy is returned when policy parameters violate numeric invariants.
	ErrInvalidAutoscalingPolicy = errors.New("invalid autoscaling policy")
)

// AutoscalingPolicy defines the operational boundaries and step factors for elastic worker scaling.
type AutoscalingPolicy struct {
	MinConcurrency int
	MaxConcurrency int
	QueueThreshold int
	ScaleFactor    float64
}

// Validate verifies that the policy satisfies numeric floor invariants.
func (p *AutoscalingPolicy) Validate() error {
	if p == nil {
		return ErrInvalidAutoscalingPolicy
	}
	if p.MinConcurrency < 1 || p.MaxConcurrency < p.MinConcurrency {
		return ErrInvalidAutoscalingPolicy
	}
	if p.ScaleFactor <= 1.0 || math.IsNaN(p.ScaleFactor) || math.IsInf(p.ScaleFactor, 0) {
		return ErrInvalidAutoscalingPolicy
	}
	if p.QueueThreshold < 0 {
		return ErrInvalidAutoscalingPolicy
	}
	return nil
}

// ResourceAutoscaler manages dynamic concurrency adjustments based on queue load.
type ResourceAutoscaler struct {
	mu                 sync.RWMutex
	policy             AutoscalingPolicy
	currentConcurrency int
}

// NewResourceAutoscaler initializes an autoscaler with a verified policy.
func NewResourceAutoscaler(policy AutoscalingPolicy) (*ResourceAutoscaler, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &ResourceAutoscaler{
		policy:             policy,
		currentConcurrency: policy.MinConcurrency,
	}, nil
}

// CurrentConcurrency returns the currently allocated worker pool size.
func (r *ResourceAutoscaler) CurrentConcurrency() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentConcurrency
}

// Policy returns a copy of the active autoscaling policy.
func (r *ResourceAutoscaler) Policy() AutoscalingPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.policy
}

// AdjustConcurrency recalculates and updates the target concurrency level
// given the current backlog depth and in-flight active workers.
func (r *ResourceAutoscaler) AdjustConcurrency(queueDepth, activeWorkers int) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if queueDepth > r.policy.QueueThreshold {
		// Scale up by scale factor
		scaled := int(math.Ceil(float64(r.currentConcurrency) * r.policy.ScaleFactor))
		if scaled > r.policy.MaxConcurrency {
			scaled = r.policy.MaxConcurrency
		}
		if scaled < r.currentConcurrency+1 {
			scaled = r.currentConcurrency + 1
			if scaled > r.policy.MaxConcurrency {
				scaled = r.policy.MaxConcurrency
			}
		}
		r.currentConcurrency = scaled
	} else if queueDepth == 0 && activeWorkers < r.currentConcurrency {
		// Scale down towards minimum
		scaled := int(math.Floor(float64(r.currentConcurrency) / r.policy.ScaleFactor))
		if scaled < r.policy.MinConcurrency {
			scaled = r.policy.MinConcurrency
		}
		r.currentConcurrency = scaled
	}

	return r.currentConcurrency
}
