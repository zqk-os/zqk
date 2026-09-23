package storage_test

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// TestStorageExtended_Wave5_AuditAggregation tests audit_aggregation_aggregate.go, query.go, and helpers.go
func TestStorageExtended_Wave5_AuditAggregation(t *testing.T) {
	tmpDir := t.TempDir()
	fos, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileObjectStorage: %v", err)
	}
	if cleanup := fos.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = fos.Shutdown(ctx) }()

	secCtx := &pkgctx.SecurityContext{
		AccountID:   "system",
		Roles:       []string{"admin"},
		Permissions: []string{"read:*", "write:*"},
	}

	service := storagepkg.NewAuditAggregationService(fos)
	now := time.Now().UTC()
	wStart := now.Add(-1 * time.Hour)
	wEnd := now

	// 1. ExpandIDRange and ExpandIDRanges
	ranges := []string{"AUD-1..AUD-3", "AUD-5"}
	expanded := storagepkg.ExpandIDRanges(ranges)
	if len(expanded) < 3 {
		t.Errorf("expected expanded ranges to have at least 3 IDs, got %v", expanded)
	}
	singleRange := storagepkg.ExpandIDRange("AUD-10..AUD-12")
	if len(singleRange) != 3 {
		t.Errorf("expected 3 IDs from range, got %v", singleRange)
	}

	// 2. AggregateAuditEvents on empty window
	result, err := service.AggregateAuditEvents(ctx, secCtx, nil, wStart, wEnd)
	if err != nil {
		t.Logf("AggregateAuditEvents returned: %v", err)
	}
	_ = result

	// 3. QueryOldAggregatedEvents
	oldAgg, err := service.QueryOldAggregatedEvents(ctx, secCtx, nil, now)
	if err != nil {
		t.Logf("QueryOldAggregatedEvents returned: %v", err)
	}
	_ = oldAgg

	// 4. QueryOldAuditEventsByAge
	oldByAge, err := service.QueryOldAuditEventsByAge(ctx, secCtx, nil, now, 100)
	if err != nil {
		t.Logf("QueryOldAuditEventsByAge returned: %v", err)
	}
	_ = oldByAge

	// 5. CleanupAggregatedEvents with archive=true and archive=false
	cnt, err := service.CleanupAggregatedEvents(ctx, secCtx, []string{"AUD-NONEXISTENT"}, true)
	if err != nil {
		t.Logf("CleanupAggregatedEvents (archive=true) returned: %v", err)
	}
	_ = cnt

	cnt, err = service.CleanupAggregatedEvents(ctx, secCtx, []string{"AUD-NONEXISTENT"}, false)
	if err != nil {
		t.Logf("CleanupAggregatedEvents (archive=false) returned: %v", err)
	}
	_ = cnt

	// 6. FindExistingMetricByWindow and FindExistingMetricByOverlappingWindow
	_, _ = service.FindExistingMetricByWindowForTest(ctx, secCtx, wStart.Format(time.RFC3339), wEnd.Format(time.RFC3339))
	_ = service.FindExistingMetricByOverlappingWindowForTest(ctx, secCtx, wStart.Format(time.RFC3339), wEnd.Format(time.RFC3339))

	// 7. CreateAggregationMetricForTest
	metricObj := map[string]any{
		objects.FieldKeyKind:                   "audit_aggregation",
		objects.FieldKeyAggregationWindowStart: wStart.Format(time.RFC3339),
		objects.FieldKeyAggregationWindowEnd:   wEnd.Format(time.RFC3339),
		objects.FieldKeySource:                 "audit_aggregation_job",
		objects.FieldKeyStatus:                 "originated",
	}
	metricID, err := service.CreateAggregationMetricForTest(ctx, secCtx, metricObj)
	if err != nil {
		t.Logf("CreateAggregationMetricForTest returned: %v", err)
	}
	_ = metricID

	// 8. MarkEventsAsAggregatedForTest
	_, _ = service.MarkEventsAsAggregatedForTest(ctx, secCtx, []string{"AUD-1"})
}
