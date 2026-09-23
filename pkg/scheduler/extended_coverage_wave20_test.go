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

func TestExtended_RetentionCleanupAndTolerance_DeepCoverage(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-retention-cleanup-deep-*")
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

	h := &RetentionToleranceHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	cutoffTime := time.Now().Add(10 * time.Hour) // in the future so created_at < cutoff matches
	cutoffStr := cutoffTime.Format(time.RFC3339)

	// Create old deletable tasks (status "error", created in past)
	bgCtx := pkgctx.WithLifecycleBreakGlass(ctx, "test-retention-cleanup")
	for i := 1; i <= 6; i++ {
		taskID := fmt.Sprintf("ATK-cleanup-del-%d", i)
		taskObj := map[string]any{
			objects.FieldKeyID:            taskID,
			objects.FieldKeyKind:          objects.KindAgentTask,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusError,
			objects.FieldKeyCreatedAt:     time.Now().Add(-10 * time.Hour).Format(time.RFC3339),
			objects.FieldKeyTitle:         fmt.Sprintf("Deletable Task %d", i),
		}
		if err := sp.Create(bgCtx, secCtx, taskObj); err != nil {
			t.Fatalf("failed to create taskObj: %v", err)
		}
	}

	// Create old protected tasks (status "in_progress", created in past)
	for i := 1; i <= 3; i++ {
		taskID := fmt.Sprintf("ATK-cleanup-prot-%d", i)
		taskObj := map[string]any{
			objects.FieldKeyID:            taskID,
			objects.FieldKeyKind:          objects.KindAgentTask,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
			objects.FieldKeyCreatedAt:     time.Now().Add(-10 * time.Hour).Format(time.RFC3339),
			objects.FieldKeyTitle:         fmt.Sprintf("Protected Task %d", i),
		}
		if err := sp.Create(bgCtx, secCtx, taskObj); err != nil {
			t.Fatalf("failed to create taskObj: %v", err)
		}
	}

	// 2. Test cleanupOldObjectsSlowPath
	deleted := h.cleanupOldObjectsSlowPath(ctx, secCtx, storageCtx, "SCH-ret-clean-1", objects.KindAgentTask, []string{objects.ObjectStatusInProgress}, 2, 2, 2, cutoffStr)
	t.Logf("cleanupOldObjectsSlowPath deleted %d items", deleted)

	// 3. Test cleanupOldObjects with remaining items
	deletedRemaining := h.cleanupOldObjects(ctx, secCtx, storageCtx, "SCH-ret-clean-2", objects.KindAgentTask, cutoffTime, []string{objects.ObjectStatusInProgress}, 5, 2, 2)
	t.Logf("cleanupOldObjects deletedRemaining: %d", deletedRemaining)

	// 4. Test enforceMaxCountBatchedList
	// Seed 8 backlog items (status "deferred" and status "exploring")
	for i := 1; i <= 8; i++ {
		status := objects.ObjectStatusDeferred
		if i%2 == 0 {
			status = objects.ObjectStatusExploring
		}
		bliID := fmt.Sprintf("BLI-maxcnt-%d", i)
		bliObj := map[string]any{
			objects.FieldKeyID:            bliID,
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        status,
			objects.FieldKeyCreatedAt:     time.Now().Add(-time.Duration(20-i) * time.Hour).Format(time.RFC3339),
			objects.FieldKeyTitle:         fmt.Sprintf("BLI %d", i),
		}
		if err := sp.Create(bgCtx, secCtx, bliObj); err != nil {
			t.Fatalf("failed to create bliObj: %v", err)
		}
	}

	// Call enforceMaxCount with maxCount = 4, protectStatuses = ["in_progress"]
	delMax, err := h.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-maxcount-1", objects.KindBacklogItem, 4, []string{objects.ObjectStatusInProgress}, 2, 5, 2)
	if err != nil {
		t.Fatalf("enforceMaxCount failed: %v", err)
	}
	t.Logf("enforceMaxCount deleted: %d", delMax)

	// Call enforceMaxCount with count <= maxCount (early return 0, nil)
	delZero, err := h.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-maxcount-2", objects.KindBacklogItem, 100, nil, 10, 2, 2)
	if err != nil || delZero != 0 {
		t.Errorf("expected 0 deletes, got %d, err: %v", delZero, err)
	}

	// Call enforceMaxCount with all items protected -> ErrObjectOverfill
	_, overfillErr := h.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-maxcount-3", objects.KindBacklogItem, 1, []string{objects.ObjectStatusDeferred, objects.ObjectStatusExploring}, 5, 2, 2)
	t.Logf("overfillErr: %v", overfillErr)

	// 5. Test getBatchConfig and getBulkDeleteWorkers
	jobConfig := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyBatchSize:         "100",
			EnvKeyMaxBatches:        "-1",
			EnvKeyBulkDeleteWorkers: "32",
		},
	}
	bs, mb := getBatchConfig(jobConfig)
	if bs != 100 || mb != retentionToleranceUnlimitedMaxBatches {
		t.Errorf("unexpected batch config: bs=%d mb=%d", bs, mb)
	}
	w := getBulkDeleteWorkers(jobConfig)
	if w != 32 {
		t.Errorf("unexpected workers: %d", w)
	}

	// Test with workers > max
	jobConfigMaxW := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyBulkDeleteWorkers: "1000",
		},
	}
	if getBulkDeleteWorkers(jobConfigMaxW) != maxBulkDeleteWorkers {
		t.Errorf("expected worker cap at %d, got %d", maxBulkDeleteWorkers, getBulkDeleteWorkers(jobConfigMaxW))
	}
}
