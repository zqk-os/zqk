package community

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// FallbackPolicy defines the strategy executed upon primary provider failure.
type FallbackPolicy string

const (
	// FallbackPolicyFailover delegates to the next available fallback provider in order.
	FallbackPolicyFailover FallbackPolicy = "failover"
	// FallbackPolicyDegradedStatic returns a pre-configured baseline synthetic response.
	FallbackPolicyDegradedStatic FallbackPolicy = "degraded_static"
	// FallbackPolicyFastReject halts immediately with an error when the primary fails.
	FallbackPolicyFastReject FallbackPolicy = "fast_reject"
)

var (
	ErrNoDegradationPlan   = errors.New("no degradation plan registered for provider")
	ErrAllFallbacksFailed  = errors.New("all fallback providers exhausted and failed")
	ErrFastRejectTriggered = errors.New("fast-reject policy triggered on provider failure")
	ErrNilProviderAction   = errors.New("cannot execute nil provider action")
)

// DegradationEvent records a degradation or fallback incident for audit telemetry.
type DegradationEvent struct {
	OriginalProvider string         `json:"original_provider"`
	FallbackProvider string         `json:"fallback_provider,omitempty"`
	Policy           FallbackPolicy `json:"policy"`
	Reason           string         `json:"reason"`
	Timestamp        time.Time      `json:"timestamp"`
	Duration         time.Duration  `json:"duration"`
	Success          bool           `json:"success"`
}

// FallbackAction represents the executable work unit for a provider.
type FallbackAction func(ctx context.Context) (any, error)

// ProviderDegradationPlan configures resilience behavior for a primary provider.
type ProviderDegradationPlan struct {
	PrimaryID      string
	FallbackIDs    []string
	Policy         FallbackPolicy
	StaticResponse any
}

// DegradationCoordinator coordinates graceful fallbacks and static baseline responses.
type DegradationCoordinator struct {
	mu        sync.RWMutex
	plans     map[string]ProviderDegradationPlan
	actions   map[string]FallbackAction
	history   []DegradationEvent
	maxEvents int
}

// NewDegradationCoordinator initializes a thread-safe DegradationCoordinator.
func NewDegradationCoordinator(maxHistory int) *DegradationCoordinator {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	return &DegradationCoordinator{
		plans:     make(map[string]ProviderDegradationPlan),
		actions:   make(map[string]FallbackAction),
		history:   make([]DegradationEvent, 0, maxHistory),
		maxEvents: maxHistory,
	}
}

// RegisterPlan registers or updates a degradation plan for a primary provider.
func (c *DegradationCoordinator) RegisterPlan(plan ProviderDegradationPlan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.plans[plan.PrimaryID] = plan
}

// RegisterAction registers the executable action for a provider.
func (c *DegradationCoordinator) RegisterAction(providerID string, action FallbackAction) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actions[providerID] = action
}

// Execute executes the primary provider action, falling back gracefully upon failure.
func (c *DegradationCoordinator) Execute(ctx context.Context, primaryID string) (any, error) {
	c.mu.RLock()
	plan, planExists := c.plans[primaryID]
	primaryAction, actionExists := c.actions[primaryID]
	c.mu.RUnlock()

	if !planExists {
		return nil, ErrNoDegradationPlan
	}
	if !actionExists || primaryAction == nil {
		return nil, ErrNilProviderAction
	}

	start := time.Now()
	res, err := primaryAction(ctx)
	if err == nil {
		return res, nil
	}

	// Primary failed - initiate degradation policy
	switch plan.Policy {
	case FallbackPolicyFastReject:
		c.recordEvent(DegradationEvent{
			OriginalProvider: primaryID,
			Policy:           FallbackPolicyFastReject,
			Reason:           err.Error(),
			Timestamp:        start,
			Duration:         time.Since(start),
			Success:          false,
		})
		return nil, errors.Join(ErrFastRejectTriggered, err)

	case FallbackPolicyDegradedStatic:
		c.recordEvent(DegradationEvent{
			OriginalProvider: primaryID,
			Policy:           FallbackPolicyDegradedStatic,
			Reason:           err.Error(),
			Timestamp:        start,
			Duration:         time.Since(start),
			Success:          true,
		})
		return plan.StaticResponse, nil

	case FallbackPolicyFailover:
		for _, fallbackID := range plan.FallbackIDs {
			c.mu.RLock()
			fbAction, fbExists := c.actions[fallbackID]
			c.mu.RUnlock()

			if !fbExists || fbAction == nil {
				continue
			}

			fbStart := time.Now()
			fbRes, fbErr := fbAction(ctx)
			if fbErr == nil {
				c.recordEvent(DegradationEvent{
					OriginalProvider: primaryID,
					FallbackProvider: fallbackID,
					Policy:           FallbackPolicyFailover,
					Reason:           err.Error(),
					Timestamp:        fbStart,
					Duration:         time.Since(fbStart),
					Success:          true,
				})
				return fbRes, nil
			}

			// Record intermediate fallback failure
			c.recordEvent(DegradationEvent{
				OriginalProvider: primaryID,
				FallbackProvider: fallbackID,
				Policy:           FallbackPolicyFailover,
				Reason:           fbErr.Error(),
				Timestamp:        fbStart,
				Duration:         time.Since(fbStart),
				Success:          false,
			})
		}
		return nil, errors.Join(ErrAllFallbacksFailed, err)

	default:
		return nil, err
	}
}

// EventHistory returns a copy of recorded degradation telemetry events.
func (c *DegradationCoordinator) EventHistory() []DegradationEvent {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]DegradationEvent, len(c.history))
	copy(result, c.history)
	return result
}

// recordEvent securely logs an event without blocking.
func (c *DegradationCoordinator) recordEvent(evt DegradationEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.history) >= c.maxEvents {
		c.history = c.history[1:]
	}
	c.history = append(c.history, evt)
}

// SafeRunGoroutine executes an async worker under POL-CONCURRENCY guidelines.
func (c *DegradationCoordinator) SafeRunGoroutine(ctx context.Context, label string, fn func()) {
	goroutinelabels.NewGoroutine("community.degradation."+label, "async degradation worker").StartSimple(fn)
}
