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

func TestExtended_AuditAggregationSession_DeepPhases(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-audit-session-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
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
	storageCtx := pkgctx.NewStorageContext()

	handler := NewAuditAggregationHandler(sp).(*AuditAggregationHandler)
	handler.logger = logger
	handler.projectRoot = tmpDir

	job := &ScheduledJob{
		ID: "SCH-audit-deep-1",
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "1h",
			EnvKeyRetentionDuration: "2h",
			EnvKeyBatchSize:         "10",
			EnvKeyArchiveEnabled:    "true",
			EnvKeyDeleteEnabled:     "true",
		},
		MaxRuntimeSeconds: 300,
	}

	session := &auditAggregationSession{
		handler:             handler,
		ctx:                 ctx,
		job:                 job,
		secCtx:              secCtx,
		storageCtx:          storageCtx,
		service:             storagepkg.NewAuditAggregationServiceWithBatchSize(sp, 10),
		phaseDurations:      make(map[string]float64),
		windowDuration:      1 * time.Hour,
		deleteAfterDuration: 2 * time.Hour,
		effectiveRetention:  2 * time.Hour,
		batchSize:           10,
		windowStart:         time.Now().Add(-1 * time.Hour),
		windowEnd:           time.Now(),
		archiveEnabled:      true,
		deleteEnabled:       true,
	}

	// 1. loadConfiguration
	if err := session.loadConfiguration(); err != nil {
		t.Fatalf("loadConfiguration failed: %v", err)
	}
	if session.batchSize != 10 {
		t.Errorf("expected batch size 10, got %d", session.batchSize)
	}

	// 2. setupPhaseContexts and hasTimeRemaining
	session.setupPhaseContexts()
	if !session.hasTimeRemaining() {
		t.Errorf("expected time remaining in new phase context")
	}

	// 3. runPreChecks
	_, _ = session.runPreChecks()

	// 4. determineEffectiveRetention
	session.determineEffectiveRetention()

	// 5. Seed some audit events
	for i := 1; i <= 5; i++ {
		evID := fmt.Sprintf("AUD-deep-event-%d", i)
		evObj := map[string]any{
			objects.FieldKeyID:            evID,
			objects.FieldKeyKind:          objects.KindAuditEvent,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "originated",
			objects.FieldKeyCreatedAt:     time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
			"action":                      "test_action",
		}
		_ = sp.Create(ctx, secCtx, evObj)
	}

	// 6. runRetentionFirstPass & runCatchUp
	session.runRetentionFirstPass()
	session.runCatchUp()

	// 7. runAggressiveCleanup & runProactiveCleanup
	session.runAggressiveCleanup()
	session.runProactiveCleanup()

	// 8. runAggregation
	res, err := session.runAggregation()
	t.Logf("runAggregation res=%v, err=%v", res, err)

	// 9. If res is nil, synthesize a dummy result to test post-agg cleanup & finalize
	if res == nil {
		res = &storagepkg.AuditAggregationResult{
			EventCount:      2,
			MetricsCreated:  1,
			MetricID:        "BAS-mock-metric-1",
			EventsProcessed: []string{"AUD-deep-event-1", "AUD-deep-event-2"},
		}
	}

	// 10. runPostAggregationCleanup
	session.runPostAggregationCleanup(res)

	// 11. runRetentionSecondPass
	session.runRetentionSecondPass()

	// 12. finalize & cleanup
	session.finalize(res)
	session.cleanup()

	// 13. Pipeline execution of AuditAggregation
	pipeJob := &ScheduledJob{
		ID: "SCH-audit-pipe-1",
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "30m",
			EnvKeyRetentionDuration: "1h",
		},
		MaxRuntimeSeconds: 60,
	}
	_ = RunAuditAggregationViaPipeline(ctx, handler, pipeJob)
}
