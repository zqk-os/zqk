package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestCRIT9039_CoordinationChannel_PublishAndWatch(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cc := NewCoordinationChannel(tmpDir)

	sub := cc.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("scheduler_test", "watch coordination events").StartSimple(func() {
		errCh <- cc.WatchEvents(ctx)
	})
	// Give watcher time to open file and seek to end before publishing.
	time.Sleep(50 * time.Millisecond)

	want := Event{
		Type:        "job_execution_started",
		JobID:       "SCH-1",
		ExecutionID: "exec-1",
		ProcessID:   42,
	}
	if err := cc.PublishEvent(want); err != nil {
		t.Fatalf("PublishEvent failed: %v", err)
	}

	var got Event
	recvErr := testkit.RunNamedTestSteps(context.Background(), "scheduler.coordination_channel_wait",
		testkit.NamedTestStep{
			Name: "WAIT_SUBSCRIBER_EVENT",
			Fn: func() error {
				select {
				case got = <-sub:
					return nil
				case <-time.After(2 * time.Second):
					return context.DeadlineExceeded
				}
			},
		},
	)
	if recvErr != nil {
		t.Fatalf("timed out waiting for event")
	}

	{
		if got.Type != want.Type {
			t.Fatalf("expected Type=%q, got %q", want.Type, got.Type)
		}
		if got.JobID != want.JobID {
			t.Fatalf("expected JobID=%q, got %q", want.JobID, got.JobID)
		}
		if got.ExecutionID != want.ExecutionID {
			t.Fatalf("expected ExecutionID=%q, got %q", want.ExecutionID, got.ExecutionID)
		}
		if got.ProcessID != want.ProcessID {
			t.Fatalf("expected ProcessID=%d, got %d", want.ProcessID, got.ProcessID)
		}
	}

	cancel()
	stopErr := testkit.RunNamedTestSteps(context.Background(), "scheduler.coordination_channel_stop_wait",
		testkit.NamedTestStep{
			Name: "WAIT_WATCH_EVENTS_STOP",
			Fn: func() error {
				select {
				case err := <-errCh:
					return err
				case <-time.After(2 * time.Second):
					return context.DeadlineExceeded
				}
			},
		},
	)
	if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		if errors.Is(stopErr, context.DeadlineExceeded) {
			t.Fatalf("timed out waiting for WatchEvents to stop")
		}
		t.Fatalf("WatchEvents returned error: %v", stopErr)
	}
}
