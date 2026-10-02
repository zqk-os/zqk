package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_RetentionMaxCount_Wave40(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	fs, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = fs.Shutdown(context.Background())
		time.Sleep(100 * time.Millisecond)
	}()

	rth := NewRetentionToleranceHandler(fs, tmpDir).(*RetentionToleranceHandler)

	ctx := context.Background()
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-reason")
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Seed 10 agent tasks
	for i := 0; i < 10; i++ {
		task := map[string]any{
			objects.FieldKeyID:                 fmt.Sprintf("ATK-max-w40-%d", i),
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:          time.Now().Add(time.Duration(-i-1) * time.Hour).Format(time.RFC3339),
			objects.FieldKeyStatus:             objects.ObjectStatusError,
			objects.FieldKeyTitle:              fmt.Sprintf("Max Task %d", i),
			objects.FieldKeyDescription:        "Max task description",
			objects.FieldKeyAssigneePersonaRef: "software_engineer",
		}
		if err := fs.Create(bgCtx, secCtx, task); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	// 1. enforceMaxCount with maxCount = 5, batchSize = 2, maxBatches = 2 (triggers toDelete > maxToProcess capping)
	del1, err := rth.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-max-1", objects.KindAgentTask, 5, []string{objects.ObjectStatusInProgress}, 2, 2, 2)
	t.Logf("enforceMaxCount capped deleted: %d, err=%v", del1, err)

	// 2. enforceMaxCount again with remaining
	del2, err := rth.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-max-2", objects.KindAgentTask, 5, nil, 5, 2, 2)
	t.Logf("enforceMaxCount remaining deleted: %d, err=%v", del2, err)

	// 3. Direct calls on helpers
	// enforceMaxCountBatchedList with IDs
	delBatchIDs, err := rth.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, "SCH-max-batch-ids", objects.KindAgentTask, 1, nil, 2, 2, 2, 10, 5, []string{"ATK-max-w40-0"})
	t.Logf("enforceMaxCountBatchedList with IDs deleted: %d, err=%v", delBatchIDs, err)

	// enforceMaxCountBatchedList without IDs (standard list)
	delBatchNoIDs, err := rth.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, "SCH-max-batch-noids", objects.KindAgentTask, 1, nil, 2, 2, 2, 10, 5, nil)
	t.Logf("enforceMaxCountBatchedList without IDs deleted: %d, err=%v", delBatchNoIDs, err)

	// enforceMaxCountViaHVNoProtect
	delHVNoProt, doneNoProt := rth.enforceMaxCountViaHVNoProtect(ctx, secCtx, "SCH-max-hv-noprot", objects.KindAgentTask, 1, 2, 2, 5)
	t.Logf("enforceMaxCountViaHVNoProtect: %d, done=%v", delHVNoProt, doneNoProt)

	// enforceMaxCountViaHVWithProtect
	delHVProt, doneProt := rth.enforceMaxCountViaHVWithProtect(ctx, secCtx, "SCH-max-hv-prot", objects.KindAgentTask, 1, 2, 2, 5, []string{objects.ObjectStatusInProgress})
	t.Logf("enforceMaxCountViaHVWithProtect: %d, done=%v", delHVProt, doneProt)
}

func TestExtended_MetricsCleanupAndConvergenceTick_Wave40(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	fs, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = fs.Shutdown(context.Background())
	}()

	ctx := context.Background()
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-reason")
	secCtx := pkgctx.NewSystemSecurityContext()

	// Seed old audit_aggregation_metric objects
	oldTime := time.Now().Add(-100 * 24 * time.Hour).Format(time.RFC3339)
	for i := 0; i < 3; i++ {
		aam := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("MET-aam-%d", i),
			objects.FieldKeyKind:          objects.KindAuditAggregationMetric,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyCreatedAt:     oldTime,
		}
		_ = fs.Create(bgCtx, secCtx, aam)
	}

	cleaner := NewAggregationMetricsCleanupHandler(fs)
	job := &ScheduledJob{
		ID: "SCH-metrics-clean",
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "30",
		},
	}
	err = cleaner.Execute(ctx, job)
	if err != nil {
		t.Errorf("unexpected error in cleaner.Execute: %v", err)
	}

	// 2. ConvergenceSessionTick helpers
	// envLookup
	if envLookup(nil, "KEY") != "" {
		t.Errorf("expected empty for nil job")
	}
	jobWithEnv := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			"KEY1": "val1",
		},
	}
	if envLookup(jobWithEnv, "KEY1") != "val1" || envLookup(jobWithEnv, "KEY2") != "" {
		t.Errorf("unexpected envLookup")
	}

	// mergeStringAnyMaps
	merged := mergeStringAnyMaps(map[string]any{"a": 1}, map[string]any{"b": 2})
	if len(merged) != 2 || merged["a"] != 1 || merged["b"] != 2 {
		t.Errorf("unexpected merge: %v", merged)
	}
	if len(mergeStringAnyMaps(nil, nil)) != 0 {
		t.Errorf("expected empty for nil maps")
	}

	// truncateOrchestrateOutputPreview
	trunc := truncateOrchestrateOutputPreview("hello world", 5)
	if trunc != "hello…" {
		t.Errorf("expected %q, got %q", "hello…", trunc)
	}

	// thresholdMaxTicksPerHour
	if thresholdMaxTicksPerHour(nil) <= 0 {
		t.Errorf("expected positive default ticks per hour")
	}
	if thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": 15}) != 15 {
		t.Errorf("expected 15 from int")
	}
	if thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": float64(20)}) != 20 {
		t.Errorf("expected 20 from float")
	}
	if thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": "25"}) != 4 {
		t.Errorf("expected 4 default from string, got %d", thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": "25"}))
	}

	// countMeasureTicksInLastHour
	now := time.Now()
	activityLog := []any{
		map[string]any{"action": "measure_test_bundle_health", "timestamp": now.Add(-10 * time.Minute).Format(time.RFC3339)},
		map[string]any{"action": "measure_test_bundle_health", "timestamp": now.Add(-90 * time.Minute).Format(time.RFC3339)},
		map[string]any{"action": "other", "timestamp": now.Add(-5 * time.Minute).Format(time.RFC3339)},
		"invalid_entry",
	}
	count := countMeasureTicksInLastHour(activityLog, now)
	if count != 1 {
		t.Errorf("expected 1 measure tick in last hour, got %d", count)
	}
}
