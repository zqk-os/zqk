package ambient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Named constants for capacity limits, history limits, debounce intervals, and action types.
const (
	// DefaultReflexCapacity defines the default bounded queue capacity for reflex events.
	DefaultReflexCapacity = 256

	// DefaultReflexHistoryLimit defines the default ring buffer retention limit for reflex events.
	DefaultReflexHistoryLimit = 256

	// DefaultReflexTimeout defines the default execution deadline for reflex action handlers.
	DefaultReflexTimeout = 5 * time.Second

	// DefaultReflexDebounceInterval defines the minimum interval between identical trigger activations.
	DefaultReflexDebounceInterval = 1 * time.Second

	// Action types for autonomous compensating actions.
	ReflexActionSweepBranches   = "sweep_branches"
	ReflexActionReplenishIntent = "replenish_intent"
	ReflexActionQuarantineHolon = "quarantine_holon"
	ReflexActionEmitAlert       = "emit_alert"

	// Trigger conditions evaluated against middle-brain telemetry snapshots.
	ReflexTriggerDegradedHealth     = "degraded_health"
	ReflexTriggerCriticalHealth     = "critical_health"
	ReflexTriggerDivergenceExceeded = "divergence_exceeded"
)

// Named sentinel errors for middle-brain reflex loop orchestration.
var (
	ErrNilSnapshot             = errors.New("reflex loop: telemetry snapshot cannot be nil")
	ErrNilActionHandler        = errors.New("reflex loop: action handler cannot be nil")
	ErrEmptyActionType         = errors.New("reflex loop: action type cannot be empty")
	ErrDuplicateHandler        = errors.New("reflex loop: handler already registered for action type")
	ErrHandlerNotFound         = errors.New("reflex loop: handler not found for action type")
	ErrEmptyTrigger            = errors.New("reflex loop: trigger condition cannot be empty")
	ErrNilPredicate            = errors.New("reflex loop: trigger predicate cannot be nil")
	ErrReflexEngineClosed      = errors.New("reflex loop: engine is closed")
	ErrReflexChannelExhausted  = errors.New("reflex loop: action channel capacity exhausted")
	ErrReflexHolonUnregistered = errors.New("reflex loop: holon unregistered in middle-brain")
)

// ReflexEvent represents an instantiated reflex trigger and resulting action intent.
type ReflexEvent struct {
	ID        string         `json:"id"`
	Trigger   string         `json:"trigger"`
	Action    string         `json:"action"`
	Reason    string         `json:"reason"`
	Timestamp time.Time      `json:"timestamp"`
	HolonID   string         `json:"holon_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// String provides a human-readable representation of the reflex event.
func (e ReflexEvent) String() string {
	return fmt.Sprintf("ReflexEvent[id=%s, trigger=%s, action=%s, reason=%s]", e.ID, e.Trigger, e.Action, e.Reason)
}

// ReflexTriggerPredicate evaluates whether an ambient telemetry snapshot satisfies a trigger condition.
type ReflexTriggerPredicate func(snapshot *MiddleBrainTelemetrySnapshot) bool

// ReflexActionHandler handles an autonomous compensating reflex action.
type ReflexActionHandler func(ctx context.Context, event ReflexEvent) error

// ReflexEngineOption configures SpinalCordReflexEngine construction.
type ReflexEngineOption func(*SpinalCordReflexEngine)

// WithReflexCapacity configures the event channel capacity.
func WithReflexCapacity(capacity int) ReflexEngineOption {
	return func(e *SpinalCordReflexEngine) {
		if capacity > 0 {
			e.capacity = capacity
		}
	}
}

// WithReflexTimeout configures the execution deadline for action handlers.
func WithReflexTimeout(timeout time.Duration) ReflexEngineOption {
	return func(e *SpinalCordReflexEngine) {
		if timeout > 0 {
			e.timeout = timeout
		}
	}
}

// WithReflexDebounceInterval configures the trigger debounce duration.
func WithReflexDebounceInterval(interval time.Duration) ReflexEngineOption {
	return func(e *SpinalCordReflexEngine) {
		if interval >= 0 {
			e.debounceInterval = interval
		}
	}
}

// WithReflexHistoryLimit configures the history buffer retention size.
func WithReflexHistoryLimit(limit int) ReflexEngineOption {
	return func(e *SpinalCordReflexEngine) {
		if limit > 0 {
			e.historyLimit = limit
		}
	}
}

// WithReflexHandler registers an initial action handler.
func WithReflexHandler(actionType string, handler ReflexActionHandler) ReflexEngineOption {
	return func(e *SpinalCordReflexEngine) {
		if strings.TrimSpace(actionType) != "" && handler != nil {
			e.handlers[strings.TrimSpace(actionType)] = handler
		}
	}
}

type reflexRule struct {
	trigger   string
	action    string
	predicate ReflexTriggerPredicate
	reason    string
}

// SpinalCordReflexEngine implements autonomous middle-brain reflex loop orchestration,
// evaluating telemetry snapshots against registered trigger conditions and dispatching
// compensating actions to restore system equilibrium.
type SpinalCordReflexEngine struct {
	mu               sync.RWMutex
	capacity         int
	timeout          time.Duration
	debounceInterval time.Duration
	historyLimit     int
	handlers         map[string]ReflexActionHandler
	rules            []reflexRule
	history          []ReflexEvent
	lastTriggered    map[string]time.Time
	actionCh         chan ReflexEvent
	closed           bool
}

// NewSpinalCordReflexEngine creates a new SpinalCordReflexEngine configured with defaults
// and overridden by any provided ReflexEngineOption.
func NewSpinalCordReflexEngine(opts ...ReflexEngineOption) *SpinalCordReflexEngine {
	engine := &SpinalCordReflexEngine{
		capacity:         DefaultReflexCapacity,
		timeout:          DefaultReflexTimeout,
		debounceInterval: DefaultReflexDebounceInterval,
		historyLimit:     DefaultReflexHistoryLimit,
		handlers:         make(map[string]ReflexActionHandler),
		rules:            make([]reflexRule, 0),
		history:          make([]ReflexEvent, 0, DefaultReflexHistoryLimit),
		lastTriggered:    make(map[string]time.Time),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(engine)
		}
	}

	engine.actionCh = make(chan ReflexEvent, engine.capacity)
	engine.registerDefaultRules()

	return engine
}

func (e *SpinalCordReflexEngine) registerDefaultRules() {
	e.rules = []reflexRule{
		{
			trigger: ReflexTriggerCriticalHealth,
			action:  ReflexActionQuarantineHolon,
			reason:  "critical holon health detected in middle-brain telemetry",
			predicate: func(s *MiddleBrainTelemetrySnapshot) bool {
				return s != nil && s.IsCritical()
			},
		},
		{
			trigger: ReflexTriggerDivergenceExceeded,
			action:  ReflexActionSweepBranches,
			reason:  "intent divergence count exceeded safe operational threshold",
			predicate: func(s *MiddleBrainTelemetrySnapshot) bool {
				return s != nil && s.DivergenceCount > MaxAllowedDivergences
			},
		},
		{
			trigger: ReflexTriggerDegradedHealth,
			action:  ReflexActionReplenishIntent,
			reason:  "degraded middle-brain health status detected",
			predicate: func(s *MiddleBrainTelemetrySnapshot) bool {
				return s != nil && s.IsDegraded()
			},
		},
	}
}

// RegisterHandler binds a handler callback to a named reflex action type in a thread-safe manner.
func (e *SpinalCordReflexEngine) RegisterHandler(actionType string, handler ReflexActionHandler) error {
	trimmed := strings.TrimSpace(actionType)
	if trimmed == "" {
		return ErrEmptyActionType
	}
	if handler == nil {
		return ErrNilActionHandler
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrReflexEngineClosed
	}

	if _, exists := e.handlers[trimmed]; exists {
		return ErrDuplicateHandler
	}

	e.handlers[trimmed] = handler
	return nil
}

// UnregisterHandler removes a handler callback for a named reflex action type.
func (e *SpinalCordReflexEngine) UnregisterHandler(actionType string) error {
	trimmed := strings.TrimSpace(actionType)
	if trimmed == "" {
		return ErrEmptyActionType
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrReflexEngineClosed
	}

	if _, exists := e.handlers[trimmed]; !exists {
		return ErrHandlerNotFound
	}

	delete(e.handlers, trimmed)
	return nil
}

// GetHandler retrieves the registered handler for an action type if present.
func (e *SpinalCordReflexEngine) GetHandler(actionType string) (ReflexActionHandler, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	handler, exists := e.handlers[strings.TrimSpace(actionType)]
	return handler, exists
}

// RegisteredActions returns a list of all currently registered action types.
func (e *SpinalCordReflexEngine) RegisteredActions() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	actions := make([]string, 0, len(e.handlers))
	for k := range e.handlers {
		actions = append(actions, k)
	}
	return actions
}

// RegisterRule dynamically appends a custom trigger-action evaluation rule.
func (e *SpinalCordReflexEngine) RegisterRule(trigger, action, reason string, predicate ReflexTriggerPredicate) error {
	trigger = strings.TrimSpace(trigger)
	action = strings.TrimSpace(action)
	if trigger == "" {
		return ErrEmptyTrigger
	}
	if action == "" {
		return ErrEmptyActionType
	}
	if predicate == nil {
		return ErrNilPredicate
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrReflexEngineClosed
	}

	e.rules = append(e.rules, reflexRule{
		trigger:   trigger,
		action:    action,
		predicate: predicate,
		reason:    reason,
	})
	return nil
}

// EvaluateSnapshot inspects a telemetry snapshot against all configured trigger rules,
// applies debounce filtering, records reflex events, and emits them to the action channel.
func (e *SpinalCordReflexEngine) EvaluateSnapshot(ctx context.Context, snapshot *MiddleBrainTelemetrySnapshot) ([]ReflexEvent, error) {
	if snapshot == nil {
		return nil, ErrNilSnapshot
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, ErrReflexEngineClosed
	}

	now := time.Now()
	var triggeredEvents []ReflexEvent

	for _, rule := range e.rules {
		if rule.predicate == nil || !rule.predicate(snapshot) {
			continue
		}

		// Debounce check for identical trigger
		if e.debounceInterval > 0 {
			if last, exists := e.lastTriggered[rule.trigger]; exists {
				if now.Sub(last) < e.debounceInterval {
					continue
				}
			}
		}

		event := ReflexEvent{
			ID:        fmt.Sprintf("rflx-%d", now.UnixNano()),
			Trigger:   rule.trigger,
			Action:    rule.action,
			Reason:    rule.reason,
			Timestamp: now,
			Metadata: map[string]any{
				"health_status":    snapshot.HealthStatus,
				"divergence_count": snapshot.DivergenceCount,
				"active_holons":    snapshot.ActiveHolonCount,
			},
		}

		// Extract target holon when quarantining if available
		if rule.action == ReflexActionQuarantineHolon {
			for _, h := range snapshot.Holons {
				if h.Phase == HolonPhaseFailed || h.Phase == HolonPhaseStopped {
					event.HolonID = h.ID
					break
				}
			}
		}

		e.lastTriggered[rule.trigger] = now
		e.appendHistoryLocked(event)
		triggeredEvents = append(triggeredEvents, event)

		// Non-blocking emit to action channel to prevent backpressure hangs
		select {
		case e.actionCh <- event:
		default:
			// Buffer full: fail-safe drop prevents goroutine lockup
		}
	}

	return triggeredEvents, nil
}

// ExecuteAction synchronously invokes the registered handler for the event's action type
// within the engine's configured timeout deadline.
func (e *SpinalCordReflexEngine) ExecuteAction(ctx context.Context, event ReflexEvent) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return ErrReflexEngineClosed
	}
	handler, exists := e.handlers[event.Action]
	timeout := e.timeout
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("%w: %s", ErrHandlerNotFound, event.Action)
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return handler(execCtx, event)
}

// EmitAction delivers an event to the internal action channel or returns ErrReflexChannelExhausted if full.
func (e *SpinalCordReflexEngine) EmitAction(event ReflexEvent) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return ErrReflexEngineClosed
	}

	select {
	case e.actionCh <- event:
		return nil
	default:
		return ErrReflexChannelExhausted
	}
}

// ActionChannel exposes the receive-only channel of reflex actions.
func (e *SpinalCordReflexEngine) ActionChannel() <-chan ReflexEvent {
	return e.actionCh
}

// History returns a snapshot copy of all recorded reflex events.
func (e *SpinalCordReflexEngine) History() []ReflexEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]ReflexEvent, len(e.history))
	copy(out, e.history)
	return out
}

// HistoryLimit returns the configured maximum history retention.
func (e *SpinalCordReflexEngine) HistoryLimit() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.historyLimit
}

// Capacity returns the action queue capacity.
func (e *SpinalCordReflexEngine) Capacity() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.capacity
}

// Timeout returns the configured action handler timeout.
func (e *SpinalCordReflexEngine) Timeout() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.timeout
}

// DebounceInterval returns the configured debounce interval.
func (e *SpinalCordReflexEngine) DebounceInterval() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.debounceInterval
}

// ResetDebounce clears the debounce timer for a trigger condition, or for all triggers if empty.
func (e *SpinalCordReflexEngine) ResetDebounce(trigger string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if trigger == "" {
		e.lastTriggered = make(map[string]time.Time)
	} else {
		delete(e.lastTriggered, trigger)
	}
}

func (e *SpinalCordReflexEngine) appendHistoryLocked(event ReflexEvent) {
	if len(e.history) >= e.historyLimit {
		e.history = e.history[1:]
	}
	e.history = append(e.history, event)
}

// Close closes the reflex engine and shuts down the action channel.
func (e *SpinalCordReflexEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true
	close(e.actionCh)
	return nil
}

// IsClosed returns true if the engine has been shut down.
func (e *SpinalCordReflexEngine) IsClosed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closed
}
