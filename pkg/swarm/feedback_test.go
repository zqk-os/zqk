package swarm_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/lanceman/zqk/pkg/swarm"
)

func TestFeedbackProcessor_Retry(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := swarm.NewInMemoryTaskQueue()
	processor := swarm.NewFeedbackProcessor(q)

	dispatcherResults := make(chan swarm.TaskResult)
	finalResults := processor.Process(ctx, dispatcherResults)

	task := swarm.Task{
		ID:           "test-task",
		SystemPrompt: "sys",
		UserPrompt:   "do something",
		MaxRetries:   1,
	}

	// Simulate a failure
	dispatcherResults <- swarm.TaskResult{
		Task:   task,
		Result: "",
		Error:  errors.New("simulated error"),
	}

	// Should have re-enqueued the task
	time.Sleep(100 * time.Millisecond) // Give goroutine time to process
	if q.Size() != 1 {
		t.Fatalf("expected queue size 1, got %d", q.Size())
	}

	retriedTask, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retriedTask.CurrentRetries != 1 {
		t.Errorf("expected CurrentRetries to be 1, got %d", retriedTask.CurrentRetries)
	}
	if !strings.Contains(retriedTask.UserPrompt, "System Feedback: Previous attempt failed") {
		t.Errorf("expected user prompt to contain feedback, got %s", retriedTask.UserPrompt)
	}

	// Simulate a second failure (max retries reached)
	dispatcherResults <- swarm.TaskResult{
		Task:   retriedTask,
		Result: "",
		Error:  errors.New("simulated error 2"),
	}

	// This one should go to finalResults
	select {
	case res := <-finalResults:
		if res.Task.ID != "test-task" {
			t.Errorf("expected test-task, got %s", res.Task.ID)
		}
		if res.Error == nil || res.Error.Error() != "simulated error 2" {
			t.Errorf("expected simulated error 2, got %v", res.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for final result")
	}

	close(dispatcherResults)
}

func TestFeedbackProcessor_Success(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := swarm.NewInMemoryTaskQueue()
	processor := swarm.NewFeedbackProcessor(q)

	dispatcherResults := make(chan swarm.TaskResult)
	finalResults := processor.Process(ctx, dispatcherResults)

	task := swarm.Task{
		ID:         "test-success",
		MaxRetries: 1,
	}

	// Simulate success
	dispatcherResults <- swarm.TaskResult{
		Task:   task,
		Result: "success",
		Error:  nil,
	}

	select {
	case res := <-finalResults:
		if res.Result != "success" {
			t.Errorf("expected success, got %s", res.Result)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for final result")
	}

	close(dispatcherResults)
}

func TestFeedbackProcessor_ContextCanceled(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := swarm.NewInMemoryTaskQueue()
	processor := swarm.NewFeedbackProcessor(q)

	dispatcherResults := make(chan swarm.TaskResult)
	finalResults := processor.Process(ctx, dispatcherResults)

	task := swarm.Task{
		ID:         "test-canceled",
		MaxRetries: 3,
	}

	// Simulate context cancellation error
	dispatcherResults <- swarm.TaskResult{
		Task:   task,
		Result: "",
		Error:  context.Canceled,
	}

	select {
	case res := <-finalResults:
		if res.Task.ID != "test-canceled" {
			t.Errorf("expected test-canceled, got %s", res.Task.ID)
		}
		if !errors.Is(res.Error, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", res.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for final result")
	}

	// Queue should be empty since we skipped retry
	if q.Size() != 0 {
		t.Errorf("expected queue size to be 0, got %d", q.Size())
	}

	close(dispatcherResults)
}
