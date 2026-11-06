package whatsnext

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCoverageExpansion_FirstFloatAndFirstInt(t *testing.T) {
	t.Parallel()

	m := map[string]any{
		"f64": float64(12.34),
		"f32": float32(56.78),
		"i":   int(42),
		"i64": int64(100),
		"jn":  json.Number("99.5"),
		"jni": json.Number("77"),
	}

	// firstFloat
	assert.InDelta(t, 12.34, firstFloat(m, "missing", "f64"), 0.001)
	assert.InDelta(t, 56.78, firstFloat(m, "f32"), 0.01)
	assert.InDelta(t, 42.0, firstFloat(m, "i"), 0.001)
	assert.InDelta(t, 100.0, firstFloat(m, "i64"), 0.001)
	assert.InDelta(t, 99.5, firstFloat(m, "jn"), 0.001)
	assert.Equal(t, 0.0, firstFloat(m, "missing", "none"))

	// firstInt
	assert.Equal(t, 12, firstInt(m, "missing", "f64"))
	assert.Equal(t, 42, firstInt(m, "i"))
	assert.Equal(t, 100, firstInt(m, "i64"))
	assert.Equal(t, 77, firstInt(m, "jni"))
	assert.Equal(t, 0, firstInt(m, "missing", "none"))
}

func TestCoverageExpansion_StringSliceField(t *testing.T) {
	t.Parallel()

	// string slice
	rawStrings := []string{"  foo ", "", "bar  ", "   "}
	assert.Equal(t, []string{"foo", "bar"}, stringSliceField(rawStrings))

	// any slice
	rawAny := []any{" hello ", "", "world ", 123}
	assert.Equal(t, []string{"hello", "world"}, stringSliceField(rawAny))

	// default non-slice
	assert.Nil(t, stringSliceField("not-a-slice"))
	assert.Nil(t, stringSliceField(123))
	assert.Nil(t, stringSliceField(nil))
}

func TestCoverageExpansion_FirstNonEmptyLine(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", firstNonEmptyLine(""))
	assert.Equal(t, "", firstNonEmptyLine("\n  \n\t\n"))
	assert.Equal(t, "hello world", firstNonEmptyLine("\n  hello world\nsecond line"))
}

func TestCoverageExpansion_IsPlanStatusCompleted(t *testing.T) {
	t.Parallel()

	assert.False(t, IsPlanStatusCompleted(""))
	assert.False(t, IsPlanStatusCompleted("  "))
	assert.True(t, IsPlanStatusCompleted(objects.ObjectStatusComplete))
	assert.True(t, IsPlanStatusCompleted(objects.ObjectStatusCompleted))
	assert.True(t, IsPlanStatusCompleted(objects.ObjectStatusArchived))
	assert.True(t, IsPlanStatusCompleted(objects.ObjectStatusCancelled))
	assert.True(t, IsPlanStatusCompleted(objects.ObjectStatusRejected))
	assert.True(t, IsPlanStatusCompleted("DONE"))
	assert.True(t, IsPlanStatusCompleted("done"))

	// Ineligible / planned / originated / draft / proposed
	assert.False(t, IsPlanStatusCompleted(objects.ObjectStatusPlanned))
	assert.False(t, IsPlanStatusCompleted(objects.ObjectStatusOriginated))
	assert.False(t, IsPlanStatusCompleted("draft"))
	assert.False(t, IsPlanStatusCompleted("proposed"))
	assert.False(t, IsPlanStatusCompleted("in_progress"))
	assert.False(t, IsPlanStatusCompleted("active"))
}

func TestCoverageExpansion_IsCachedPlanStale(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// nil plan
	assert.False(t, IsCachedPlanStale(ctx, nil, nil))

	// empty plan ID
	assert.False(t, IsCachedPlanStale(ctx, nil, &WhatsNextPriorityPlan{ID: "   "}))

	// plan status completed
	assert.True(t, IsCachedPlanStale(ctx, nil, &WhatsNextPriorityPlan{ID: "PRI-1", Status: "complete"}))
	assert.True(t, IsCachedPlanStale(ctx, nil, &WhatsNextPriorityPlan{ID: "PRI-1", Status: "completed"}))

	// nil sp with active plan
	assert.False(t, IsCachedPlanStale(ctx, nil, &WhatsNextPriorityPlan{ID: "PRI-1", Status: "in_progress"}))

	// with mock storage provider
	mock := newMockStorage()

	// Not found in storage, but storage has other plans -> true
	assert.True(t, IsCachedPlanStale(ctx, mock, &WhatsNextPriorityPlan{ID: "PRI-NONEXISTENT", Status: "in_progress"}))

	// Clear priority plans in mock
	mock.mu.Lock()
	mock.objects[objects.KindPriorityPlan] = nil
	mock.mu.Unlock()

	// Not found in storage and storage has NO plans -> false
	assert.False(t, IsCachedPlanStale(ctx, mock, &WhatsNextPriorityPlan{ID: "PRI-NONEXISTENT", Status: "in_progress"}))

	// PRI-1 in storage as active
	mock.mu.Lock()
	mock.objects[objects.KindPriorityPlan] = []map[string]any{
		{
			objects.FieldKeyID:     "PRI-1",
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	}
	mock.mu.Unlock()

	assert.False(t, IsCachedPlanStale(ctx, mock, &WhatsNextPriorityPlan{ID: "PRI-1", Status: "in_progress"}))

	// PRI-1 in storage as completed
	mock.mu.Lock()
	mock.objects[objects.KindPriorityPlan] = []map[string]any{
		{
			objects.FieldKeyID:     "PRI-1",
			objects.FieldKeyStatus: objects.ObjectStatusCompleted,
		},
	}
	mock.mu.Unlock()

	assert.True(t, IsCachedPlanStale(ctx, mock, &WhatsNextPriorityPlan{ID: "PRI-1", Status: "in_progress"}))
}

func writePayloadFile(t *testing.T, targetPath string, payload *WhatsNextLitePayload) {
	t.Helper()
	data, err := json.MarshalIndent(payload, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(targetPath, data, paths.FilePerm644))
}

func TestCoverageExpansion_GetOrRecoverPayload_FreshAndInvalidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()

	// 1. Missing lite file -> returns bootstrap payload
	bootstrap, err := GetOrRecoverPayload(ctx, nil, tempDir, 0)
	require.NoError(t, err)
	assert.True(t, bootstrap.Stale)
	assert.True(t, bootstrap.Recovering)
	assert.Equal(t, "cold_boot_materialized_projection_missing", bootstrap.DegradedReason)

	// 2. Write a valid lite file directly
	targetPath := WhatsNextLiteFilePath(tempDir)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(targetPath)))

	payload := &WhatsNextLitePayload{
		SchemaVersion:  MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		LeadPlan: &WhatsNextPriorityPlan{
			ID:     "PRI-FRESH",
			Title:  "Fresh Plan",
			Status: "in_progress",
		},
		ActivePlans: []WhatsNextPriorityPlan{
			{
				ID:     "PRI-FRESH",
				Title:  "Fresh Plan",
				Status: "in_progress",
			},
		},
	}
	writePayloadFile(t, targetPath, payload)

	freshPayload, err := GetOrRecoverPayload(ctx, nil, tempDir, 10*time.Minute)
	require.NoError(t, err)
	assert.False(t, freshPayload.Stale)
	assert.False(t, freshPayload.Recovering)
	assert.Equal(t, "PRI-FRESH", freshPayload.LeadPlan.ID)

	// 3. Cached lead plan is stale (status completed)
	payload.LeadPlan = &WhatsNextPriorityPlan{
		ID:     "PRI-DONE",
		Title:  "Done Plan",
		Status: "completed",
	}
	payload.ActivePlans = []WhatsNextPriorityPlan{
		{
			ID:     "PRI-DONE",
			Title:  "Done Plan",
			Status: "completed",
		},
	}
	writePayloadFile(t, targetPath, payload)

	stalePayload, err := GetOrRecoverPayload(ctx, nil, tempDir, 10*time.Minute)
	require.NoError(t, err)
	assert.True(t, stalePayload.Stale)
	assert.True(t, stalePayload.Recovering)
	assert.Contains(t, stalePayload.DegradedReason, "PRI-DONE is completed")
	assert.Nil(t, stalePayload.LeadPlan)
	assert.Empty(t, stalePayload.ActivePlans)
}

func TestCoverageExpansion_GetOrRecoverPayload_Fallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()
	targetPath := WhatsNextLiteFilePath(tempDir)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(targetPath)))

	payload := &WhatsNextLitePayload{
		SchemaVersion:  MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().Add(-1 * time.Hour),
		LeadPlan: &WhatsNextPriorityPlan{
			ID:     "PRI-OLD",
			Title:  "Old Plan",
			Status: "in_progress",
		},
		ActivePlans: []WhatsNextPriorityPlan{
			{
				ID:     "PRI-OLD",
				Title:  "Old Plan",
				Status: "in_progress",
			},
		},
	}
	writePayloadFile(t, targetPath, payload)

	// Create quiescent WAL file modified before MaterializedAt
	walPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.WalDir, "lifecycle_events.wal")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(walPath)))
	require.NoError(t, fileutil.WriteFile(walPath, []byte(""), paths.FilePerm644))
	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, fileutil.Chtimes(walPath, oldTime, oldTime))

	// GetOrRecoverPayload should touch watermark and return not stale
	quiescentPayload, err := GetOrRecoverPayload(ctx, nil, tempDir, 10*time.Minute)
	require.NoError(t, err)
	assert.False(t, quiescentPayload.Stale)
	assert.False(t, quiescentPayload.Recovering)

	// Now set WAL mtime to newer than MaterializedAt -> should mark stale
	newTime := time.Now().Add(10 * time.Minute)
	require.NoError(t, fileutil.Chtimes(walPath, newTime, newTime))
	payload.MaterializedAt = time.Now().Add(-1 * time.Hour)
	writePayloadFile(t, targetPath, payload)

	staleWalPayload, err := GetOrRecoverPayload(ctx, nil, tempDir, 10*time.Minute)
	require.NoError(t, err)
	assert.True(t, staleWalPayload.Stale)
	assert.True(t, staleWalPayload.Recovering)
	assert.Contains(t, staleWalPayload.DegradedReason, "materialized view watermark exceeds tolerance")
}

func TestCoverageExpansion_TriggerAsyncRebuildAndReconcile(t *testing.T) {
	t.Parallel()

	assert.False(t, TriggerAsyncRebuild("", nil))
	assert.False(t, TriggerAsyncRebuild(t.TempDir(), nil))

	// WaitForReconcile with short timeout
	assert.True(t, WaitForReconcile(10*time.Millisecond))
}

func TestCoverageExpansion_SubscribeWALBranches(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	view := NewWhatsNextMaterializedView(t.TempDir())
	ch := make(chan struct{}, 1)
	// With nil engine, calling SubscribeWAL with cancelled context returns quickly
	view.SubscribeWAL(ctx, ch)

	// StartBackgroundWALSubscriber starts goroutine and returns
	view.StartBackgroundWALSubscriber(ctx, ch)
}

func TestCoverageExpansion_CAPCycleHelpers(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// EnsureCAPStage with empty stage does nothing
	EnsureCAPStage(tempDir, "")

	// LastCapAdvanceJournalEntry on missing file returns false
	entry, ok := LastCapAdvanceJournalEntry(tempDir)
	assert.False(t, ok)
	assert.Equal(t, "", entry.NextStage)

	// AppendCapAdvanceJournal with empty timestamp and source
	err := AppendCapAdvanceJournal(tempDir, CapAdvanceJournalEntry{
		CompletedStage: CapStages[0],
		NextStage:      CapStages[1],
	})
	require.NoError(t, err)

	entry, ok = LastCapAdvanceJournalEntry(tempDir)
	assert.True(t, ok)
	assert.Equal(t, CapStages[1], entry.NextStage)
	assert.Equal(t, "cap_orchestrator", entry.Source)
	assert.NotEmpty(t, entry.Timestamp)

	// CapCycleTamperDetected: current stage defaults to CapStages[0], but journal says CapStages[1]
	assert.True(t, CapCycleTamperDetected(tempDir))

	// RestoreCAPStageFromJournal
	restored, ok := RestoreCAPStageFromJournal(tempDir)
	assert.True(t, ok)
	assert.Equal(t, CapStages[1], restored)
	assert.Equal(t, CapStages[1], PeekCAPStage(tempDir))
	assert.False(t, CapCycleTamperDetected(tempDir))
}

func TestCoverageExpansion_PriorityPlanPackagingCue(t *testing.T) {
	t.Parallel()

	// empty plan ID
	assert.Empty(t, PriorityPlanPackagingCue("", "title", "active", map[string]int{objects.ObjectStatusComplete: 1}))
	assert.Empty(t, PriorityPlanPackagingCue("  ", "title", "active", map[string]int{objects.ObjectStatusComplete: 1}))

	// ineligible status
	assert.Empty(t, PriorityPlanPackagingCue("PRI-1", "title", "planned", map[string]int{objects.ObjectStatusComplete: 1}))
	assert.Empty(t, PriorityPlanPackagingCue("PRI-1", "title", "proposed", map[string]int{objects.ObjectStatusComplete: 1}))

	// total == 0 or negative counts
	assert.Empty(t, PriorityPlanPackagingCue("PRI-1", "title", "active", map[string]int{objects.ObjectStatusComplete: 0, "closed": -1}))

	// open > 0
	assert.Empty(t, PriorityPlanPackagingCue("PRI-1", "title", "active", map[string]int{objects.ObjectStatusInProgress: 1, objects.ObjectStatusComplete: 1}))

	// finished == 0
	assert.Empty(t, PriorityPlanPackagingCue("PRI-1", "title", "active", map[string]int{objects.ObjectStatusPlanned: 1}))

	// valid cue with title
	cue := PriorityPlanPackagingCue("PRI-1", "Feature Title", "active", map[string]int{objects.ObjectStatusComplete: 2})
	assert.Contains(t, cue, "Feature Title (PRI-1)")
	assert.Contains(t, cue, "PRI≈PR wrap")

	// valid cue with empty title fallback to planID
	cueNoTitle := PriorityPlanPackagingCue("PRI-1", "", "active", map[string]int{objects.ObjectStatusComplete: 2})
	assert.Contains(t, cueNoTitle, "PRI-1 has no open children")
}
