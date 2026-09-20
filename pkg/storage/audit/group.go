package audit

import (
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

const UnknownValue = "unknown"

// FirstMatchingRule returns the first rule that matches event, or nil.
func FirstMatchingRule(rules []Rule, event map[string]any) *Rule {
	for i := range rules {
		if RuleMatches(rules[i], event) {
			return &rules[i]
		}
	}
	return nil
}

// AggregationKey builds the group key from GroupBy fields.
func AggregationKey(event map[string]any, groupBy []string) string {
	parts := make([]string, 0, len(groupBy))
	for _, field := range groupBy {
		value := groupByValue(event, field)
		if value == "" {
			value = UnknownValue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", field, value))
	}
	return strings.Join(parts, "|")
}

func groupByValue(event map[string]any, field string) string {
	switch field {
	case objects.FieldKeyEventType:
		value, _ := event[objects.FieldKeyEventType].(string)
		return value
	case objects.FieldKeyTargetKind:
		value, _ := event[objects.FieldKeyTargetKind].(string)
		return value
	case objects.FieldKeySeverity:
		value, _ := event[objects.FieldKeySeverity].(string)
		return value
	case objects.FieldKeyJobType:
		if metadata, ok := event[objects.FieldKeyMetadata].(map[string]any); ok {
			return objects.GetString(metadata, objects.FieldKeyJobType)
		}
	}
	return ""
}

// NewGroup starts an empty aggregation group for key.
func NewGroup(key, eventType, severity string, event map[string]any, preserveSamples int, now time.Time) *Group {
	g := &Group{
		Key:          key,
		EventType:    eventType,
		Severity:     severity,
		Count:        0,
		FirstSeen:    now,
		LastSeen:     now,
		SampleEvents: make([]map[string]any, 0, preserveSamples),
		Events:       make([]map[string]any, 0),
	}
	if targetKind := objects.GetString(event, objects.FieldKeyTargetKind); targetKind != "" {
		g.TargetKind = targetKind
	}
	return g
}

// AppendEvent records event on group. shouldFlush is true when Count reaches threshold.
func AppendEvent(group *Group, event map[string]any, preserveSamples, threshold int, now time.Time) (shouldFlush bool) {
	if group == nil {
		return false
	}
	group.Count++
	group.LastSeen = now
	group.Events = append(group.Events, event)
	if len(group.SampleEvents) < preserveSamples {
		sample := make(map[string]any, len(event))
		for k, v := range event {
			sample[k] = v
		}
		group.SampleEvents = append(group.SampleEvents, sample)
	}
	return group.Count >= threshold
}
