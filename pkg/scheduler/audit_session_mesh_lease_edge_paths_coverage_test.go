package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_AuditAggregationSession_MeshLeaseEdgePhases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	handler := NewAuditAggregationHandler(sp).(*AuditAggregationHandler)

	job := &ScheduledJob{
		ID:       DefaultAuditEventAggregationSchedulerJobID,
		JobType:  JobTypeAuditEventAggregation,
		Category: CategoryMaintenance,
		EnvironmentVariables: map[string]string{
			"PRE_AGGREGATION_CLEANUP": "true",
			"ARCHIVE_ENABLED":         "true",
			"DELETE_ENABLED":          "false",
			"BATCH_SIZE":              "10",
			"MAX_BATCHES":             "2",
			"MAX_RUNTIME_SECONDS":     "600",
		},
		MaxRuntimeSeconds: 600,
	}

	ctxDeadline, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Minute))
	defer cancel()

	sess := &auditAggregationSession{
		handler:        handler,
		ctx:            ctxDeadline,
		job:            job,
		phaseDurations: make(map[string]float64),
	}
	_ = sess.loadConfiguration()
	sess.setupPhaseContexts()

	// 1. determineEffectiveRetention thresholds
	sess.deleteAfterDuration = 1 * time.Hour
	sess.earlyCountErr = nil

	sess.earlyEventCount = 100001
	sess.determineEffectiveRetention()

	sess.earlyEventCount = 25000
	sess.determineEffectiveRetention()

	sess.earlyEventCount = 6000
	sess.determineEffectiveRetention()

	// 2. runAggregation, post cleanup, and finalize
	res, _ := sess.runAggregation()
	sess.runPostAggregationCleanup(res)
	sess.finalize(res)

	// Create dummy result
	dummyResult := &storagepkg.AuditAggregationResult{
		MetricID:        "AAM-dummy-1",
		EventCount:      10,
		MetricsCreated:  2,
		WindowStart:     time.Now().Add(-1 * time.Hour),
		WindowEnd:       time.Now(),
		EventsProcessed: []string{"ev1", "ev2"},
	}
	sess.runPostAggregationCleanup(dummyResult)
	sess.finalize(dummyResult)
}

func TestExtended_MeshLeaseSupervision_SubprocessErrors(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewMeshLeaseSupervisionHandler(sp, tmpDir, logger).(*MeshLeaseSupervisionHandler)

	// 1. Non-file URI
	httpKernel := map[string]any{
		objects.FieldKeyID:            "KERNEL-http",
		objects.FieldKeyKind:          objects.KindProviderProfile,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyEndpoint:      "https://example.com/api",
	}
	_ = sp.Create(ctx, secCtx, httpKernel)
	err = handler.ensureSubprocess(ctx, "LEASE-http", "KERNEL-http")
	if err == nil {
		t.Errorf("expected error for non-file endpoint")
	}

	// 2. File URI with non-existent directory
	missingKernel := map[string]any{
		objects.FieldKeyID:            "KERNEL-missing-dir",
		objects.FieldKeyKind:          objects.KindProviderProfile,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyEndpoint:      "file:///path/to/nonexistent/project/dir",
	}
	_ = sp.Create(ctx, secCtx, missingKernel)
	err = handler.ensureSubprocess(ctx, "LEASE-missing", "KERNEL-missing-dir")
	if err == nil {
		t.Errorf("expected error for non-existent consumer project root")
	}
}

func TestExtended_RunWrapper_Execution_EdgePaths(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)

	job := &ScheduledJob{
		ID:          "SCH-rw-edge-1",
		JobType:     JobTypeRunWrapper,
		Category:    CategoryMaintenance,
		Command:     "echo",
		CommandArgs: []string{"edge_test"},
		EnvironmentVariables: map[string]string{
			"WRITE_JOB_LOG_FILES":              "true",
			"WRITE_SEPARATE_JOB_LOGS":          "true",
			"STREAM_LOG_DIR":                   filepath.Join(tmpDir, "logs"),
			"TEST_BUNDLE_METADATA_FINGERPRINT": "expected_fp",
		},
	}

	_ = h.executeRunWrapperCore(ctx, job)
}
