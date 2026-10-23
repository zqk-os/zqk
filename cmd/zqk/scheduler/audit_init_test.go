package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestAuditSchedulerInitFull(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.scheduler.audit_init"})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(p.Root)
	})
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

	// Wait briefly to allow scheduler to start without panicking
	for i := 0; i < 20; i++ {
		if s.IsRunning() {
			break
		}
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
				t.Fatalf("Scheduler Start failed: %v", err)
			}
			t.Log("Audit successful (exited early).")
			return
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Cancel context to stop scheduler
	cancel()

	err := <-errCh
	if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Scheduler Start failed: %v", err)
	}
	t.Log("Audit successful.")
}
