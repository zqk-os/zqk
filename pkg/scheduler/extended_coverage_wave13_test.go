package scheduler

import (
	"context"
	"fmt"
	"os"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_RetentionMaxCount_DeepBranches(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-retention-maxcount-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("storage failed: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	ctx := context.Background()

	h := &RetentionToleranceHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	kind := objects.KindAgentTask

	// Seed 6 objects: 4 completed, 2 in_progress
	var ids []string
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("ATK-1785886324283087000-deep%05d", i)
		ids = append(ids, id)
		status := "completed"
		if i >= 4 {
			status = "in_progress"
		}
		_ = sp.Create(ctx, secCtx, map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          kind,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        status,
		})
	}

	// 1. enforceMaxCountBatchedList with empty allIDs (exercises fallback list loop, lines 591-673)
	delCount, err := h.enforceMaxCountBatchedList(
		ctx,
		secCtx,
		storageCtx,
		"SCH-maxcount-deep",
		kind,
		2, // toDelete = 2
		[]string{"in_progress"},
		2,  // batchSize
		10, // maxBatches
		1,  // workers
		6,  // count
		4,  // maxCount
		nil, // allIDs nil -> trigger fallback
	)
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
	t.Logf("enforceMaxCountBatchedList fallback deleted: %d", delCount)

	// 2. enforceMaxCountBatchedList with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = h.enforceMaxCountBatchedList(
		canceledCtx,
		secCtx,
		storageCtx,
		"SCH-maxcount-deep",
		kind,
		2,
		[]string{"in_progress"},
		2,
		10,
		1,
		6,
		4,
		ids,
	)

	// 3. enforceMaxCount with zero count or count <= maxCount
	n, err := h.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-job-mc", kind, 100, []string{"in_progress"}, 10, 5, 1)
	if err != nil || n != 0 {
		t.Errorf("expected 0 deleted when count <= maxCount, got %d, err %v", n, err)
	}

	// 4. enforceMaxCount with negative or zero batchSize / maxBatches defaults
	n2, _ := h.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-job-mc", kind, 1, []string{"in_progress"}, 0, 0, 0)
	t.Logf("enforceMaxCount with defaults deleted: %d", n2)
}
