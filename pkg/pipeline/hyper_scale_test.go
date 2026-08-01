package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
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
		if err.Error() != expectedErr.Error() {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})
}
