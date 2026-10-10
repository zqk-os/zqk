package audit

import (
	"context"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestCreatingEvent_GoroutineIsolation(t *testing.T) {
	// In the main test goroutine, we begin event creation.
	done := BeginEventCreation()
	defer done()

	if !IsCreatingEvent() {
		t.Fatal("expected IsCreatingEvent() to be true in the goroutine that called BeginEventCreation()")
	}

	// In a separate goroutine, IsCreatingEvent() must NOT be affected.
	var otherSawCreating bool
	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("audit_test", "verify goroutine isolation").StartSimple(func() {
		defer wg.Done()
		otherSawCreating = IsCreatingEvent()
	})
	wg.Wait()

	if otherSawCreating {
		t.Fatal("IsCreatingEvent() leaked to a different goroutine! It must be scoped to the caller's goroutine.")
	}
}

func TestCreatingEvent_ContextIsolation(t *testing.T) {
	ctx := context.Background()
	if IsCreatingEventContext(ctx) {
		t.Fatal("expected IsCreatingEventContext(nil) to be false")
	}

	markedCtx := WithCreatingEvent(ctx)
	if !IsCreatingEventContext(markedCtx) {
		t.Fatal("expected IsCreatingEventContext(markedCtx) to be true")
	}
	if IsCreatingEventContext(ctx) {
		t.Fatal("parent context should not be modified")
	}
}

func TestCurGoroutineID(t *testing.T) {
	gid1 := curGoroutineID()
	if gid1 == 0 {
		t.Fatal("expected curGoroutineID() to return valid non-zero goroutine ID")
	}

	var gid2 int64
	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("audit_test", "curGoroutineID check").StartSimple(func() {
		defer wg.Done()
		gid2 = curGoroutineID()
	})
	wg.Wait()

	if gid2 == 0 {
		t.Fatal("expected child goroutine to return valid non-zero goroutine ID")
	}
	if gid1 == gid2 {
		t.Fatalf("expected different goroutine IDs, got %d and %d", gid1, gid2)
	}
}
