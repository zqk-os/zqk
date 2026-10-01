package transceiver

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
)

var (
	reRuleName  = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	reEventType = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// ValidateRoutingRule validates a routing rule configuration
//
//nolint:gocritic // rule passed by value to avoid shared mutation during validation
func ValidateRoutingRule(rule RoutingRule, router *Router) []ValidationError {
	var errors []ValidationError

	// Validate rule name
	if rule.Name == emptyValue {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "rule name is required",
		})
	} else if !isValidRuleName(rule.Name) {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("rule name '%s' contains invalid characters (use alphanumeric, underscore, hyphen)", rule.Name),
		})
	}

	// Validate priority (should be reasonable range)
	if rule.Priority < 0 || rule.Priority > 10000 {
		errors = append(errors, ValidationError{
			Field:   "priority",
			Message: fmt.Sprintf("priority %d is out of range (0-10000)", rule.Priority),
		})
	}

	// Validate actions
	if len(rule.Actions) == 0 {
		errors = append(errors, ValidationError{
			Field:   "actions",
			Message: "at least one action is required",
		})
	}

	// Validate each action
	for i, action := range rule.Actions {
		actionErrors := validateAction(action, router, i)
		errors = append(errors, actionErrors...)
	}

	// Validate matcher
	matcherErrors := validateMatcher(rule.Match)
	errors = append(errors, matcherErrors...)

	return errors
}

// validateAction validates an action configuration
func validateAction(action types.Action, router *Router, index int) []ValidationError {
	var errors []ValidationError
	prefix := fmt.Sprintf("actions[%d]", index)

	// Validate protocol
	if action.Protocol == emptyValue {
		errors = append(errors, ValidationError{
			Field:   prefix + ".protocol",
			Message: "protocol is required",
		})
	} else if router != nil && !router.HasAdapter(action.Protocol) {
		errors = append(errors, ValidationError{
			Field:   prefix + ".protocol",
			Message: fmt.Sprintf("protocol adapter '%s' not registered", action.Protocol),
		})
	}

	// Validate endpoint/target based on protocol
	switch action.Protocol {
	case "webhook":
		if action.Endpoint == emptyValue {
			errors = append(errors, ValidationError{
				Field:   prefix + ".endpoint",
				Message: "endpoint URL is required for webhook protocol",
			})
		} else if !isValidURL(action.Endpoint) {
			errors = append(errors, ValidationError{
				Field:   prefix + ".endpoint",
				Message: fmt.Sprintf("invalid URL format: %s", action.Endpoint),
			})
		}
	case "command":
		if action.Endpoint == emptyValue {
			errors = append(errors, ValidationError{
				Field:   prefix + ".endpoint",
				Message: "command name is required for command protocol",
			})
		}
	case "event":
		if action.Endpoint == emptyValue {
			errors = append(errors, ValidationError{
				Field:   prefix + ".endpoint",
				Message: "event name is required for event protocol",
			})
		}
	}

	// Validate retry configuration
	if action.Retry != nil {
		if action.Retry.MaxAttempts < 0 {
			errors = append(errors, ValidationError{
				Field:   prefix + ".retry.max_attempts",
				Message: "max_attempts must be >= 0",
			})
		}
		validBackoffTypes := []string{"exponential", "linear", "fixed"}
		if action.Retry.Backoff != emptyValue {
			valid := false
			for _, bt := range validBackoffTypes {
				if action.Retry.Backoff == bt {
					valid = true
					break
				}
			}
			if !valid {
				errors = append(errors, ValidationError{
					Field:   prefix + ".retry.backoff",
					Message: fmt.Sprintf("backoff must be one of: %s", strings.Join(validBackoffTypes, ", ")),
				})
			}
		}
		if action.Retry.InitialDelay < 0 {
			errors = append(errors, ValidationError{
				Field:   prefix + ".retry.initial_delay",
				Message: "initial_delay must be >= 0",
			})
		}
		if action.Retry.MaxDelay < 0 {
			errors = append(errors, ValidationError{
				Field:   prefix + ".retry.max_delay",
				Message: "max_delay must be >= 0",
			})
		}
	}

	// Validate timeout
	if action.Timeout < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".timeout",
			Message: "timeout must be >= 0",
		})
	}

	return errors
}

// validateMatcher validates a message matcher configuration
//
//nolint:gocritic // matcher passed by value; validation is read-only
func validateMatcher(matcher MessageMatcher) []ValidationError {
	var errors []ValidationError

	// Validate event type format (if provided)
	if matcher.EventType != emptyValue && !isValidEventType(matcher.EventType) {
		errors = append(errors, ValidationError{
			Field:   "match.event_type",
			Message: fmt.Sprintf("invalid event_type format: %s", matcher.EventType),
		})
	}

	// Validate conditions
	for i, condition := range matcher.Conditions {
		conditionErrors := validateCondition(condition, i)
		errors = append(errors, conditionErrors...)
	}

	return errors
}

// validateCondition validates a condition
func validateCondition(condition Condition, index int) []ValidationError {
	var errors []ValidationError
	prefix := fmt.Sprintf("match.conditions[%d]", index)

	// Validate field
	if condition.Field == emptyValue {
		errors = append(errors, ValidationError{
			Field:   prefix + ".field",
			Message: "field is required",
		})
	}

	// Validate operator
	validOperators := []string{"eq", "==", "ne", "!=", "gt", ">", "lt", "<", "gte", ">=", "lte", "<=", "contains", "regex", "in", "isNull"}
	valid := false
	for _, op := range validOperators {
		if condition.Operator == op {
			valid = true
			break
		}
	}
	if !valid {
		errors = append(errors, ValidationError{
			Field:   prefix + ".operator",
			Message: fmt.Sprintf("invalid operator '%s', must be one of: %s", condition.Operator, strings.Join(validOperators, ", ")),
		})
	}

	// Validate value (required for most operators)
	if condition.Operator != "isNull" && condition.Value == nil {
		errors = append(errors, ValidationError{
			Field:   prefix + ".value",
			Message: "value is required for this operator",
		})
	}

	return errors
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateRoutingRules validates multiple routing rules
func ValidateRoutingRules(rules []RoutingRule, router *Router, logger logging.Logger) []ValidationError {
	var allErrors []ValidationError
	ruleNames := make(map[string]bool)

	//nolint:gocritic // rangeValCopy: rules are read-only; copying acceptable
	for i, rule := range rules {
		// Check for duplicate names
		if ruleNames[rule.Name] {
			allErrors = append(allErrors, ValidationError{
				Field:   fmt.Sprintf("rules[%d].name", i),
				Message: fmt.Sprintf("duplicate rule name: %s", rule.Name),
			})
		}
		ruleNames[rule.Name] = true

		// Validate rule
		errors := ValidateRoutingRule(rule, router)
		for _, err := range errors {
			// Prefix with rule index
			allErrors = append(allErrors, ValidationError{
				Field:   fmt.Sprintf("rules[%d].%s", i, err.Field),
				Message: err.Message,
			})
		}
	}

	return allErrors
}

// Helper functions

func isValidRuleName(name string) bool {
	return reRuleName.MatchString(name)
}

func isValidEventType(eventType string) bool {
	// Event types should be lowercase with underscores
	return reEventType.MatchString(eventType)
}

func isValidURL(url string) bool {
	// Basic URL validation - check for http:// or https://
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return true
	}
	// Allow environment variable references
	if strings.HasPrefix(url, "${") && strings.HasSuffix(url, "}") {
		return true
	}
	return false
}
