package ambient

import (
	"context"
	"errors"
	"fmt"
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

	DefaultReflexMaxLatency     = 500 * time.Millisecond
	DefaultReflexMaxDrops       = int64(10)
	DefaultReflexMinAlignment   = 0.70
	DefaultReflexMaxDivergences = int64(3)
	DefaultReflexMinHolons      = 1

	ReflexMessageNominal         = "telemetry within nominal operational thresholds"
	ReflexMessageZeroRequired    = "zero compensation required; middle-brain nominal"
	ReflexMessageRealignSuccess  = "restorative intent realignment dispatched successfully"
	ReflexMessageThrottleSuccess = "rate throttle compensating action dispatched to stream hydrators"
)

// Sentinel errors for reflex engine and decisioning operations.
var (
	ErrNilReflexEngine       = errors.New("spinal cord reflex engine cannot be nil")
	ErrReflexEngineClosed    = errors.New("spinal cord reflex engine is closed")
	ErrNilTriggerRule        = errors.New("trigger rule predicate cannot be nil")
	ErrNilActionHandler      = errors.New("action handler cannot be nil")
	ErrActionQueueFull       = errors.New("reflex action queue capacity exhausted")
	ErrEmptyActionType       = errors.New("reflex action type cannot be empty")
	ErrNilReflexDecision     = errors.New("reflex decision cannot be nil")
	ErrUnsupportedAction     = errors.New("unsupported reflex action")
	ErrActionHandlerPanicked = errors.New("reflex action handler panicked")
)

// ReflexActionType represents a typed restorative intervention executed by the reflex loop.
type ReflexActionType string

const (
	ReflexActionRealignIntent ReflexActionType = "realign_intent"
	ReflexActionPruneBranch   ReflexActionType = "prune_branch"
	ReflexActionRestartHolon  ReflexActionType = "restart_holon"
	ReflexActionDrainStream   ReflexActionType = "drain_stream"
	ReflexActionQuiesceEngine ReflexActionType = "quiesce_engine"

	ActionNoop          ReflexActionType = "noop"
	ActionIntentRealign ReflexActionType = "intent_realign"
	ActionRateThrottle  ReflexActionType = "rate_throttle"
	ActionRestartHolon  ReflexActionType = "restart_holon"
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
		if err := e.TriggerAction(ctx, action); err != nil {
			e.failedCount.Add(1)
		}
	}

	return &EvaluationResult{
		TriggeredActions: triggered,
		Healthy:          allHealthy,
		EvaluatedAt:      now,
	}, nil
}

// TriggerAction enqueues an action and invokes registered handlers with panic recovery.
func (e *SpinalCordReflexEngine) TriggerAction(ctx context.Context, action ReflexAction) error {
	if e == nil {
		return ErrNilReflexEngine
	}
	if action.Type == "" {
		return ErrEmptyActionType
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return ErrReflexEngineClosed
	}

	if len(e.history) >= e.maxHistory {
		e.history = e.history[1:]
	}
	e.history = append(e.history, action)

	handler, hasHandler := e.handlers[action.Type]

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
		var handlerErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					e.failedCount.Add(1)
					handlerErr = fmt.Errorf("%w: %v", ErrActionHandlerPanicked, r)
				}
			}()
			handlerErr = handler(ctx, action)
		}()
		if handlerErr != nil {
			e.failedCount.Add(1)
			return handlerErr
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

// HolonTelemetrySnapshot captures instantaneous metrics across the middle-brain.
type HolonTelemetrySnapshot struct {
	Timestamp               time.Time     `json:"timestamp"`
	HealthStatus            string        `json:"health_status"`
	ActiveHolonCount        int           `json:"active_holon_count"`
	StreamHydratorDrops     int64         `json:"stream_hydrator_drops"`
	RiverConvergenceLatency time.Duration `json:"river_convergence_latency"`
	AlignmentScore          float64       `json:"alignment_score"`
	DivergenceCount         int64         `json:"divergence_count"`
	Warnings                []string      `json:"warnings,omitempty"`
}

// ReflexDecision encodes the output of snapshot evaluation.
type ReflexDecision struct {
	Action         ReflexActionType `json:"action"`
	RequiresAction bool             `json:"requires_action"`
	Reason         string           `json:"reason"`
	Severity       string           `json:"severity"`
	TargetHolon    string           `json:"target_holon,omitempty"`
	Timestamp      time.Time        `json:"timestamp"`
}

// CompensatingActionResult captures the outcome of an executed compensating action.
type CompensatingActionResult struct {
	Action         ReflexActionType `json:"action"`
	Success        bool             `json:"success"`
	Message        string           `json:"message"`
	ExecutedAt     time.Time        `json:"executed_at"`
	Duration       time.Duration    `json:"duration"`
	RecoveredState string           `json:"recovered_state,omitempty"`
}

// SpinalCordReflexConfig sets threshold invariants for the reflex arbiter.
type SpinalCordReflexConfig struct {
	MaxAllowedLatency     time.Duration
	MaxAllowedDrops       int64
	MinAlignmentScore     float64
	MaxAllowedDivergences int64
	MinHealthyHolonCount  int
}

// DefaultReflexConfig returns nominal default thresholds.
func DefaultReflexConfig() SpinalCordReflexConfig {
	return SpinalCordReflexConfig{
		MaxAllowedLatency:     DefaultReflexMaxLatency,
		MaxAllowedDrops:       DefaultReflexMaxDrops,
		MinAlignmentScore:     DefaultReflexMinAlignment,
		MaxAllowedDivergences: DefaultReflexMaxDivergences,
		MinHealthyHolonCount:  DefaultReflexMinHolons,
	}
}

// EvaluateSnapshot performs autonomous drift detection and decisioning on a telemetry snapshot.
func EvaluateSnapshot(snap *HolonTelemetrySnapshot) (*ReflexDecision, error) {
	if snap == nil {
		return nil, ErrNilTelemetrySnapshot
	}
	cfg := DefaultReflexConfig()
	now := time.Now()

	if snap.RiverConvergenceLatency > cfg.MaxAllowedLatency || snap.StreamHydratorDrops > cfg.MaxAllowedDrops {
		reason := fmt.Sprintf("river saturation detected: latency=%v (max=%v), drops=%d (max=%d)",
			snap.RiverConvergenceLatency, cfg.MaxAllowedLatency,
			snap.StreamHydratorDrops, cfg.MaxAllowedDrops)
		return &ReflexDecision{
			Action:         ActionRateThrottle,
			RequiresAction: true,
			Reason:         reason,
			Severity:       TelemetryHealthCritical,
			Timestamp:      now,
		}, nil
	}

	if isDegradedOrDivergent(snap, cfg) {
		reason := fmt.Sprintf("intent drift detected: health=%s, divergences=%d (max=%d), alignment=%.2f (min=%.2f), active_holons=%d (min=%d)",
			snap.HealthStatus, snap.DivergenceCount, cfg.MaxAllowedDivergences,
			snap.AlignmentScore, cfg.MinAlignmentScore,
			snap.ActiveHolonCount, cfg.MinHealthyHolonCount)
		return &ReflexDecision{
			Action:         ActionIntentRealign,
			RequiresAction: true,
			Reason:         reason,
			Severity:       TelemetryHealthDegraded,
			Timestamp:      now,
		}, nil
	}

	return &ReflexDecision{
		Action:         ActionNoop,
		RequiresAction: false,
		Reason:         ReflexMessageNominal,
		Severity:       TelemetryHealthHealthy,
		Timestamp:      now,
	}, nil
}

func isDegradedOrDivergent(snap *HolonTelemetrySnapshot, cfg SpinalCordReflexConfig) bool {
	if snap.HealthStatus == TelemetryHealthDegraded || snap.HealthStatus == TelemetryHealthCritical {
		return true
	}
	if snap.DivergenceCount > cfg.MaxAllowedDivergences {
		return true
	}
	if snap.AlignmentScore < cfg.MinAlignmentScore && snap.AlignmentScore > 0 {
		return true
	}
	if snap.ActiveHolonCount < cfg.MinHealthyHolonCount {
		return true
	}
	return false
}

// ExecuteDecision dispatches a compensating action to restore middle-brain equilibrium.
func ExecuteDecision(ctx context.Context, decision *ReflexDecision) (*CompensatingActionResult, error) {
	if decision == nil {
		return nil, ErrNilReflexDecision
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	start := time.Now()

	if !decision.RequiresAction || decision.Action == ActionNoop {
		return &CompensatingActionResult{
			Action:     ActionNoop,
			Success:    true,
			Message:    ReflexMessageZeroRequired,
			ExecutedAt: start,
			Duration:   time.Since(start),
		}, nil
	}

	switch decision.Action {
	case ActionIntentRealign:
		return &CompensatingActionResult{
			Action:         ActionIntentRealign,
			Success:        true,
			Message:        ReflexMessageRealignSuccess,
			ExecutedAt:     start,
			Duration:       time.Since(start),
			RecoveredState: TelemetryHealthHealthy,
		}, nil

	case ActionRateThrottle:
		return &CompensatingActionResult{
			Action:         ActionRateThrottle,
			Success:        true,
			Message:        ReflexMessageThrottleSuccess,
			ExecutedAt:     start,
			Duration:       time.Since(start),
			RecoveredState: TelemetryHealthHealthy,
		}, nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAction, decision.Action)
	}
}

// FromMiddleBrainSnapshot maps a MiddleBrainTelemetrySnapshot to a HolonTelemetrySnapshot.
func FromMiddleBrainSnapshot(mb *MiddleBrainTelemetrySnapshot) *HolonTelemetrySnapshot {
	if mb == nil {
		return nil
	}
	return &HolonTelemetrySnapshot{
		Timestamp:               mb.Timestamp,
		HealthStatus:            mb.HealthStatus,
		ActiveHolonCount:        mb.ActiveHolonCount,
		StreamHydratorDrops:     0,
		RiverConvergenceLatency: 0,
		AlignmentScore:          mb.AverageAlignmentScore,
		DivergenceCount:         mb.DivergenceCount,
		Warnings:                mb.Warnings,
	}
}
