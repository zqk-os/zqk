package scheduler

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_HandlersMetricsCleanup_DeepCoverage(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-metrics-cleanup-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-metrics-cleanup")
	bgCtx = pkgctx.WithPromoteOnCreate(bgCtx)

	// 1. ChangeJournalAggregationHandler
	cjHandler := &ChangeJournalAggregationHandler{
		storage: sp,
		logger:  logger,
	}
	cjJob := &ScheduledJob{
		ID:      "SCH-cj-1",
		JobType: "change_journal_aggregation",
	}
	_ = cjHandler.preExecutionHealthCheck(ctx, cjJob, objects.KindAuditAggregationMetric)

	// 2. AggregationMetricsCleanupHandler
	aggCleanHandler := &AggregationMetricsCleanupHandler{
		storage: sp,
		logger:  logger,
	}
	aggCleanJob := &ScheduledJob{
		ID:      "SCH-aggclean-1",
		JobType: "aggregation_metrics_cleanup",
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "7",
		},
	}
	// Run when no metrics exist
	if err := aggCleanHandler.executeAggregationMetricsCleanupCore(ctx, aggCleanJob); err != nil {
		t.Errorf("executeAggregationMetricsCleanupCore failed on empty: %v", err)
	}

	// 3. GenericMetricsCleanupHandler
	genCleanHandler := &GenericMetricsCleanupHandler{
		storage: sp,
		logger:  logger,
	}

	// Case A: Missing METRIC_KIND env var -> error
	jobNoKind := &ScheduledJob{
		ID:      "SCH-genclean-nokind",
		JobType: "generic_metrics_cleanup",
	}
	if err := genCleanHandler.executeGenericMetricsCleanupCore(ctx, jobNoKind); err == nil {
		t.Errorf("expected error when METRIC_KIND is not set")
	}

	// Case B: Retention <= 0 -> skip (early return nil)
	jobZeroRet := &ScheduledJob{
		ID:      "SCH-genclean-zeroret",
		JobType: "generic_metrics_cleanup",
		EnvironmentVariables: map[string]string{
			EnvKeyMetricKind:    objects.KindAuditAggregationMetric,
			EnvKeyRetentionDays: "0",
		},
	}
	if err := genCleanHandler.executeGenericMetricsCleanupCore(ctx, jobZeroRet); err != nil {
		t.Errorf("expected nil for zero retention, got: %v", err)
	}

	// Case C: Seed old audit_aggregation_metric and delete
	cutoffPast := time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	for i := 1; i <= 3; i++ {
		metricID := fmt.Sprintf("AAM-clean-%d", i)
		if err := sp.Create(bgCtx, secCtx, map[string]any{
			objects.FieldKeyID:            metricID,
			objects.FieldKeyKind:          objects.KindAuditAggregationMetric,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyTitle:         fmt.Sprintf("Metric %d", i),
			objects.FieldKeyDescription:   "Old aggregation metric",
			objects.FieldKeyCreatedAt:     cutoffPast,
		}); err != nil {
			t.Logf("create metric err: %v", err)
		}
	}

	jobWithKind := &ScheduledJob{
		ID:      "SCH-genclean-run",
		JobType: "generic_metrics_cleanup",
		EnvironmentVariables: map[string]string{
			EnvKeyMetricKind:    objects.KindAuditAggregationMetric,
			EnvKeyRetentionDays: "7",
		},
	}
	if err := genCleanHandler.executeGenericMetricsCleanupCore(ctx, jobWithKind); err != nil {
		t.Errorf("executeGenericMetricsCleanupCore failed: %v", err)
	}

	// 4. preExecutionHealthCheck directly
	_ = preExecutionHealthCheck(ctx, sp, logger, jobWithKind, objects.KindAuditAggregationMetric)

	// 5. Interface factory constructors
	_ = NewChangeJournalAggregationHandler(sp)
	_ = NewAggregationMetricsCleanupHandler(sp)
	_ = NewGenericMetricsCleanupHandler(sp)
}
