package ambient

import (
	"context"
	"testing"
	"time"
)

func TestNoopService(t *testing.T) {
	svc := NewNoopService()

	if status := svc.Status(); status != "disabled (noop)" {
		t.Errorf("expected status 'disabled (noop)', got %q", status)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := svc.Start(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}
