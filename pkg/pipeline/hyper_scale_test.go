package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestHyperScaleOrchestrator_Orchestrate(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		h := NewHyperScaleOrchestrator(2)
		taskIDs := []string{"task1", "task2", "task3"}

		var mu sync.Mutex
		executed := []string{}

		err := h.Orchestrate(context.Background(), taskIDs, func(ctx context.Context, taskID string) error {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			executed = append(executed, taskID)
			mu.Unlock()
			return nil
		})

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		if len(executed) != 3 {
			t.Fatalf("expected 3 tasks to be executed, got %d", len(executed))
		}
	})

	t.Run("error", func(t *testing.T) {
		h := NewHyperScaleOrchestrator(2)
		taskIDs := []string{"task1", "task2"}

		expectedErr := errors.New("dispatch error")
		err := h.Orchestrate(context.Background(), taskIDs, func(ctx context.Context, taskID string) error {
			if taskID == "task1" {
				return expectedErr
			}
			return nil
		})

		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v (via errors.Is), got %v", expectedErr, err)
		}
	})

	t.Run("context cancel while worker busy", func(t *testing.T) {
		h := NewHyperScaleOrchestrator(1)
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		release := make(chan struct{})
		orchDone := make(chan struct{})

		goroutinelabels.NewGoroutine("hyper_scale_test", "cancel-during-busy Orchestrate harness").
			StartSimple(func() {
				_ = h.Orchestrate(ctx, []string{"hold", "waiter"}, func(ctx context.Context, taskID string) error {
					if taskID == "hold" {
						close(started)
						<-release
					}
					return nil
				})
				close(orchDone)
			})

		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("hold task did not start")
		}
		cancel()
		close(release)

		select {
		case <-orchDone:
		case <-time.After(2 * time.Second):
			t.Fatal("Orchestrate did not return after cancel + release")
		}

		// Must not leak / hang: a fresh Orchestrate still completes.
		done := make(chan struct{})
		goroutinelabels.NewGoroutine("hyper_scale_test", "follow-up Orchestrate harness").
			StartSimple(func() {
				_ = h.Orchestrate(context.Background(), []string{"solo"}, func(ctx context.Context, taskID string) error {
					return nil
				})
				close(done)
			})
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("follow-up Orchestrate hung — pool/budget leak suspected")
		}
	})
}
