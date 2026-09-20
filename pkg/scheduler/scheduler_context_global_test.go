package scheduler

import (
	"context"
	"testing"
	"time"
)

// setAdmissionTimeoutForTest overrides the one-shot admission hourglass. Pass 0 to clear.
func setAdmissionTimeoutForTest(d time.Duration) {
	if d <= 0 {
		testAdmissionTimeoutNS.Store(0)
		return
	}
	testAdmissionTimeoutNS.Store(int64(d))
}

func TestSchedulerContextGlobal_WithEventData(t *testing.T) {
	ctx := context.Background()
	data := map[string]any{"key": "value"}
	ctxWithData := WithEventData(ctx, data)
	if ctxWithData == nil {
		t.Fatal("expected non-nil context")
	}
}
