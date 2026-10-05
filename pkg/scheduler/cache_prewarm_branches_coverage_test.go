package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

type mockValidationScanner struct {
	count int
	err   error
}

func (m *mockValidationScanner) EnqueueAll(ctx context.Context, projectRoot string) (int, error) {
	return m.count, m.err
}

func TestExtended_CachePrewarm_DeepBranches(t *testing.T) {
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("storage failed: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewCachePrewarmHandler(nil, nil, sp, tmpDir, nil).(*CachePrewarmHandler)
	h.logger = logger

	job := &ScheduledJob{
		ID:                "SCH-cp-deep-1",
		JobType:           "cache_prewarm",
		MaxRuntimeSeconds: 1,
	}

	// 1. Execute with short deadline (< 30s remaining)
	shortCtx, cancelShort := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancelShort()
	err = h.Execute(shortCtx, job)
	if err != nil {
		t.Errorf("expected nil error for short deadline skip, got %v", err)
	}

	// 2. getProjectRoot branches
	// A. Explicit projectRoot
	if h.getProjectRoot() != tmpDir {
		t.Errorf("expected %s, got %s", tmpDir, h.getProjectRoot())
	}

	// B. Empty projectRoot but FileObjectStorage available
	hEmpty := &CachePrewarmHandler{storage: sp}
	if hEmpty.getProjectRoot() != sp.GetProjectRoot() {
		t.Errorf("expected %s, got %s", sp.GetProjectRoot(), hEmpty.getProjectRoot())
	}

	// C. Empty projectRoot and nil storage
	hNil := &CachePrewarmHandler{}
	if hNil.getProjectRoot() != "" {
		t.Errorf("expected empty string, got %s", hNil.getProjectRoot())
	}

	// 3. prewarmEnqueueValidation branches
	// A. Canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = h.prewarmEnqueueValidation(canceledCtx)

	// B. validationScanner is nil
	h.validationScanner = nil
	_ = h.prewarmEnqueueValidation(context.Background())

	// C. projectRoot is empty
	hEmptyScanner := &CachePrewarmHandler{
		validationScanner: &mockValidationScanner{count: 1},
		logger:            logger,
	}
	_ = hEmptyScanner.prewarmEnqueueValidation(context.Background())

	// D. validationScanner returns items enqueued
	hWithScanner := &CachePrewarmHandler{
		projectRoot:       tmpDir,
		validationScanner: &mockValidationScanner{count: 5},
		logger:            logger,
	}
	_ = hWithScanner.prewarmEnqueueValidation(context.Background())

	// E. validationScanner returns 0 items
	hZeroScanner := &CachePrewarmHandler{
		projectRoot:       tmpDir,
		validationScanner: &mockValidationScanner{count: 0},
		logger:            logger,
	}
	_ = hZeroScanner.prewarmEnqueueValidation(context.Background())

	// F. validationScanner returns error
	hErrScanner := &CachePrewarmHandler{
		projectRoot:       tmpDir,
		validationScanner: &mockValidationScanner{err: errors.New("scan failed")},
		logger:            logger,
	}
	_ = hErrScanner.prewarmEnqueueValidation(context.Background())
}
