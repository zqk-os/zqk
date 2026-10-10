package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestJobWakerRegistry_DispatchAndAwait(t *testing.T) {
	registry := NewJobWakerRegistry()
	defer registry.Close()

	targetJobID := "JOB-TEST-001"
	eventCh, unregister := registry.Register(targetJobID)
	defer unregister()

	expectedEvent := &LifecycleEvent{
		EventType: EventTypeSchedulerCallback,
		ID:        targetJobID,
		Kind:      "scheduler_job",
		ToStatus:  "completed",
		Ts:        time.Now(),
	}

	dispatched := registry.Dispatch(expectedEvent)
	if dispatched != 1 {
		t.Fatalf("expected 1 dispatched event, got %d", dispatched)
	}

	select {
	case received := <-eventCh:
		if received.ID != targetJobID {
			t.Errorf("expected job ID %s, got %s", targetJobID, received.ID)
		}
		if received.ToStatus != "completed" {
			t.Errorf("expected status completed, got %s", received.ToStatus)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for event on registered channel")
	}
}

func TestJobWakerRegistry_MultiListenerFanOut(t *testing.T) {
	registry := NewJobWakerRegistry()
	defer registry.Close()

	targetJobID := "JOB-FANOUT-001"
	ch1, unreg1 := registry.Register(targetJobID)
	defer unreg1()
	ch2, unreg2 := registry.Register(targetJobID)
	defer unreg2()

	ev := &LifecycleEvent{
		EventType: EventTypeSchedulerCallback,
		ID:        targetJobID,
		ToStatus:  "completed",
	}

	dispatched := registry.Dispatch(ev)
	if dispatched != 2 {
		t.Fatalf("expected 2 dispatched events, got %d", dispatched)
	}

	select {
	case <-ch1:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("listener 1 did not receive event")
	}

	select {
	case <-ch2:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("listener 2 did not receive event")
	}
}

func TestJobWakerRegistry_Wildcard(t *testing.T) {
	registry := NewJobWakerRegistry()
	defer registry.Close()

	wildcardCh, unregister := registry.RegisterWildcard()
	defer unregister()

	jobs := []string{"JOB-A", "JOB-B", "JOB-C"}
	for _, j := range jobs {
		ev := &LifecycleEvent{
			EventType: EventTypeSchedulerCallback,
			ID:        j,
			ToStatus:  "completed",
		}
		registry.Dispatch(ev)
	}

	for _, expectedJob := range jobs {
		select {
		case ev := <-wildcardCh:
			if ev.ID != expectedJob {
				t.Errorf("expected job %s, got %s", expectedJob, ev.ID)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for wildcard event for %s", expectedJob)
		}
	}
}

func TestJobWakerRegistry_AwaitJobTimeout(t *testing.T) {
	registry := NewJobWakerRegistry()
	defer registry.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := registry.AwaitJob(ctx, "JOB-NONEXISTENT")
	if err == nil {
		t.Fatalf("expected error on timeout, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestJobWakerRegistry_AwaitJobSuccess(t *testing.T) {
	registry := NewJobWakerRegistry()
	defer registry.Close()

	jobID := "JOB-AWAIT-OK"
	goroutinelabels.NewGoroutine("test_waker_dispatch", "dispatch event for await test").StartSimple(func() {
		time.Sleep(20 * time.Millisecond)
		registry.Dispatch(&LifecycleEvent{
			EventType: EventTypeSchedulerCallback,
			ID:        jobID,
			ToStatus:  "completed",
		})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ev, err := registry.AwaitJob(ctx, jobID)
	if err != nil {
		t.Fatalf("unexpected error awaiting job: %v", err)
	}
	if ev.ID != jobID || ev.ToStatus != "completed" {
		t.Errorf("unexpected event received: %+v", ev)
	}
}

func TestJobWakerRegistry_ConcurrentSafety(t *testing.T) {
	registry := NewJobWakerRegistry()
	defer registry.Close()

	const workers = 20
	const iterations = 50
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("test_waker_worker", "concurrent safety worker").StartSimple(func() {
			defer wg.Done()
			jobID := fmt.Sprintf("JOB-%d", workerID)
			for j := 0; j < iterations; j++ {
				ch, unregister := registry.Register(jobID)
				registry.Dispatch(&LifecycleEvent{
					EventType: EventTypeSchedulerCallback,
					ID:        jobID,
					ToStatus:  "running",
				})
				select {
				case <-ch:
				default:
				}
				unregister()
			}
		})
	}

	wg.Wait()
}

func TestListener_SchedulerCallbackIntegration(t *testing.T) {
	tempDir := t.TempDir()
	wal, err := NewLifecycleEventWAL(tempDir)
	if err != nil {
		t.Fatalf("failed to create WAL: %v", err)
	}

	listener := NewListener(tempDir, wal, nil, nil, nil)
	customWaker := NewJobWakerRegistry()
	listener.RegisterWaker(customWaker)

	jobID := "JOB-INTEGRATION-001"
	eventCh, unregister := customWaker.Register(jobID)
	defer unregister()

	walEvent := &LifecycleEvent{
		EventType: EventTypeSchedulerCallback,
		ID:        jobID,
		Kind:      "scheduler_job",
		ToStatus:  "completed",
		Ts:        time.Now(),
	}
	if err := wal.Append(walEvent); err != nil {
		t.Fatalf("failed to append WAL event: %v", err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("failed to sync WAL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_listener_run", "run listener for integration test").StartSimple(func() {
		runErrCh <- listener.Run(ctx)
	})

	select {
	case ev := <-eventCh:
		if ev.ID != jobID {
			t.Errorf("expected job ID %s, got %s", jobID, ev.ID)
		}
		if ev.ToStatus != "completed" {
			t.Errorf("expected status completed, got %s", ev.ToStatus)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for listener to process scheduler callback")
	}

	cancel()
	<-runErrCh
}
