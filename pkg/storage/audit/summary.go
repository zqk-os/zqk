package audit

import (
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	RollupKeyCount           = "count"
	RollupKeyFirstOccurrence = "first_occurrence"
	RollupKeyLastOccurrence  = "last_occurrence"
	MetaKeyAggregationKey    = "aggregation_key"
	SourceBuffer             = "audit_event_buffer"
	AggregatedEventsFmt      = "Aggregated %d %s events"
	ActorSystem              = "system"
)

// WindowString is the aggregation_window value for a group.
func WindowString(group *Group) string {
	if group == nil {
		return ""
	}
	return fmt.Sprintf("%s to %s",
		zqktime.FormatRFC3339UTC(group.FirstSeen),
		zqktime.FormatRFC3339UTC(group.LastSeen))
}

// OperationString is the human-readable operation for an aggregated_summary.
func OperationString(group *Group) string {
	if group == nil {
		return ""
	}
	operation := fmt.Sprintf(AggregatedEventsFmt, group.Count, group.EventType)
	if group.TargetKind != "" {
		operation += fmt.Sprintf(" for %s", group.TargetKind)
	}
	return operation
}

// AggregatedSummaryMap builds the aggregated_summary instance map. id must already be allocated.
func AggregatedSummaryMap(id, key, actor string, group *Group, now time.Time) map[string]any {
	if group == nil {
		return nil
	}
	if actor == "" {
		actor = ActorSystem
	}
	aggregatedEvents := []map[string]any{{
		objects.FieldKeyEventType: group.EventType,
		RollupKeyCount:            group.Count,
		RollupKeyFirstOccurrence:  zqktime.FormatRFC3339UTC(group.FirstSeen),
		RollupKeyLastOccurrence:   zqktime.FormatRFC3339UTC(group.LastSeen),
	}}
	if group.TargetKind != "" {
		aggregatedEvents[0][objects.FieldKeyTargetKind] = group.TargetKind
	}
	createdAt := now.Format(time.RFC3339)
	out := map[string]any{
		objects.FieldKeyID:                id,
		objects.FieldKeyKind:              objects.KindAuditEvent,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:         createdAt,
		objects.FieldKeyCreatedBy:         actor,
		objects.FieldKeyUpdatedAt:         createdAt,
		objects.FieldKeyUpdatedBy:         actor,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyStatus:            StatusCompleted,
		objects.FieldKeyEventType:         EventTypeAggregatedSummary,
		objects.FieldKeyOperation:         OperationString(group),
		objects.FieldKeyTargetKind:        group.TargetKind,
		objects.FieldKeySeverity:          group.Severity,
		objects.FieldKeyAggregationWindow: WindowString(group),
		objects.FieldKeyAggregatedCount:   group.Count,
		objects.FieldKeyAggregatedEvents:  aggregatedEvents,
		objects.FieldKeyMetadata: map[string]any{
			objects.FieldKeySource:           SourceBuffer,
			MetaKeyAggregationKey:            key,
			objects.FieldKeyPreservedSamples: len(group.SampleEvents),
		},
	}
	if len(group.SampleEvents) > 0 {
		out[objects.FieldKeyPreservedSamples] = group.SampleEvents
	}
	return out
}
