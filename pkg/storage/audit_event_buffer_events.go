package storage

import (
	"fmt"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
)

// ShouldAggregate checks if an event should be aggregated based on rules
func (b *AuditEventBuffer) ShouldAggregate(event map[string]any) bool {
	if !b.enabled {
		return false
	}

	eventType, _ := event[objects.FieldKeyEventType].(string)
	severity, _ := event[objects.FieldKeySeverity].(string)

	// Check each rule
	for _, rule := range b.rules {
		// Check event type
		eventTypeMatch := false
		for _, et := range rule.EventTypes {
			if et == eventType {
				eventTypeMatch = true
				break
			}
		}
		if !eventTypeMatch {
			continue
		}

		// Check severity
		severityMatch := false
		for _, sev := range rule.Severities {
			if sev == severity {
				severityMatch = true
				break
			}
		}
		if !severityMatch {
			continue
		}

		// Check if event has target_id (non-aggregatable per design)
		// Exception: scheduler job events can be aggregated even with target_id
		// because they're high-frequency and we want to reduce I/O
		isSchedulerEvent := eventType == EventTypeSchedulerJobStarted || eventType == EventTypeSchedulerJobCompleted

		if targetID := objects.GetString(event, objects.FieldKeyTargetID); targetID != emptyValue && !isSchedulerEvent {
			// Events with target_id are not aggregated (preserve individually)
			continue
		}

		// This event matches a rule - should be aggregated
		return true
	}

	return false
}

// AddEvent adds an event to the buffer if it should be aggregated
func (b *AuditEventBuffer) AddEvent(event map[string]any) error {
	if !b.ShouldAggregate(event) {
		// Event should not be aggregated - write immediately
		return nil
	}

	b.eventsAddedTotal.Add(1)

	var flushKey string
	var shouldFlush bool

	// Use TryLock to avoid deadlocks when audit events are created reentrantly
	// from the flush path (e.g. WriteSystemObjectAndRegisterHash -> AddEvent).
	// If we can't get the lock immediately, we return an error so the caller
	// can fall back to immediate write (avoiding both deadlock and data loss).
	var locked bool
	for retries := 0; retries < 10; retries++ {
		locked = b.mu.TryLock()
		if locked {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !locked {
		return errfmt.Errorf(ConstAuditAuditBufferIsBusyPossibleReentrantCallFromFlushPath)
	}

	defer b.mu.Unlock()

	var matchingRule *AggregationRule
	eventType, _ := event[objects.FieldKeyEventType].(string)
	severity, _ := event[objects.FieldKeySeverity].(string)

	for i := range b.rules {
		rule := &b.rules[i]
		eventTypeMatch := false
		for _, et := range rule.EventTypes {
			if et == eventType {
				eventTypeMatch = true
				break
			}
		}
		if !eventTypeMatch {
			continue
		}
		severityMatch := false
		for _, sev := range rule.Severities {
			if sev == severity {
				severityMatch = true
				break
			}
		}
		if !severityMatch {
			continue
		}
		matchingRule = rule
		break
	}

	if matchingRule == nil {
		return nil
	}

	key := b.generateAggregationKey(event, matchingRule.GroupBy)
	group, exists := b.buffer[key]
	if !exists {
		group = &AggregationGroup{
			Key:          key,
			EventType:    eventType,
			Severity:     severity,
			Count:        0,
			FirstSeen:    time.Now().UTC(),
			LastSeen:     time.Now().UTC(),
			SampleEvents: make([]map[string]any, 0, matchingRule.PreserveSamples),
			Events:       make([]map[string]any, 0),
		}
		if targetKind := objects.GetString(event, objects.FieldKeyTargetKind); targetKind != "" {
			group.TargetKind = targetKind
		}
		b.buffer[key] = group
	}

	group.Count++
	group.LastSeen = time.Now().UTC()
	group.Events = append(group.Events, event)

	if len(group.SampleEvents) < matchingRule.PreserveSamples {
		sampleEvent := make(map[string]any)
		for k, v := range event {
			sampleEvent[k] = v
		}
		group.SampleEvents = append(group.SampleEvents, sampleEvent)
	}

	if group.Count >= matchingRule.Threshold {
		flushKey = key
		shouldFlush = true
	}

	// Flush outside lock (async to avoid blocking)
	if shouldFlush {
		goroutinelabels.NewGoroutine(OpNameAuditEventBufferFlushGroup, fmt.Sprintf(DescFlushAggGroupFmt, flushKey)).
			StartSimple(func() {
				b.flushGroup(flushKey)
			})
	}

	return nil
}

// generateAggregationKey generates a key for grouping events
func (b *AuditEventBuffer) generateAggregationKey(event map[string]any, groupBy []string) string {
	parts := make([]string, 0, len(groupBy))
	for _, field := range groupBy {
		var value string
		switch field {
		case objects.FieldKeyEventType:
			value, _ = event[objects.FieldKeyEventType].(string)
		case objects.FieldKeyTargetKind:
			value, _ = event[objects.FieldKeyTargetKind].(string)
		case objects.FieldKeySeverity:
			value, _ = event[objects.FieldKeySeverity].(string)
		case objects.FieldKeyJobType:
			// Extract job_type from metadata if available
			if metadata, ok := event[objects.FieldKeyMetadata].(map[string]any); ok {
				if jobType := objects.GetString(metadata, objects.FieldKeyJobType); jobType != "" {
					value = jobType
				}
			}
		}
		if value == emptyValue {
			value = ValueUnknown
		}
		parts = append(parts, fmt.Sprintf("%s:%s", field, value))
	}
	return strings.Join(parts, "|")
}
