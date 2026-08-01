package swarm_test

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/swarm"
)

type mockRunner struct {
	fn func(ctx context.Context, sys, user string) (string, error)
}

func (m *mockRunner) Run(ctx context.Context, sys, user string) (string, error) {
	return m.fn(ctx, sys, user)
}

func TestDispatcher_Start(t *testing.T) {
	q := swarm.NewInMemoryTaskQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q.Enqueue(ctx, swarm.Task{ID: "t1", SystemPrompt: "sys", UserPrompt: "user"})

	var runCount int
	runner := &mockRunner{
		fn: func(c context.Context, sys, user string) (string, error) {
			runCount++
			return "done", nil
		},
	}

	dispatcher := swarm.NewDispatcher(q, []swarm.Runner{runner})
	results := dispatcher.Start(ctx)

	select {
	case res := <-results:
		if res.Task.ID != "t1" {
			t.Errorf("expected t1, got %s", res.Task.ID)
		}
		if res.Result != "done" {
			t.Errorf("expected done, got %s", res.Result)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for result")
	}

	cancel() // This should stop the dispatcher
}

func TestDispatcher_QueueClosed(t *testing.T) {
	q := swarm.NewInMemoryTaskQueue()
	ctx := context.Background()

	runner := &mockRunner{
		fn: func(c context.Context, sys, user string) (string, error) {
			return "done", nil
		},
	}

	dispatcher := swarm.NewDispatcher(q, []swarm.Runner{runner})
	results := dispatcher.Start(ctx)

	// Close the queue instead of canceling context.
	q.Close()

	// Wait for results channel to be closed, signifying that dispatcher stopped.
	select {
	case _, ok := <-results:
		if ok {
			t.Errorf("expected results channel to be closed, but got a value")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for dispatcher to stop after queue close")
	}
}
