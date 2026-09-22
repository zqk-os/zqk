// BLI-STARTER-COMMUNITY-039 / PRI-STARTER-COMMUNITY-039 coverage elevation
package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
)

func TestExtraRuntimeHelpers(t *testing.T) {
	coord := &MockCoordinator{}
	if _, _, err := StartPeriodicTask(nil, "x", time.Second, nil); err == nil {
		t.Fatal("nil manager")
	}
	mgr := NewGoroutineManager(context.Background(), coord)
	t.Cleanup(func() { _ = mgr.Shutdown() })
	if _, _, err := StartPeriodicTask(mgr, "x", 0, nil); err == nil {
		t.Fatal("interval")
	}
	id, _, err := StartPeriodicTask(mgr, "tick", 50*time.Millisecond, func(context.Context) error { return errors.New("stop") })
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	_ = mgr.Stop(id)

	if _, err := StartWorkerPool(nil, "w", 1, func(context.Context, int) error { return nil }); err == nil {
		t.Fatal("nil manager pool")
	}
	if _, err := StartWorkerPool(mgr, "w", 0, func(context.Context, int) error { return nil }); err == nil {
		t.Fatal("count")
	}
	if _, err := StartWorkerPool(mgr, "w", 1, nil); err == nil {
		t.Fatal("nil fn")
	}
	ids, err := StartWorkerPool(mgr, "w", 2, func(ctx context.Context, _ int) error {
		<-ctx.Done()
		return nil
	})
	if err != nil || len(ids) != 2 {
		t.Fatalf("pool = %v %v", ids, err)
	}

	if err := EmitEventAsync(nil, nil, nil); err == nil {
		t.Fatal("nil coordinator")
	}
	if err := EmitEventAsync(nil, coord, nil); err == nil {
		t.Fatal("nil event")
	}
	ev := coordination.NewEventContext("op-1", "op", "complete")
	if err := EmitEventAsync(nil, coord, ev); err != nil {
		t.Fatal(err)
	}
	if err := EmitEventAsync(mgr, coord, ev); err != nil {
		t.Fatal(err)
	}

	_, _, errCh := ExecuteAsync(nil, "async", func(context.Context) error { return nil })
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("async fallback timeout")
	}
	_, _, errCh = ExecuteAsync(mgr, "async2", func(context.Context) error { return errors.New("x") })
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("async tracked timeout")
	}

	svc := NewBaseService(coord)
	if svc.GetGoroutineManager() == nil {
		t.Fatal("manager")
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
	empty := &BaseService{}
	if err := empty.Stop(); err != nil {
		t.Fatal(err)
	}
}
