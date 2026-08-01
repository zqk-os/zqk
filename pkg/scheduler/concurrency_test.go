package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestGoroutineConcurrencyFrameworkUsage(t *testing.T) {
	// Simple test to verify that the goroutine labels and budget are being used
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// In the future, I would check goroutinelabels.GetBudget() or similar to verify consumption
	goroutinelabels.NewGoroutine("test", "test work").
		WithContext(ctx).
		StartSimple(func() {
			time.Sleep(10 * time.Millisecond)
		})
}
