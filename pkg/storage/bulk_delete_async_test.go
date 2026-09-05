package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestBulkDeleteJobManager_LifetimeCounters(t *testing.T) {
	mock := &mockStorageProvider{}
	mgr := NewBulkDeleteJobManager(mock)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	job, err := mgr.CreateJob(ctx, []string{"id1", "id2"})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	created, completed := mgr.GetJobManagerStats()
	if created != 1 {
		t.Errorf("expected created=1, got %d", created)
	}
	if completed != 0 {
		t.Errorf("expected completed=0, got %d", completed)
	}

	err = mgr.ExecuteJob(ctx, secCtx, job.ID, false, 2)
	if err != nil {
		t.Fatalf("failed to execute job: %v", err)
	}

	// Poll briefly for async background goroutine to complete post-cleanup
	success := false
	for i := 0; i < 20; i++ {
		time.Sleep(10 * time.Millisecond)
		_, completed = mgr.GetJobManagerStats()
		if completed == 1 {
			success = true
			break
		}
	}

	if !success {
		t.Errorf("expected completed=1 after async execution, got %d", completed)
	}
}
