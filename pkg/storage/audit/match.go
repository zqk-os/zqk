package audit

import "github.com/lanceman/zqk/pkg/objects"

// RuleMatches reports whether event matches one aggregation rule.
func RuleMatches(rule Rule, event map[string]any) bool {
	eventType, _ := event[objects.FieldKeyEventType].(string)
	severity, _ := event[objects.FieldKeySeverity].(string)

	eventTypeMatch := false
	for _, et := range rule.EventTypes {
		if et == eventType {
			eventTypeMatch = true
			break
		}
	}
	if !eventTypeMatch {
		return false
	}

	severityMatch := false
	for _, sev := range rule.Severities {
		if sev == severity {
			severityMatch = true
			break
		}
	}
	if !severityMatch {
		return false
	}

	if targetID := objects.GetString(event, objects.FieldKeyTargetID); targetID != "" {
		return false
	}
	return true
}

// ShouldAggregate reports whether event matches any rule while buffering is enabled.
func ShouldAggregate(enabled bool, rules []Rule, event map[string]any) bool {
	if !enabled {
		return false
	}
	for _, rule := range rules {
		if RuleMatches(rule, event) {
			return true
		}
	}
	return false
}
