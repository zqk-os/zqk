package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestDriftEventHandler(t *testing.T) {
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)

	// Create scheduler with a project root so it initializes coordinationChannel
	s := NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, testRoot, nil)
	sched, ok := s.(*Scheduler)
	if !ok {
		t.Fatal("Expected *Scheduler")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler
	goroutinelabels.NewGoroutine("test-drift-scheduler-start", "starting scheduler for drift event test").
		StartSimple(func() {
			_ = sched.Start(ctx)
		})

	// Wait a moment for it to start
	time.Sleep(200 * time.Millisecond)

	// Add a mock job
	jobCtx, jobCancel := context.WithCancel(context.Background()) //nolint:gosec
	job := &ScheduledJob{
		ID:      "errant-job",
		Enabled: true,
	}
	job.executionCtx = jobCtx
	job.executionCancel = jobCancel

	sched.jobsMu.Lock()
	sched.jobs[job.ID] = job
	sched.jobsMu.Unlock()

	// Ensure the context is not canceled initially
	select {
	case <-jobCtx.Done():
		t.Fatal("Job context should not be canceled initially")
	default:
	}

	// Publish a drift event
	ev := Event{
		Type:      "drift_detected",
		Timestamp: time.Now(),
		JobID:     "errant-job",
		ProcessID: 1234,
	}
	err := sched.coordinationChannel.PublishEvent(ev)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// Wait deterministically for the handler to process the event
	select {
	case <-jobCtx.Done():
		// Success, job was canceled deterministically
	case <-time.After(2 * time.Second):
		t.Fatal("Job context was not canceled after drift event within timeout")
	}
}
