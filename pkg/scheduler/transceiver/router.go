package transceiver

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver/types"
	"github.com/lanceman/zqk/pkg/when"
)

// Re-export types for convenience
type Message = types.Message
type Action = types.Action
type PayloadTransform = types.PayloadTransform
type AuthConfig = types.AuthConfig
type RetryConfig = types.RetryConfig
type ProtocolAdapter = types.ProtocolAdapter

// RoutingRule defines how messages are routed
type RoutingRule struct {
	Name        string
	Description string
	Enabled     bool
	Match       MessageMatcher
	Actions     []types.Action
	Priority    int // Higher priority routes evaluated first
}

// MessageMatcher defines matching criteria for routing
type MessageMatcher struct {
	EventType   string      // Exact match or pattern (e.g., "scheduler_job_completed")
	Source      string      // Source filter (e.g., "scheduler")
	JobID       string      // Specific job ID (e.g., "SCH-002")
	JobCategory string      // Category filter (e.g., "maintenance")
	JobType     string      // Job type filter (e.g., "run_wrapper")
	Severity    string      // Severity filter (e.g., "high", "medium", "low")
	Conditions  []Condition // Complex field-based conditions
}

// Condition defines a field-based condition for matching
type Condition struct {
	Field    string // Field path in payload (e.g., "duration", "metadata.job_id")
	Operator string // eq, ne, gt, lt, gte, lte, contains, regex, in
	Value    any    // Comparison value
}

// Router is the main transceiver router
type Router struct {
	rules      []RoutingRule
	adapters   map[string]types.ProtocolAdapter
	rulesMu    sync.RWMutex
	adaptersMu sync.RWMutex
	logger     logging.Logger
	metrics    *RouterMetrics
}

// NewRouter creates a new transceiver router
func NewRouter(logger logging.Logger) *Router {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &Router{
		rules:    make([]RoutingRule, 0),
		adapters: make(map[string]ProtocolAdapter),
		logger:   logger,
		metrics:  NewRouterMetrics(),
	}
}

// GetMetrics returns the router metrics tracker
func (r *Router) GetMetrics() *RouterMetrics {
	return r.metrics
}

// RegisterAdapter registers a protocol adapter
func (r *Router) RegisterAdapter(adapter types.ProtocolAdapter) error {
	if adapter == nil {
		return errfmt.Errorf("adapter cannot be nil")
	}
	if adapter.Name() == emptyValue {
		return errfmt.Errorf("adapter name cannot be empty")
	}

	var exists bool
	err := concurrency.RunInLockWithLogger(
		&r.adaptersMu,
		LockNameRouterRegisterAdapter,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if _, exists = r.adapters[adapter.Name()]; exists {
				return nil
			}
			r.adapters[adapter.Name()] = adapter
			return nil
		},
	)
	if err != nil {
		return errfmt.Newf("failed to register adapter").Wrap(err)
	}
	if exists {
		return errfmt.Errorf("adapter already registered: %s", adapter.Name())
	}
	logging.Fluent(r.logger).Info(LogEventSchedulerTransceiverRouterRegisteredAdapter).
		Protocol(adapter.Name()).
		Log()
	return nil
}

// UnregisterAdapter removes a protocol adapter
func (r *Router) UnregisterAdapter(protocol string) {
	_ = concurrency.RunInLockWithLogger(
		&r.adaptersMu,
		LockNameRouterUnregisterAdapter,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(r.adapters, protocol)
			return nil
		},
	)
}

// LoadRules loads routing rules (from config, storage, etc.)
// Rules are sorted by priority (higher priority first)
func (r *Router) LoadRules(rules []RoutingRule) {
	_ = concurrency.RunInLockWithLogger(
		&r.rulesMu,
		LockNameRouterLoadRules,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			r.rules = make([]RoutingRule, len(rules))
			copy(r.rules, rules)

			// Sort by priority (higher priority first)
			sort.Slice(r.rules, func(i, j int) bool {
				return r.rules[i].Priority > r.rules[j].Priority
			})
			return nil
		},
	)

	logging.Fluent(r.logger).Info(LogEventSchedulerTransceiverRouterLoadedRules).
		Count(len(rules)).
		Log()
}

// GetRules returns a copy of current routing rules
func (r *Router) GetRules() []RoutingRule {
	var rules []RoutingRule
	_ = concurrency.RunInRLockWithLogger(
		&r.rulesMu,
		LockNameRouterGetRules,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			rules = make([]RoutingRule, len(r.rules))
			copy(rules, r.rules)
			return nil
		},
	)
	return rules
}

// Route routes a message through matching rules
// Returns error if all actions fail, but continues trying all matching rules
//
//nolint:gocritic // Message passed by value to keep original immutable across rules
func (r *Router) Route(ctx context.Context, message types.Message) error {
	startTime := time.Now()
	defer func() {
		r.metrics.RecordRoutingTime(time.Since(startTime))
	}()

	r.metrics.RecordMessageRouted()

	var rules []RoutingRule
	_ = concurrency.RunInRLockWithLogger(
		&r.rulesMu,
		LockNameRouterRouteGetRules,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			rules = make([]RoutingRule, len(r.rules))
			copy(rules, r.rules)
			return nil
		},
	)

	// Find matching rules
	var matchedRules []RoutingRule
	//nolint:gocritic // rangeValCopy: rule struct copy is acceptable for read-only evaluation
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		r.metrics.RecordRuleEvaluated()
		if r.matches(message, rule.Match) {
			matchedRules = append(matchedRules, rule)
			r.metrics.RecordRuleMatched()
			logging.Fluent(r.logger).Debug(LogEventSchedulerTransceiverRouterRuleMatched).
				EventType(message.EventType).
				RuleName(rule.Name).
				Log()
		}
	}

	if len(matchedRules) == 0 {
		logging.Fluent(r.logger).Debug(LogEventSchedulerTransceiverRouterNoRulesMatched).
			EventType(message.EventType).
			Log()
		return nil // No matches is not an error
	}

	r.metrics.RecordMessageMatched()

	// Execute actions for matched rules
	var lastErr error
	actionCount := 0
	for i := range matchedRules {
		rule := matchedRules[i]
		for _, action := range rule.Actions {
			actionCount++
			err := r.executeAction(ctx, message, action, rule.Name)
			success := err == nil
			r.metrics.RecordActionExecuted(action.Protocol, success)
			when.When(func() bool { return err != nil }).Then(func() {
				logging.Fluent(r.logger).Warn(LogEventSchedulerTransceiverRouterActionFailed).
					RuleName(rule.Name).
					Protocol(action.Protocol).
					WithError(err).
					Log()
				lastErr = err
				// Continue with other actions (best-effort delivery)
			}).OrElse(func() {
				logging.Fluent(r.logger).Debug(LogEventSchedulerTransceiverRouterActionSucceeded).
					RuleName(rule.Name).
					Protocol(action.Protocol).
					Log()
			}).Run()
		}
	}

	if lastErr != nil && actionCount > 0 {
		r.metrics.RecordMessageFailed()
		// Return error if at least one action failed
		return errfmt.Errorf("some actions failed (last error: %w)", lastErr)
	}

	return nil
}

// matches checks if a message matches the matcher criteria
func (r *Router) matches(message types.Message, matcher MessageMatcher) bool {
	// Event type matching
	if matcher.EventType != emptyValue && message.EventType != matcher.EventType {
		return false
	}

	// Source matching
	if matcher.Source != emptyValue && message.Source != matcher.Source {
		return false
	}

	// Job ID matching (from metadata)
	if matcher.JobID != emptyValue {
		if jobID, ok := message.Metadata["job_id"]; !ok || jobID != matcher.JobID {
			return false
		}
	}

	// Job category matching (from metadata)
	if matcher.JobCategory != emptyValue {
		if category, ok := message.Metadata[objects.FieldKeyCategory]; !ok || category != matcher.JobCategory {
			return false
		}
	}

	// Job type matching (from metadata)
	if matcher.JobType != emptyValue {
		if jobType, ok := message.Metadata[objects.FieldKeyJobType]; !ok || jobType != matcher.JobType {
			return false
		}
	}

	// Severity matching (from metadata)
	if matcher.Severity != emptyValue {
		if severity, ok := message.Metadata[objects.FieldKeySeverity]; !ok || severity != matcher.Severity {
			return false
		}
	}

	// Condition matching
	for _, condition := range matcher.Conditions {
		if !r.evaluateCondition(message, condition) {
			return false
		}
	}

	return true
}

// evaluateCondition evaluates a single condition against a message
func (r *Router) evaluateCondition(message types.Message, condition Condition) bool {
	// Get field value from payload or metadata
	var fieldValue any
	var exists bool

	// Check metadata first (common fields)
	if val, ok := message.Metadata[condition.Field]; ok {
		fieldValue = val
		exists = true
	} else if val, ok := message.Payload[condition.Field]; ok {
		fieldValue = val
		exists = true
	}

	if !exists {
		// Field doesn't exist - most operators will fail
		return condition.Operator == "isNull" || condition.Operator == "ne"
	}

	// Evaluate operator
	switch condition.Operator {
	case "eq", "==":
		return fieldValue == condition.Value
	case "ne", "!=":
		return fieldValue != condition.Value
	case "gt", ">":
		return r.compareValues(fieldValue, condition.Value) > 0
	case "lt", "<":
		return r.compareValues(fieldValue, condition.Value) < 0
	case "gte", ">=":
		return r.compareValues(fieldValue, condition.Value) >= 0
	case "lte", "<=":
		return r.compareValues(fieldValue, condition.Value) <= 0
	case "contains":
		// String contains
		if str, ok := fieldValue.(string); ok {
			if valStr, ok := condition.Value.(string); ok {
				return contains(str, valStr)
			}
		}
		return false
	case "regex":
		// Regular expression match
		if str, ok := fieldValue.(string); ok {
			if pattern, ok := condition.Value.(string); ok {
				re, err := getCachedRegexp(pattern)
				if err != nil {
					// Invalid patterns should be caught during validation, but be defensive.
					return false
				}
				return re.MatchString(str)
			}
		}
		return false
	case "in":
		// Value in array
		if arr, ok := condition.Value.([]any); ok {
			return slices.Contains(arr, fieldValue)
		}
		return false
	case "isNull":
		return fieldValue == nil
	default:
		logging.Fluent(r.logger).Warn(LogEventSchedulerTransceiverRouterUnknownOperator).
			String("operator", condition.Operator).
			Log()
		return false
	}
}

// compareValues compares two values numerically
func (r *Router) compareValues(a, b any) int {
	// Try to convert to float64 for numeric comparison
	aFloat, aOk := r.toFloat64(a)
	bFloat, bOk := r.toFloat64(b)

	if aOk && bOk {
		if aFloat < bFloat {
			return -1
		} else if aFloat > bFloat {
			return 1
		}
		return 0
	}

	// Fallback to string comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	if aStr < bStr {
		return -1
	} else if aStr > bStr {
		return 1
	}
	return 0
}

// toFloat64 attempts to convert a value to float64
func (r *Router) toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	default:
		return 0, false
	}
}

// executeAction executes an action using the appropriate adapter
//
//nolint:gocritic // Message passed by value to avoid mutation across adapters
func (r *Router) executeAction(ctx context.Context, message types.Message, action types.Action, ruleName string) error {
	// Get adapter
	var adapter ProtocolAdapter
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&r.adaptersMu,
		LockNameRouterGetAdapter,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			adapter, exists = r.adapters[action.Protocol]
			return nil
		},
	)

	if !exists {
		return errfmt.Errorf("protocol adapter not found: %s", action.Protocol)
	}

	// Validate action
	if err := adapter.Validate(action); err != nil {
		return errfmt.Newf("invalid action configuration").Wrap(err)
	}

	// Apply payload transformation if configured
	transformedMessage := message
	if action.Transform != nil {
		transformedMessage = r.transformPayload(message, action.Transform)
	}

	// Execute with retry if configured
	if action.Retry != nil && action.Retry.MaxAttempts > 0 {
		return r.executeWithRetry(ctx, adapter, transformedMessage, action, ruleName)
	}

	// Execute with timeout
	if action.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, action.Timeout)
		defer cancel()
	}

	return adapter.Send(ctx, transformedMessage, action)
}

// transformPayload applies transformation rules to message payload
//
//nolint:gocritic // Message passed by value to keep original immutable
func (r *Router) transformPayload(message types.Message, transform *types.PayloadTransform) types.Message {
	result := types.Message{
		EventType:   message.EventType,
		Source:      message.Source,
		Destination: message.Destination,
		Timestamp:   message.Timestamp,
		Payload:     make(map[string]any),
		Metadata:    make(map[string]string),
	}

	// Copy metadata
	for k, v := range message.Metadata {
		result.Metadata[k] = v
	}

	// Include/exclude fields
	if len(transform.IncludeFields) > 0 {
		// Only include specified fields
		for _, field := range transform.IncludeFields {
			if val, ok := message.Payload[field]; ok {
				result.Payload[field] = val
			}
		}
	} else {
		// Include all fields (then exclude)
		for k, v := range message.Payload {
			result.Payload[k] = v
		}
	}

	// Exclude fields
	for _, field := range transform.ExcludeFields {
		delete(result.Payload, field)
	}

	// Rename fields
	for oldName, newName := range transform.RenameFields {
		if val, ok := result.Payload[oldName]; ok {
			result.Payload[newName] = val
			delete(result.Payload, oldName)
		}
	}

	// Add fields
	for k, v := range transform.AddFields {
		result.Payload[k] = v
	}

	return result
}

// executeWithRetry executes an action with retry logic
//
//nolint:gocritic // Message passed by value to avoid shared mutation during retries
func (r *Router) executeWithRetry(ctx context.Context, adapter types.ProtocolAdapter, message types.Message, action types.Action, ruleName string) error {
	maxAttempts := action.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	delay := action.Retry.InitialDelay
	if delay == 0 {
		delay = 1 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			logging.Fluent(r.logger).Debug(LogEventSchedulerTransceiverRouterRetryingAction).
				RuleName(ruleName).
				Protocol(action.Protocol).
				RetryAttempt(attempt + 1).
				RetryMaxAttempts(maxAttempts).
				RetryDelay(delay.String()).
				Log()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				// Continue with retry
			}

			// Calculate next delay based on backoff strategy
			delay = r.calculateBackoff(delay, action.Retry)
		}

		// Execute with timeout
		actionCtx := ctx
		when.When(func() bool { return action.Timeout > 0 }).Then(func() {
			var cancel context.CancelFunc
			actionCtx, cancel = context.WithTimeout(ctx, action.Timeout)
			err := adapter.Send(actionCtx, message, action)
			cancel()
			lastErr = err
		}).OrElse(func() {
			lastErr = adapter.Send(actionCtx, message, action)
		}).Run()

		if lastErr == nil {
			// Success
			return nil
		}

		// Check if context was cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	// All retries exhausted
	return errfmt.Errorf("action failed after %d attempts: %w", maxAttempts, lastErr)
}

// calculateBackoff calculates the next delay based on backoff strategy
func (r *Router) calculateBackoff(currentDelay time.Duration, retry *types.RetryConfig) time.Duration {
	maxDelay := retry.MaxDelay
	if maxDelay == 0 {
		maxDelay = 30 * time.Second
	}

	switch retry.Backoff {
	case "exponential":
		nextDelay := currentDelay * 2
		if nextDelay > maxDelay {
			return maxDelay
		}
		return nextDelay
	case "linear":
		nextDelay := currentDelay + retry.InitialDelay
		if nextDelay > maxDelay {
			return maxDelay
		}
		return nextDelay
	case "fixed":
		return currentDelay
	default:
		// Default to exponential
		nextDelay := currentDelay * 2
		if nextDelay > maxDelay {
			return maxDelay
		}
		return nextDelay
	}
}

// contains checks if a string contains a substring (case-sensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == emptyValue || indexOfSubstring(s, substr) >= 0)
}

// indexOfSubstring finds the index of a substring (simple implementation)
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
