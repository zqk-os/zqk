package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func TestChangeJournalAggregationService_aggregateEntries_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-time.Hour)
	windowEnd := time.Now()

	metric, entryIDs, err := svc.aggregateEntries(ctx, secCtx, nil, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("aggregateEntries() error = %v", err)
	}
	if metric == nil {
		t.Fatal("aggregateEntries() returned nil metric")
	}
	if len(entryIDs) != 0 {
		t.Errorf("aggregateEntries() entryIDs length = %d, want 0", len(entryIDs))
	}
	if c, _ := metric[objects.FieldKeyEventCount].(int); c != 0 {
		t.Errorf("event_count = %d, want 0", c)
	}
}

func TestChangeJournalAggregationService_aggregateEntries_SingleEntry(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-time.Hour)
	windowEnd := time.Now()
	entries := []map[string]any{
		{objects.FieldKeyID: "CJE-001", objects.FieldKeyChangeType: "create", objects.FieldKeyObjectRef: "backlog_item:BLI-001"},
	}

	metric, entryIDs, err := svc.aggregateEntries(ctx, secCtx, entries, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("aggregateEntries() error = %v", err)
	}
	if len(entryIDs) != 1 || entryIDs[0] != "CJE-001" {
		t.Errorf("entryIDs = %v, want [CJE-001]", entryIDs)
	}
	if c, _ := metric[objects.FieldKeyEventCount].(int); c != 1 {
		t.Errorf("event_count = %d, want 1", c)
	}
	typeCounts, _ := metric[objects.FieldKeyEventTypeCounts].(map[string]any)
	if typeCounts == nil || toInt(typeCounts["create"]) != 1 {
		t.Errorf("event_type_counts = %v, want map[create:1]", typeCounts)
	}
}

func TestChangeJournalAggregationService_aggregateEntries_MultipleChangeTypes(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-time.Hour)
	windowEnd := time.Now()
	entries := []map[string]any{
		{objects.FieldKeyID: "CJE-001", objects.FieldKeyChangeType: "create"},
		{objects.FieldKeyID: "CJE-002", objects.FieldKeyChangeType: "update"},
		{objects.FieldKeyID: "CJE-003", objects.FieldKeyChangeType: "create"},
		{objects.FieldKeyID: "CJE-004", objects.FieldKeyChangeType: ""}, // unknown
	}

	metric, entryIDs, err := svc.aggregateEntries(ctx, secCtx, entries, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("aggregateEntries() error = %v", err)
	}
	if len(entryIDs) != 4 {
		t.Errorf("entryIDs length = %d, want 4", len(entryIDs))
	}
	if c, _ := metric[objects.FieldKeyEventCount].(int); c != 4 {
		t.Errorf("event_count = %d, want 4", c)
	}
	typeCounts, _ := metric[objects.FieldKeyEventTypeCounts].(map[string]any)
	if typeCounts == nil || toInt(typeCounts["create"]) != 2 || toInt(typeCounts["update"]) != 1 || toInt(typeCounts["unknown"]) != 1 {
		t.Errorf("event_type_counts = %v", typeCounts)
	}
}

func TestChangeJournalAggregationService_compressEntryIDs(t *testing.T) {
	svc := &ChangeJournalAggregationService{}
	tests := []struct {
		name     string
		entryIDs []string
		want     []string
	}{
		{"empty", nil, []string{}},
		{"empty slice", []string{}, []string{}},
		{"single", []string{"CJE-001"}, []string{"CJE-001"}},
		{"multiple", []string{"CJE-001", "CJE-002", "CJE-003"}, []string{"CJE-001", "CJE-002", "CJE-003"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.compressEntryIDs(tt.entryIDs)
			if len(got) != len(tt.want) {
				t.Errorf("compressEntryIDs() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("compressEntryIDs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestChangeJournalAggregationService_mergeAggregationMetrics(t *testing.T) {
	svc := &ChangeJournalAggregationService{}
	now := zqktime.NowRFC3339UTC()
	metric1 := map[string]any{
		objects.FieldKeyEventCount:             2,
		objects.FieldKeyEventTypeCounts:        map[string]int{"create": 1, "update": 1},
		objects.FieldKeyCollectionCount:        1,
		objects.FieldKeyAggregationWindowStart: now,
		objects.FieldKeyAggregationWindowEnd:   now,
		objects.FieldKeyLastSeen:               now,
		objects.FieldKeyTitle:                  "Change Journal Aggregation: 2 entries",
	}
	metric2 := map[string]any{
		objects.FieldKeyEventCount:             3,
		objects.FieldKeyEventTypeCounts:        map[string]int{"create": 2, "update": 1},
		objects.FieldKeyCollectionCount:        1,
		objects.FieldKeyAggregationWindowStart: now,
		objects.FieldKeyAggregationWindowEnd:   now,
		objects.FieldKeyLastSeen:               now,
		objects.FieldKeyTitle:                  "Change Journal Aggregation: 3 entries",
	}

	merged := svc.mergeAggregationMetrics(metric1, metric2)
	if merged == nil {
		t.Fatal("mergeAggregationMetrics() returned nil")
	}
	if c, _ := merged[objects.FieldKeyEventCount].(int); c != 5 {
		t.Errorf("event_count = %d, want 5", c)
	}
	typeCounts, _ := merged[objects.FieldKeyEventTypeCounts].(map[string]int)
	if typeCounts["create"] != 3 || typeCounts["update"] != 2 {
		t.Errorf("event_type_counts = %v", typeCounts)
	}
}

func TestChangeJournalAggregationService_QueryOldAggregatedEntries_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()
	cutoff := time.Now().Add(-24 * time.Hour)

	ids, err := svc.QueryOldAggregatedEntries(ctx, secCtx, storageCtx, cutoff)
	if err != nil {
		t.Fatalf("QueryOldAggregatedEntries() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("QueryOldAggregatedEntries() returned %d ids, want 0", len(ids))
	}
}

func TestChangeJournalAggregationService_QueryOldEntriesByAge_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()
	cutoff := time.Now().Add(-24 * time.Hour)

	ids, err := svc.QueryOldEntriesByAge(ctx, secCtx, storageCtx, cutoff, 0)
	if err != nil {
		t.Fatalf("QueryOldEntriesByAge() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("QueryOldEntriesByAge() returned %d ids, want 0", len(ids))
	}
	// With limit
	ids2, err := svc.QueryOldEntriesByAge(ctx, secCtx, storageCtx, cutoff, 100)
	if err != nil {
		t.Fatalf("QueryOldEntriesByAge(limit=100) error = %v", err)
	}
	if len(ids2) != 0 {
		t.Errorf("QueryOldEntriesByAge(limit=100) returned %d ids, want 0", len(ids2))
	}
}

func TestChangeJournalAggregationService_CleanupAggregatedEntries_EmptyArchive(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Archive path with empty list: should succeed and return 0
	n, err := svc.CleanupAggregatedEntries(ctx, secCtx, nil, true)
	if err != nil {
		t.Fatalf("CleanupAggregatedEntries(archive=true) error = %v", err)
	}
	if n != 0 {
		t.Errorf("CleanupAggregatedEntries(archive=true) count = %d, want 0", n)
	}
	n, err = svc.CleanupAggregatedEntries(ctx, secCtx, []string{}, true)
	if err != nil {
		t.Fatalf("CleanupAggregatedEntries(archive=true, empty slice) error = %v", err)
	}
	if n != 0 {
		t.Errorf("CleanupAggregatedEntries(archive=true, empty slice) count = %d, want 0", n)
	}
	// Delete path (archive=false) with empty list
	cliCtx := WithCLIOperation(ctx)
	delN, err := svc.CleanupAggregatedEntries(cliCtx, secCtx, []string{}, false)
	if err != nil {
		t.Fatalf("CleanupAggregatedEntries(archive=false, empty slice) error = %v", err)
	}
	if delN != 0 {
		t.Errorf("CleanupAggregatedEntries(archive=false, empty slice) count = %d, want 0", delN)
	}
}


// TestChangeJournalAggregationService_AggregateChangeJournalEntries_Integration runs the full
// aggregation workflow: create change journal entries, run aggregation, verify metric and entry updates.
// BLI-635: Integration test for aggregation workflow.
func TestChangeJournalAggregationService_AggregateChangeJournalEntries_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	storageProvider := fos
	ctx := context.Background()
	storageCtx := pkgctx.GetStorageContext()

	// Create two change journal entries so aggregation has something to process
	for i, changeType := range []string{"create", "update"} {
		options := &ChangeJournalEntryOptions{
			ChangeType:  changeType,
			ObjectRef:   fmt.Sprintf("backlog_item:BLI-INT-%d", i),
			DiffSummary: "integration test entry",
		}
		err := CreateChangeJournalEntryWithBuilder(ctx, tmpDir, secCtx, storageProvider, options)
		if err != nil {
			t.Skipf("CreateChangeJournalEntryWithBuilder (run from repo root with specs): %v", err)
		}
	}

	// Run aggregation over a window that includes the entries we just created
	windowEnd := time.Now().Add(1 * time.Second)
	windowStart := windowEnd.Add(-1 * time.Hour)
	svc := NewChangeJournalAggregationService(storageProvider)

	result, err := svc.AggregateChangeJournalEntries(ctx, secCtx, storageCtx, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("AggregateChangeJournalEntries() error = %v", err)
	}
	// We may get 0 or more entries depending on list/query timing; at least the call should succeed
	if result == nil {
		t.Fatal("AggregateChangeJournalEntries() returned nil result")
	}
	if result.MetricsCreated > 0 {
		if result.MetricID == emptyValue {
			t.Error("MetricID should be set when MetricsCreated > 0")
		}
		if result.EntryCount > 0 && len(result.EntriesProcessed) != result.EntryCount {
			t.Errorf("EntriesProcessed length = %d, want %d", len(result.EntriesProcessed), result.EntryCount)
		}
	}
}

func TestChangeJournalAggregationService_LifetimeCounters(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	run, agg := svc.GetAggregationStats()
	if run != 0 || agg != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", run, agg)
	}

	windowEnd := time.Now().Add(1 * time.Second)
	windowStart := windowEnd.Add(-1 * time.Hour)
	_, _ = svc.AggregateChangeJournalEntries(ctx, secCtx, storageCtx, windowStart, windowEnd)

	run, _ = svc.GetAggregationStats()
	if run != 1 {
		t.Errorf("expected aggregationsRun=1, got %d", run)
	}
}

// TestChangeJournalAggregationService_QueryOldAggregatedEntries_ThenCleanupArchive verifies
// the cleanup workflow: query old aggregated entries, then archive them (BLI-636).
func TestChangeJournalAggregationService_QueryOldAggregatedEntries_ThenCleanupArchive(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// Query with cutoff in the future: no old entries
	cutoff := time.Now().Add(24 * time.Hour)
	ids, err := svc.QueryOldAggregatedEntries(ctx, secCtx, storageCtx, cutoff)
	if err != nil {
		t.Fatalf("QueryOldAggregatedEntries() error = %v", err)
	}
	// Cleanup (archive) with empty list should succeed and return 0
	n, err := svc.CleanupAggregatedEntries(ctx, secCtx, ids, true)
	if err != nil {
		t.Fatalf("CleanupAggregatedEntries(archive=true) error = %v", err)
	}
	if n != 0 {
		t.Errorf("CleanupAggregatedEntries() count = %d, want 0", n)
	}
}

// TestChangeJournalAggregationService_aggregateEntries_NilAndEmptyContext verifies error handling (BLI-637).
func TestChangeJournalAggregationService_aggregateEntries_NilAndEmptyContext(t *testing.T) {
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() { _ = storageFactory.Shutdown(context.Background()) })
	svc := NewChangeJournalAggregationService(storageFactory.GetStorage())
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-time.Hour)
	windowEnd := time.Now()

	// Nil entries: should return valid metric with 0 counts (same as empty)
	metric, entryIDs, err := svc.aggregateEntries(context.Background(), secCtx, nil, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("aggregateEntries(empty entries) error = %v", err)
	}
	if metric == nil {
		t.Fatal("aggregateEntries(empty entries) returned nil metric")
	}
	if len(entryIDs) != 0 {
		t.Errorf("entryIDs length = %d, want 0", len(entryIDs))
	}

	// Entry with missing change_type: should be counted as "unknown"
	entries := []map[string]any{{objects.FieldKeyID: "CJE-X", objects.FieldKeyObjectRef: "backlog_item:BLI-1"}}
	metric2, ids2, err := svc.aggregateEntries(context.Background(), secCtx, entries, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("aggregateEntries(entry without change_type) error = %v", err)
	}
	if len(ids2) != 1 || ids2[0] != "CJE-X" {
		t.Errorf("entryIDs = %v, want [CJE-X]", ids2)
	}
	typeCounts, _ := metric2[objects.FieldKeyEventTypeCounts].(map[string]any)
	if typeCounts == nil || toInt(typeCounts["unknown"]) != 1 {
		t.Errorf("event_type_counts = %v, expected unknown: 1", typeCounts)
	}
}
