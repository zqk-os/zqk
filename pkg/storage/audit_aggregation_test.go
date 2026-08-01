package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestAuditAggregationService_CompressEventIDs(t *testing.T) {
	service := &AuditAggregationService{}

	tests := []struct {
		name     string
		events   []map[string]any
		expected []string
	}{
		{
			name: "consecutive IDs compressed to range",
			events: []map[string]any{
				{objects.FieldKeyID: "AUD-100"},
				{objects.FieldKeyID: "AUD-101"},
				{objects.FieldKeyID: "AUD-102"},
				{objects.FieldKeyID: "AUD-103"},
			},
			expected: []string{"AUD-100..AUD-103"},
		},
		{
			name: "non-consecutive IDs remain individual",
			events: []map[string]any{
				{objects.FieldKeyID: "AUD-1"},
				{objects.FieldKeyID: "AUD-5"},
				{objects.FieldKeyID: "AUD-10"},
			},
			expected: []string{"AUD-1", "AUD-5", "AUD-10"},
		},
		{
			name: "mixed consecutive and non-consecutive",
			events: []map[string]any{
				{objects.FieldKeyID: "AUD-1"},
				{objects.FieldKeyID: "AUD-100"},
				{objects.FieldKeyID: "AUD-101"},
				{objects.FieldKeyID: "AUD-102"},
				{objects.FieldKeyID: "AUD-250"},
			},
			expected: []string{"AUD-1", "AUD-100..AUD-102", "AUD-250"},
		},
		{
			name: "two consecutive IDs remain individual",
			events: []map[string]any{
				{objects.FieldKeyID: "AUD-100"},
				{objects.FieldKeyID: "AUD-101"},
			},
			expected: []string{"AUD-100", "AUD-101"},
		},
		{
			name: "large range",
			events: func() []map[string]any {
				events := make([]map[string]any, 0, 100)
				for i := 1; i <= 100; i++ {
					events = append(events, map[string]any{objects.FieldKeyID: fmt.Sprintf("AUD-%d", i)})
				}
				return events
			}(),
			expected: []string{"AUD-1..AUD-100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.compressEventIDs(tt.events)
			if len(result) != len(tt.expected) {
				t.Errorf("compressEventIDs() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("compressEventIDs()[%d] = %v, want %v", i, result[i], expected)
				}
			}
		})
	}
}

// toInt converts map value to int (builder may store int; JSON round-trip uses float64)
func toInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func TestAuditAggregationService_AggregateEvents(t *testing.T) {
	service := &AuditAggregationService{}

	windowStart := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2025, 12, 1, 23, 59, 59, 0, time.UTC)

	events := []map[string]any{
		{
			objects.FieldKeyID:         "AUD-1",
			objects.FieldKeyEventType:  "hash_regeneration",
			objects.FieldKeyStatus:     "completed",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeyOperation:  "Regenerated hash for ITEM-001",
		},
		{
			objects.FieldKeyID:         "AUD-2",
			objects.FieldKeyEventType:  "hash_regeneration",
			objects.FieldKeyStatus:     "failed",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeyOperation:  "Regenerated hash for ITEM-002",
		},
		{
			objects.FieldKeyID:         "AUD-3",
			objects.FieldKeyEventType:  "integrity_recovery",
			objects.FieldKeyStatus:     "error",
			objects.FieldKeyTargetKind: "goal",
			objects.FieldKeyOperation:  "Recovered integrity for GOAL-001",
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	metric, eventIDs, err := service.aggregateEvents(ctx, secCtx, events, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("aggregateEvents failed: %v", err)
	}

	// Verify metric structure
	if metric[objects.FieldKeyKind] != objects.KindAuditAggregationMetric {
		t.Errorf("metric kind = %v, want %s", metric[objects.FieldKeyKind], objects.KindAuditAggregationMetric)
	}

	if metric[objects.FieldKeyEventCount] != 3 {
		t.Errorf("metric event_count = %v, want 3", metric[objects.FieldKeyEventCount])
	}

	// Verify event type counts (builder returns map[string]any; values may be int or float64 from JSON)
	eventTypeCountsAny, ok := metric[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatal("event_type_counts is not a map[string]any")
	}
	if toInt(eventTypeCountsAny["hash_regeneration"]) != 2 {
		t.Errorf("hash_regeneration count = %d, want 2", toInt(eventTypeCountsAny["hash_regeneration"]))
	}
	if toInt(eventTypeCountsAny["integrity_recovery"]) != 1 {
		t.Errorf("integrity_recovery count = %d, want 1", toInt(eventTypeCountsAny["integrity_recovery"]))
	}

	// Verify object kind counts
	objectKindCountsAny, ok := metric[objects.FieldKeyObjectKindCounts].(map[string]any)
	if !ok {
		t.Fatal("object_kind_counts is not a map[string]any")
	}
	if toInt(objectKindCountsAny["backlog_item"]) != 2 {
		t.Errorf("backlog_item count = %d, want 2", toInt(objectKindCountsAny["backlog_item"]))
	}
	if toInt(objectKindCountsAny["goal"]) != 1 {
		t.Errorf("goal count = %d, want 1", toInt(objectKindCountsAny["goal"]))
	}

	// Verify status-based error metrics
	statusCountsAny, ok := metric[objects.FieldKeyStatusCounts].(map[string]any)
	if !ok {
		t.Fatal("status_counts is not a map[string]any")
	}
	if toInt(statusCountsAny["completed"]) != 1 {
		t.Errorf("completed status count = %d, want 1", toInt(statusCountsAny["completed"]))
	}
	if toInt(statusCountsAny[objects.FieldKeyFailed]) != 1 {
		t.Errorf("failed status count = %d, want 1", toInt(statusCountsAny[objects.FieldKeyFailed]))
	}
	if toInt(statusCountsAny["error"]) != 1 {
		t.Errorf("error status count = %d, want 1", toInt(statusCountsAny["error"]))
	}
	if toInt(metric[objects.FieldKeyErrorEventCount]) != 2 {
		t.Errorf("error_event_count = %d, want 2", toInt(metric[objects.FieldKeyErrorEventCount]))
	}
	errorRate, ok := metric[objects.FieldKeyErrorRate].(float64)
	if !ok {
		t.Fatalf("error_rate is not a float64: %T", metric[objects.FieldKeyErrorRate])
	}
	if errorRate < 0.666 || errorRate > 0.667 {
		t.Errorf("error_rate = %.6f, want ~0.666667", errorRate)
	}

	// Verify event IDs
	if len(eventIDs) != 3 {
		t.Errorf("eventIDs length = %d, want 3", len(eventIDs))
	}

	// Verify ID ranges are compressed
	aggregatedIDs, ok := metric[objects.FieldKeyAggregatedEventIDs].([]string)
	if !ok {
		t.Fatal("aggregated_event_ids is not a []string")
	}
	// Should be compressed to a range since they're consecutive
	if len(aggregatedIDs) != 1 || aggregatedIDs[0] != "AUD-1..AUD-3" {
		t.Errorf("aggregated_event_ids = %v, want [AUD-1..AUD-3]", aggregatedIDs)
	}
}

// TestAuditAggregationService_AggregateEvents_EventTypeCountsNonEmptyWhenNoEventTypes verifies that when
// events have no event_type (or batch is empty), the built metric still has event_type_counts with
// at least one entry so spec validation (minCount: 1) passes. Prevents "Field event_type_counts is required" on persist.
func TestAuditAggregationService_AggregateEvents_EventTypeCountsNonEmptyWhenNoEventTypes(t *testing.T) {
	service := &AuditAggregationService{}

	windowStart := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2025, 12, 1, 23, 59, 59, 0, time.UTC)

	// Events with no event_type field so eventTypeCounts would be empty without the fix
	events := []map[string]any{
		{objects.FieldKeyID: "AUD-1", objects.FieldKeyTargetKind: "backlog_item"},
		{objects.FieldKeyID: "AUD-2", objects.FieldKeyTargetKind: "backlog_item"},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	metric, _, err := service.aggregateEvents(ctx, secCtx, events, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("aggregateEvents failed: %v", err)
	}

	eventTypeCountsAny, ok := metric[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok {
		t.Fatal("event_type_counts is not a map[string]any")
	}
	if len(eventTypeCountsAny) < 1 {
		t.Errorf("event_type_counts must have at least one entry (spec minCount: 1), got %d", len(eventTypeCountsAny))
	}
	// Fix sets "aggregated": len(events) when no event types present
	if toInt(eventTypeCountsAny["aggregated"]) != 2 {
		t.Errorf("event_type_counts[aggregated] = %v, want 2", eventTypeCountsAny["aggregated"])
	}
}

func TestAuditAggregationService_MergeAggregationMetrics_EventTypeCountsFromMapStringAny(t *testing.T) {
	svc := &AuditAggregationService{}

	// Builder returns map[string]any; merge must handle that and produce non-empty event_type_counts (spec minCount: 1).
	metric1 := map[string]any{
		objects.FieldKeyEventCount:       2,
		objects.FieldKeyEventTypeCounts:  map[string]any{"create": 1, "update": 1},
		objects.FieldKeyObjectKindCounts: nil,
		objects.FieldKeyOperationCounts:  nil,
	}
	metric2 := map[string]any{
		objects.FieldKeyEventCount:       3,
		objects.FieldKeyEventTypeCounts:  map[string]any{"create": 2, "delete": 1},
		objects.FieldKeyStatusCounts:     map[string]any{"completed": 1, objects.FieldKeyFailed: 2},
		objects.FieldKeyObjectKindCounts: nil,
		objects.FieldKeyOperationCounts:  nil,
	}
	merged := svc.mergeAggregationMetrics(metric1, metric2)
	if merged == nil {
		t.Fatal("mergeAggregationMetrics() returned nil")
	}
	etc, ok := merged[objects.FieldKeyEventTypeCounts].(map[string]any)
	if !ok || len(etc) == 0 {
		t.Fatalf("event_type_counts missing or empty after merge (minCount: 1); got %#v", merged[objects.FieldKeyEventTypeCounts])
	}
	if toInt(etc["create"]) != 3 || toInt(etc["update"]) != 1 || toInt(etc["delete"]) != 1 {
		t.Errorf("event_type_counts = %v, want create=3, update=1, delete=1", etc)
	}
	statusCountsAny, ok := merged[objects.FieldKeyStatusCounts].(map[string]any)
	if !ok {
		t.Fatalf("status_counts missing or wrong type: %#v", merged[objects.FieldKeyStatusCounts])
	}
	if toInt(statusCountsAny["completed"]) != 1 || toInt(statusCountsAny[objects.FieldKeyFailed]) != 2 {
		t.Errorf("status_counts = %v, want completed=1, failed=2", statusCountsAny)
	}
	if toInt(merged[objects.FieldKeyErrorEventCount]) != 2 {
		t.Errorf("error_event_count = %v, want 2", merged[objects.FieldKeyErrorEventCount])
	}
	mergedErrorRate, ok := merged[objects.FieldKeyErrorRate].(float64)
	if !ok {
		t.Fatalf("merged error_rate is not float64: %T", merged[objects.FieldKeyErrorRate])
	}
	if mergedErrorRate < 0.399 || mergedErrorRate > 0.401 {
		t.Errorf("merged error_rate = %.6f, want ~0.4", mergedErrorRate)
	}

	// When both inputs have nil/empty event_type_counts, merge must still set at least one entry (aggregated = total count).
	empty1 := map[string]any{objects.FieldKeyEventCount: 5, objects.FieldKeyEventTypeCounts: nil}
	empty2 := map[string]any{objects.FieldKeyEventCount: 10, objects.FieldKeyEventTypeCounts: nil}
	mergedEmpty := svc.mergeAggregationMetrics(empty1, empty2)
	etcEmpty, _ := mergedEmpty[objects.FieldKeyEventTypeCounts].(map[string]any)
	if len(etcEmpty) == 0 {
		t.Fatal("event_type_counts empty after merge of two nil maps; need fallback for spec minCount: 1")
	}
	if toInt(etcEmpty["aggregated"]) != 15 {
		t.Errorf("event_type_counts[aggregated] = %v, want 15", etcEmpty["aggregated"])
	}
}

func TestAuditAggregationService_QueryAuditEventsInWindow(t *testing.T) {
	// Do not t.Parallel: disableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED.
	disableStreamStorageForTest(t)

	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "audit-aggregation-test")
	mustEnsureProcessSpecsLayout(t, testRoot)

	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fileStorage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := WithCLIOperation(pkgctx.NewSystemContext())
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system",
	}

	// Create some audit events across different times
	time1 := time.Date(2025, 12, 1, 10, 0, 0, 0, time.UTC)
	time2 := time.Date(2025, 12, 1, 15, 0, 0, 0, time.UTC)
	time3 := time.Date(2025, 12, 2, 10, 0, 0, 0, time.UTC)

	events := []map[string]any{
		{
			objects.FieldKeyID:            "AUD-001",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     time1.Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "account:system",
			objects.FieldKeyUpdatedAt:     time1.Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "account:system",
			objects.FieldKeyOriginSystem:  "test",
			objects.FieldKeyOriginProject: "test",
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeyEventType:     "object_creation",
		},
		{
			objects.FieldKeyID:            "AUD-002",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     time2.Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "account:system",
			objects.FieldKeyUpdatedAt:     time2.Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "account:system",
			objects.FieldKeyOriginSystem:  "test",
			objects.FieldKeyOriginProject: "test",
			objects.FieldKeyStatus:        "failed",
			objects.FieldKeyEventType:     "object_update",
		},
		{
			objects.FieldKeyID:            "AUD-003",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     time3.Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "account:system",
			objects.FieldKeyUpdatedAt:     time3.Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "account:system",
			objects.FieldKeyOriginSystem:  "test",
			objects.FieldKeyOriginProject: "test",
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeyEventType:     "object_deletion",
		},
	}

	for _, ev := range events {
		err = fileStorage.Create(ctx, secCtx, ev)
		if err != nil {
			t.Fatalf("Failed to create audit event: %v", err)
		}
	}

	// Wait for index updates
	FlushOrFail(t, fileStorage.GetProjectRoot(), "audit_event")

	service := NewAuditAggregationService(fileStorage)
	storageCtx := pkgctx.NewStorageContext()

	windowStart := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2025, 12, 1, 23, 59, 59, 0, time.UTC)

	results, err := service.queryAuditEventsInWindow(ctx, secCtx, storageCtx, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("Failed to query audit events: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 events in window, got %d", len(results))
	}

	// Make sure AUD-001 and AUD-002 are the ones returned
	found1, found2 := false, false
	for _, res := range results {
		id, _ := res[objects.FieldKeyID].(string)
		if id == "AUD-001" {
			found1 = true
		}
		if id == "AUD-002" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Errorf("Expected to find AUD-001 and AUD-002 in results, but found: %v", results)
	}
}

func TestAuditAggregationService_IsConsecutive(t *testing.T) {
	service := &AuditAggregationService{}

	tests := []struct {
		id1  string
		id2  string
		want bool
	}{
		{"AUD-100", "AUD-101", true},
		{"AUD-100", "AUD-102", false},
		{"AUD-1", "AUD-2", true},
		{"AUD-999", "AUD-1000", true},
		{"AUD-100", "AUD-99", false},
	}

	for _, tt := range tests {
		t.Run(tt.id1+"_"+tt.id2, func(t *testing.T) {
			got := service.isConsecutive(tt.id1, tt.id2)
			if got != tt.want {
				t.Errorf("isConsecutive(%q, %q) = %v, want %v", tt.id1, tt.id2, got, tt.want)
			}
		})
	}
}

func TestAuditAggregationService_ExtractAuditIDNumber(t *testing.T) {
	service := &AuditAggregationService{}

	tests := []struct {
		id   string
		want int
	}{
		{"AUD-1", 1},
		{"AUD-100", 100},
		{"AUD-999", 999},
		{"AUD-1234", 1234},
		{"AUD-0", 0},
		{"INVALID", 0},
		{"AUD-", 0},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := service.extractAuditIDNumber(tt.id)
			if got != tt.want {
				t.Errorf("extractAuditIDNumber(%q) = %d, want %d", tt.id, got, tt.want)
			}
		})
	}
}

func TestAuditAggregationService_CompareAuditID(t *testing.T) {
	service := &AuditAggregationService{}

	tests := []struct {
		id1  string
		id2  string
		want int // -1, 0, or 1
	}{
		{"AUD-1", "AUD-2", -1},
		{"AUD-100", "AUD-50", 1},
		{"AUD-100", "AUD-100", 0},
		{"AUD-1", "AUD-1000", -1},
	}

	for _, tt := range tests {
		t.Run(tt.id1+"_"+tt.id2, func(t *testing.T) {
			got := service.compareAuditID(tt.id1, tt.id2)
			if (got < 0 && tt.want >= 0) || (got == 0 && tt.want != 0) || (got > 0 && tt.want <= 0) {
				t.Errorf("compareAuditID(%q, %q) = %d, want %d", tt.id1, tt.id2, got, tt.want)
			}
		})
	}
}

func TestAuditAggregationService_LifetimeCounters(t *testing.T) {
	service := &AuditAggregationService{}
	agg, events := service.GetAuditAggregationStats()
	if agg != 0 || events != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", agg, events)
	}

	service.aggregationsTotal.Add(1)
	service.eventsAggregatedTotal.Add(42)

	agg, events = service.GetAuditAggregationStats()
	if agg != 1 || events != 42 {
		t.Errorf("expected (1, 42), got (%d, %d)", agg, events)
	}
}
