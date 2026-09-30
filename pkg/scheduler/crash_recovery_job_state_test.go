package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestScheduler_CrashRecoveryJobStateReplay(t *testing.T) {
	sched, testRoot, cleanup := setupTestScheduler(t)
	defer cleanup()

	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	// 1. Create durable scheduled jobs in storage pre-crash
	job1ID := "SCH-TEST-DURABLE-001"
	job2ID := "SCH-TEST-DURABLE-002"
	createTestJob(t, sched.storage, job1ID, "run_wrapper", "timer", "*/5 * * * *")
	createTestJob(t, sched.storage, job2ID, "run_wrapper", "timer", "0 * * * *")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh1 := make(chan error, 1)
	goroutinelabels.NewGoroutine("test-crash-recovery-sched-start-1", "starting scheduler pre-crash").
		StartSimple(func() {
			errCh1 <- sched.Start(ctx)
		})
	time.Sleep(100 * time.Millisecond)

	// Verify jobs are registered in running scheduler
	j1, exists1 := sched.GetJob(job1ID)
	if !exists1 || j1 == nil {
		t.Fatalf("job 1 not found in running scheduler")
	}

	// 2. Simulate abrupt process termination / crash (stop pre-crash scheduler instance)
	sched.Stop()

	// 3. Restart: create fresh scheduler instance pointing at the same project root/storage
	storage := sched.storage
	specLoader := sched.specLoader
	lifecycleLoader := sched.lifecycleLoader

	restartedSchedIface := NewSchedulerWithProjectRoot(
		storage,
		specLoader,
		lifecycleLoader,
		testRoot,
		nil,
	)
	restartedSched := restartedSchedIface.(*Scheduler)
	restartedSched.SetSecurityContext(secCtx)

	// Start restarted scheduler - loads jobs from durable storage replay
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	errCh2 := make(chan error, 1)
	goroutinelabels.NewGoroutine("test-crash-recovery-sched-start-2", "starting restarted scheduler").
		StartSimple(func() {
			errCh2 <- restartedSched.Start(ctx2)
		})
	time.Sleep(100 * time.Millisecond)
	defer restartedSched.Stop()

	// 4. Assert durable job definitions survived crash and are active
	recovered1, recExists1 := restartedSched.GetJob(job1ID)
	if !recExists1 || recovered1 == nil {
		t.Fatalf("CRIT-CEF-R14-RCV-CRASH-REPLAY-001: job 1 lost after crash recovery restart")
	}
	if recovered1.ID != job1ID || recovered1.ScheduleExpr != "*/5 * * * *" {
		t.Errorf("job 1 state corrupted after restart: %+v", recovered1)
	}

	recovered2, recExists2 := restartedSched.GetJob(job2ID)
	if !recExists2 || recovered2 == nil {
		t.Fatalf("CRIT-CEF-R14-RCV-CRASH-REPLAY-001: job 2 lost after crash recovery restart")
	}
	if recovered2.ID != job2ID {
		t.Errorf("job 2 state corrupted after restart: %+v", recovered2)
	}
}
