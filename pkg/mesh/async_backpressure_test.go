package mesh

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestAsyncBackpressureThrottling(t *testing.T) {
	cfg := BackpressureConfig{
		HighWatermark: 10,
		LowWatermark:  5,
	}
	ctrl := NewBackpressureController(cfg)

	if ctrl.IsThrottled() {
		t.Errorf("expected controller to start in unthrottled state")
	}

	// Record up to 9
	ctrl.RecordEnqueue(9)
	if ctrl.IsThrottled() {
		t.Errorf("expected unthrottled under high watermark")
	}

	// Record at high watermark
	ctrl.RecordEnqueue(10)
	if !ctrl.IsThrottled() {
		t.Errorf("expected throttled at high watermark")
	}

	// Drain slightly to 8 (still above low watermark)
	ctrl.RecordDrain(8)
	if !ctrl.IsThrottled() {
		t.Errorf("expected throttled hysteresis between low and high watermark")
	}

	// Drain to low watermark
	ctrl.RecordDrain(5)
	if ctrl.IsThrottled() {
		t.Errorf("expected unthrottled at or below low watermark")
	}
}

func TestAsyncBackpressureWaitUntilHealthy(t *testing.T) {
	cfg := BackpressureConfig{
		HighWatermark: 5,
		LowWatermark:  2,
	}
	ctrl := NewBackpressureController(cfg)

	ctrl.RecordEnqueue(5)
	if !ctrl.IsThrottled() {
		t.Fatal("expected controller to be throttled")
	}

	unblocked := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	goroutinelabels.NewGoroutine("test", "wait-healthy").StartSimple(func() {
		err := ctrl.WaitUntilHealthy(ctx)
		if err == nil {
			close(unblocked)
		}
	})

	time.Sleep(50 * time.Millisecond)
	select {
	case <-unblocked:
		t.Fatal("unblocked prematurely before drain")
	default:
	}

	ctrl.RecordDrain(2)

	select {
	case <-unblocked:
		// Succeeded
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for unblock")
	}
}
