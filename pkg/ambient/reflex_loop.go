package ambient

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Named constants enforcing bounded resource allocation and deterministic execution.
const (
	DefaultReflexActionQueueCapacity = 256
	DefaultReflexEvaluationTimeout   = 5 * time.Second
	DefaultMaxTriggerRules           = 64
	DefaultMaxActionHistory          = 512
)

var (
	ErrNilReflexEngine    = errors.New("spinal cord reflex engine cannot be nil")
	ErrReflexEngineClosed = errors.New("spinal cord reflex engine is closed")
	ErrNilTriggerRule     = errors.New("trigger rule predicate cannot be nil")
	ErrNilActionHandler   = errors.New("action handler cannot be nil")
	ErrActionQueueFull    = errors.New("reflex action queue capacity exhausted")
	ErrEmptyActionType    = errors.New("reflex action type cannot be empty")
)

// ReflexActionType represents a typed restorative intervention executed by the reflex loop.
type ReflexActionType string

const (
	ReflexActionRealignIntent ReflexActionType = "realign_intent"
	ReflexActionPruneBranch   ReflexActionType = "prune_branch"
	ReflexActionRestartHolon  ReflexActionType = "restart_holon"
	ReflexActionDrainStream   ReflexActionType = "drain_stream"
	ReflexActionQuiesceEngine ReflexActionType = "quiesce_engine"
)

// ReflexAction represents an actionable restorative intervention.
type ReflexAction struct {
	ID        string           `json:"id"`
	Type      ReflexActionType `json:"type"`
	Target    string           `json:"target"`
	Reason    string           `json:"reason"`
	Payload   map[string]any   `json:"payload,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

// EvaluationResult captures the outcome of evaluating a telemetry snapshot against reflex rules.
type EvaluationResult struct {
	TriggeredActions []ReflexAction `json:"triggered_actions"`
	Healthy          bool           `json:"healthy"`
	Reason           string         `json:"reason,omitempty"`
	EvaluatedAt      time.Time      `json:"evaluated_at"`
}

// ReflexTriggerPredicate evaluates a telemetry snapshot and produces compensating actions.
type ReflexTriggerPredicate func(snapshot *MiddleBrainTelemetrySnapshot) ([]ReflexAction, bool)

// ReflexActionHandler defines the callback when an action is executed.
type ReflexActionHandler func(ctx context.Context, action ReflexAction) error

// ReflexOption configures the SpinalCordReflexEngine.
type ReflexOption func(*SpinalCordReflexEngine)

// WithActionQueueCapacity sets the action channel buffer capacity.
func WithActionQueueCapacity(capacity int) ReflexOption {
	return func(e *SpinalCordReflexEngine) {
		if capacity > 0 {
			e.capacity = capacity
		}
	}
}

// WithEvaluationTimeout configures the maximum evaluation time.
func WithEvaluationTimeout(timeout time.Duration) ReflexOption {
	return func(e *SpinalCordReflexEngine) {
		if timeout > 0 {
			e.timeout = timeout
		}
	}
}

// WithTriggerRule registers an initial trigger rule predicate.
func WithTriggerRule(rule ReflexTriggerPredicate) ReflexOption {
	return func(e *SpinalCordReflexEngine) {
		if rule != nil {
			e.rules = append(e.rules, rule)
		}
	}
}

// WithActionHandler registers an initial action execution handler.
func WithActionHandler(actionType ReflexActionType, handler ReflexActionHandler) ReflexOption {
	return func(e *SpinalCordReflexEngine) {
		if handler != nil {
			e.handlers[actionType] = handler
		}
	}
}

// SpinalCordReflexMetrics encapsulates operational telemetry of the reflex engine.
type SpinalCordReflexMetrics struct {
	EvaluationsCount int64            `json:"evaluations_count"`
	ActionsTriggered int64            `json:"actions_triggered"`
	ActionsHandled   int64            `json:"actions_handled"`
	ActionsFailed    int64            `json:"actions_failed"`
	LastEvaluatedAt  time.Time        `json:"last_evaluated_at"`
	ActionCounts     map[string]int64 `json:"action_counts"`
}

// SpinalCordReflexEngine coordinates autonomous health evaluation and restorative actions.
type SpinalCordReflexEngine struct {
	mu           sync.RWMutex
	capacity     int
	timeout      time.Duration
	rules        []ReflexTriggerPredicate
	handlers     map[ReflexActionType]ReflexActionHandler
	actionQueue  chan ReflexAction
	history      []ReflexAction
	maxHistory   int
	closed       bool
	evalCount    atomic.Int64
	actionCount  atomic.Int64
	handledCount atomic.Int64
	failedCount  atomic.Int64
	lastEvalTime atomic.Pointer[time.Time]
}

// NewSpinalCordReflexEngine creates a new reflex loop engine with safe capacity floors.
func NewSpinalCordReflexEngine(opts ...ReflexOption) *SpinalCordReflexEngine {
	e := &SpinalCordReflexEngine{
		capacity:   DefaultReflexActionQueueCapacity,
		timeout:    DefaultReflexEvaluationTimeout,
		maxHistory: DefaultMaxActionHistory,
		rules:      make([]ReflexTriggerPredicate, 0, DefaultMaxTriggerRules),
		handlers:   make(map[ReflexActionType]ReflexActionHandler),
		history:    make([]ReflexAction, 0, DefaultMaxActionHistory),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	e.actionQueue = make(chan ReflexAction, e.capacity)
	return e
}

// Evaluate evaluates a telemetry snapshot against all registered rules.
// Nil receiver returns safe fallback without panicking.
func (e *SpinalCordReflexEngine) Evaluate(ctx context.Context, snapshot *MiddleBrainTelemetrySnapshot) (*EvaluationResult, error) {
	now := time.Now()
	if e == nil {
		return &EvaluationResult{
			TriggeredActions: nil,
			Healthy:          false,
			Reason:           "nil reflex engine receiver",
			EvaluatedAt:      now,
		}, ErrNilReflexEngine
	}

	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return &EvaluationResult{
			TriggeredActions: nil,
			Healthy:          false,
			Reason:           "engine closed",
			EvaluatedAt:      now,
		}, ErrReflexEngineClosed
	}
	rulesCopy := make([]ReflexTriggerPredicate, len(e.rules))
	copy(rulesCopy, e.rules)
	e.mu.RUnlock()

	e.evalCount.Add(1)
	e.lastEvalTime.Store(&now)

	if snapshot == nil {
		return &EvaluationResult{
			TriggeredActions: nil,
			Healthy:          false,
			Reason:           "nil telemetry snapshot",
			EvaluatedAt:      now,
		}, nil
	}

	var triggered []ReflexAction
	allHealthy := true
	for _, rule := range rulesCopy {
		if rule == nil {
			continue
		}
		actions, healthy := rule(snapshot)
		if !healthy {
			allHealthy = false
		}
		if len(actions) > 0 {
			triggered = append(triggered, actions...)
		}
	}

	for _, action := range triggered {
		_ = e.TriggerAction(ctx, action)
	}

	return &EvaluationResult{
		TriggeredActions: triggered,
		Healthy:          allHealthy,
		EvaluatedAt:      now,
	}, nil
}

// TriggerAction enqueues an action and invokes registered handlers.
// Nil receiver returns ErrNilReflexEngine without panicking.
func (e *SpinalCordReflexEngine) TriggerAction(ctx context.Context, action ReflexAction) error {
	if e == nil {
		return ErrNilReflexEngine
	}
	if action.Type == "" {
		return ErrEmptyActionType
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return ErrReflexEngineClosed
	}

	// Buffer into bounded history
	if len(e.history) >= e.maxHistory {
		e.history = e.history[1:]
	}
	e.history = append(e.history, action)

	handler, hasHandler := e.handlers[action.Type]

	// Push non-blocking to bounded action queue
	select {
	case e.actionQueue <- action:
		e.actionCount.Add(1)
	default:
		e.failedCount.Add(1)
		e.mu.Unlock()
		return ErrActionQueueFull
	}
	e.mu.Unlock()

	if hasHandler && handler != nil {
		if err := handler(ctx, action); err != nil {
			e.failedCount.Add(1)
			return err
		}
		e.handledCount.Add(1)
	}
	return nil
}

func (e *SpinalCordReflexEngine) withMutationLock(fn func() error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrReflexEngineClosed
	}
	return fn()
}

// RegisterRule registers a trigger evaluation predicate.
func (e *SpinalCordReflexEngine) RegisterRule(rule ReflexTriggerPredicate) error {
	if e == nil {
		return ErrNilReflexEngine
	}
	if rule == nil {
		return ErrNilTriggerRule
	}
	return e.withMutationLock(func() error {
		e.rules = append(e.rules, rule)
		return nil
	})
}

// RegisterHandler registers a typed reflex action execution callback.
func (e *SpinalCordReflexEngine) RegisterHandler(actionType ReflexActionType, handler ReflexActionHandler) error {
	if e == nil {
		return ErrNilReflexEngine
	}
	if actionType == "" {
		return ErrEmptyActionType
	}
	if handler == nil {
		return ErrNilActionHandler
	}
	return e.withMutationLock(func() error {
		e.handlers[actionType] = handler
		return nil
	})
}

// Metrics returns snapshot metrics of reflex executions.
func (e *SpinalCordReflexEngine) Metrics() SpinalCordReflexMetrics {
	if e == nil {
		return SpinalCordReflexMetrics{ActionCounts: make(map[string]int64)}
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	var lastEval time.Time
	if ptr := e.lastEvalTime.Load(); ptr != nil {
		lastEval = *ptr
	}

	counts := make(map[string]int64)
	for _, a := range e.history {
		counts[string(a.Type)]++
	}

	return SpinalCordReflexMetrics{
		EvaluationsCount: e.evalCount.Load(),
		ActionsTriggered: e.actionCount.Load(),
		ActionsHandled:   e.handledCount.Load(),
		ActionsFailed:    e.failedCount.Load(),
		LastEvaluatedAt:  lastEval,
		ActionCounts:     counts,
	}
}

// History returns a copy of historical actions.
func (e *SpinalCordReflexEngine) History() []ReflexAction {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]ReflexAction(nil), e.history...)
}

// Close shuts down the engine and releases buffered channels.
func (e *SpinalCordReflexEngine) Close() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	close(e.actionQueue)
	return nil
}
