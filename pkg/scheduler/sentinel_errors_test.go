package scheduler_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/zqk-os/zqk/pkg/scheduler"
)

func TestSchedulerSentinelErrors_ConformsToErrorsIs(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrJobNotFound", scheduler.ErrJobNotFound},
		{"ErrJobAlreadyRunning", scheduler.ErrJobAlreadyRunning},
		{"ErrJobCancelled", scheduler.ErrJobCancelled},
		{"ErrJobTimeout", scheduler.ErrJobTimeout},
		{"ErrSchedulerDown", scheduler.ErrSchedulerDown},
		{"ErrQueueFull", scheduler.ErrQueueFull},
		{"ErrWorkerPoolExhausted", scheduler.ErrWorkerPoolExhausted},
		{"ErrInvalidCronSchedule", scheduler.ErrInvalidCronSchedule},
		{"ErrJobValidationFailed", scheduler.ErrJobValidationFailed},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatalf("%s is nil", tc.name)
			}
			if tc.err.Error() == "" {
				t.Fatalf("%s has empty error message", tc.name)
			}

			// Verify %w wrapping and errors.Is unwrap conformance
			wrapped := fmt.Errorf("scheduler dispatch failed: %w", tc.err)
			if !errors.Is(wrapped, tc.err) {
				t.Errorf("expected errors.Is to match wrapped sentinel for %s", tc.name)
			}

			// Multi-level wrapping
			nested := fmt.Errorf("daemon supervisor: %w", wrapped)
			if !errors.Is(nested, tc.err) {
				t.Errorf("expected errors.Is to match nested wrapped sentinel for %s", tc.name)
			}
		})
	}
}
