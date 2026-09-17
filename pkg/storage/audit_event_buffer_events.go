package storage

import (
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/audit"
)

// ShouldAggregate checks if an event should be aggregated based on rules
func (b *AuditEventBuffer) ShouldAggregate(event map[string]any) bool {
	return audit.ShouldAggregate(b.enabled, b.rules, event)
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
	locked = audit.TryAcquire(b.mu.TryLock, audit.AddEventLockAttempts, audit.AddEventLockPause)
	if !locked {
		return errfmt.Errorf(ConstAuditAuditBufferIsBusyPossibleReentrantCallFromFlushPath)
	}

	defer b.mu.Unlock()

	matchingRule := audit.FirstMatchingRule(b.rules, event)
	if matchingRule == nil {
		return nil
	}

	now := time.Now().UTC()
	eventType, _ := event[objects.FieldKeyEventType].(string)
	severity, _ := event[objects.FieldKeySeverity].(string)
	key := audit.AggregationKey(event, matchingRule.GroupBy)
	group, exists := b.buffer[key]
	if !exists {
		group = audit.NewGroup(key, eventType, severity, event, matchingRule.PreserveSamples, now)
		b.buffer[key] = group
	}
	if audit.AppendEvent(group, event, matchingRule.PreserveSamples, matchingRule.Threshold, now) {
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
	return audit.AggregationKey(event, groupBy)
}
