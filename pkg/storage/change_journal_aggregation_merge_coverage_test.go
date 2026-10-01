package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestStorageExtended_Wave22_ChangeJournalAggregationMetric(t *testing.T) {
	ctx := context.Background()
	_, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	svc := NewChangeJournalAggregationService(fos)

	now := time.Now().UTC()
	windowStartTime := now.Add(-1 * time.Hour)
	windowEndTime := now
	windowStart := windowStartTime.Format(time.RFC3339)
	windowEnd := windowEndTime.Format(time.RFC3339)

	// 1. Initial creation via aggregateEntries and createAggregationMetricSync
	entries := []map[string]any{
		{
			objects.FieldKeyID:         "CJE-001",
			objects.FieldKeyChangeType: "create",
		},
	}
	metric, _, err := svc.aggregateEntries(ctx, secCtx, entries, windowStartTime, windowEndTime)
	require.NoError(t, err)
	metricID := metric[objects.FieldKeyID].(string)

	createdID, err := svc.createAggregationMetricSync(ctx, secCtx, metric)
	assert.NoError(t, err)
	assert.Equal(t, metricID, createdID)

	// 2. Second creation with existing metric ID triggers already-exists and update path
	updatedID, err := svc.createAggregationMetricSync(ctx, secCtx, metric)
	assert.NoError(t, err)
	assert.Equal(t, metricID, updatedID)

	// 3. createAggregationMetric (asynchronous wrapper)
	asyncStartTime := now.Add(-3 * time.Hour)
	asyncEndTime := now.Add(-2 * time.Hour)
	metric3, _, err := svc.aggregateEntries(ctx, secCtx, entries, asyncStartTime, asyncEndTime)
	require.NoError(t, err)
	asyncID, err := svc.createAggregationMetric(ctx, secCtx, metric3)
	assert.NoError(t, err)
	assert.NotEmpty(t, asyncID)

	// 4. findExistingMetricByWindow and findExistingMetricByOverlappingWindow
	found, err := svc.findExistingMetricByWindow(ctx, secCtx, windowStart, windowEnd)
	assert.NoError(t, err)
	assert.NotNil(t, found)

	// Overlapping window find
	overlapID := svc.findExistingMetricByOverlappingWindow(ctx, secCtx,
		now.Add(-30*time.Minute).Format(time.RFC3339),
		now.Add(30*time.Minute).Format(time.RFC3339))
	assert.Equal(t, metricID, overlapID)

	// Overlapping search with non-matching window
	noOverlapID := svc.findExistingMetricByOverlappingWindow(ctx, secCtx,
		now.Add(-10*time.Hour).Format(time.RFC3339),
		now.Add(-9*time.Hour).Format(time.RFC3339))
	assert.Empty(t, noOverlapID)

	// 5. markEntriesAsAggregated
	n, err := svc.markEntriesAsAggregated(ctx, secCtx, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)

	// 6. QueryOldAggregatedEntries, QueryOldEntriesByAge, CleanupAggregatedEntries
	storageCtx := pkgctx.GetStorageContext()
	oldEntries, err := svc.QueryOldAggregatedEntries(ctx, secCtx, storageCtx, now.Add(1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, oldEntries)

	entriesByAge, err := svc.QueryOldEntriesByAge(ctx, secCtx, storageCtx, now.Add(1*time.Hour), 10)
	assert.NoError(t, err)
	assert.Empty(t, entriesByAge)

	cleaned, err := svc.CleanupAggregatedEntries(ctx, secCtx, nil, true)
	assert.NoError(t, err)
	assert.Equal(t, 0, cleaned)

	cliCtx := WithCLIOperation(ctx)
	cleanedDel, err := svc.CleanupAggregatedEntries(cliCtx, secCtx, nil, false)
	assert.NoError(t, err)
	assert.Equal(t, 0, cleanedDel)
}

func TestStorageExtended_Wave22_AggregateChangeJournalEntries(t *testing.T) {
	ctx := context.Background()
	_, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	svc := NewChangeJournalAggregationServiceWithBatchSize(fos, 10)
	assert.Equal(t, 10, svc.batchSize)

	now := time.Now().UTC()
	windowStart := now.Add(-2 * time.Hour)
	windowEnd := now

	storageCtx := pkgctx.GetStorageContext()

	// 1. When no change journal entries match, returns 0 metrics created
	res, err := svc.AggregateChangeJournalEntries(ctx, secCtx, storageCtx, windowStart, windowEnd)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, 0, res.MetricsCreated)

	runs, aggregated := svc.GetAggregationStats()
	assert.Equal(t, int64(1), runs)
	assert.Equal(t, int64(0), aggregated)
}

func TestStorageExtended_Wave22_MergeAndCompress(t *testing.T) {
	svc := NewChangeJournalAggregationServiceWithBatchSize(nil, 0)
	assert.Equal(t, DefaultBatchSize, svc.batchSize)

	m1 := map[string]any{
		objects.FieldKeyEventCount:             3,
		objects.FieldKeyCollectionCount:        1,
		objects.FieldKeyLastSeen:               "2026-09-23T01:00:00Z",
		objects.FieldKeyAggregationWindowStart: "2026-09-23T00:00:00Z",
		objects.FieldKeyAggregationWindowEnd:   "2026-09-23T02:00:00Z",
		objects.FieldKeyEventTypeCounts: map[string]int{
			"create": 2,
			"update": 1,
		},
	}
	m2 := map[string]any{
		objects.FieldKeyEventCount:             2,
		objects.FieldKeyCollectionCount:        1,
		objects.FieldKeyLastSeen:               "2026-09-23T01:30:00Z",
		objects.FieldKeyAggregationWindowStart: "2026-09-23T00:00:00Z",
		objects.FieldKeyAggregationWindowEnd:   "2026-09-23T02:00:00Z",
		objects.FieldKeyEventTypeCounts: map[string]int{
			"create": 1,
			"delete": 1,
		},
	}

	merged := svc.mergeAggregationMetrics(m1, m2)
	assert.Equal(t, 5, merged[objects.FieldKeyEventCount])
	assert.Equal(t, 2, merged[objects.FieldKeyCollectionCount])
	assert.Equal(t, "2026-09-23T01:30:00Z", merged[objects.FieldKeyLastSeen])
	counts := merged[objects.FieldKeyEventTypeCounts].(map[string]int)
	assert.Equal(t, 3, counts["create"])
	assert.Equal(t, 1, counts["update"])
	assert.Equal(t, 1, counts["delete"])

	compressed := svc.compressEntryIDs([]string{"a", "b"})
	assert.Equal(t, []string{"a", "b"}, compressed)
	assert.Empty(t, svc.compressEntryIDs(nil))
}
