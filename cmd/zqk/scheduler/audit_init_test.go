package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestAuditSchedulerInitFull(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.scheduler.audit_init"})
	t.Logf("Project Root: %s", p.Root)
	if p.FileStorage == nil {
		t.Fatal("p.FileStorage is nil")
	}
	t.Log("FileStorage initialized successfully")
	specLoader := objects.NewSpecLoader(p.Root)
	lifecycleLoader := objects.NewLifecycleLoader(p.Root)
	s := scheduler.NewSchedulerWithProjectRoot(p.FileStorage, specLoader, lifecycleLoader, p.Root, nil)
	s.SetSecurityContext(pkgctx.NewSystemSecurityContext())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test.audit_scheduler_init.start", "starting scheduler in audit init test").
		WithContext(ctx).
		StartSimple(func() {
			errCh <- s.Start(ctx)
		})

	t.Log("Scheduler initialized. Attempting Start()...")

	// Wait briefly to allow scheduler to start
	// Note: We don't need it to run fully, just to initialize without panicking
	<-time.After(100 * time.Millisecond)

	// Cancel context to stop scheduler
	cancel()

	err := <-errCh
	if err != nil {
		t.Fatalf("Scheduler Start failed: %v", err)
	}
	t.Log("Audit successful.")
}
