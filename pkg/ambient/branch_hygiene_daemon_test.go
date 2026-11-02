package ambient

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Traceability: BLI-1793578878406936000-670dab43, CRIT-1793578812700305000-d188af82

type mockBranchPruner struct {
	pruneFn func(ctx context.Context, dryRun bool) ([]string, error)
	calls   atomic.Int64
}

func (m *mockBranchPruner) PruneMergedBranches(ctx context.Context, dryRun bool) ([]string, error) {
	m.calls.Add(1)
	if m.pruneFn != nil {
		return m.pruneFn(ctx, dryRun)
	}
	return nil, nil
}

func TestBranchHygieneDaemon_StaticFloor(t *testing.T) {
	if _, err := NewBranchHygieneDaemon(nil); !errors.Is(err, ErrBranchHygieneEngineRequired) {
		t.Fatalf("expected ErrBranchHygieneEngineRequired, got %v", err)
	}

	pruner := &mockBranchPruner{}
	d, err := NewBranchHygieneDaemon(pruner)
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if d.ID() == "" || d.Role() == "" || d.Ecosystem() == "" {
		t.Fatalf("holon identity incomplete: %#v", d.Status())
	}
	if d.Status().Health.State != HolonStateRegistered {
		t.Fatalf("expected registered, got %s", d.Status().Health.State)
	}
}

func TestBranchHygieneDaemon_TickPrunesViaEngine(t *testing.T) {
	pruner := &mockBranchPruner{
		pruneFn: func(ctx context.Context, dryRun bool) ([]string, error) {
			if dryRun {
				t.Fatal("default tick must prune live (dryRun=false)")
			}
			return []string{"integration/pri-stale-1"}, nil
		},
	}
	d, err := NewBranchHygieneDaemon(pruner)
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	pruned, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(pruned) != 1 || pruned[0] != "integration/pri-stale-1" {
		t.Fatalf("unexpected pruned=%v", pruned)
	}
	if pruner.calls.Load() != 1 {
		t.Fatalf("expected 1 prune call, got %d", pruner.calls.Load())
	}
}

func TestBranchHygieneDaemon_TickFailClosed(t *testing.T) {
	want := errors.New("git unavailable")
	pruner := &mockBranchPruner{
		pruneFn: func(ctx context.Context, dryRun bool) ([]string, error) {
			return nil, want
		},
	}
	d, err := NewBranchHygieneDaemon(pruner)
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	pruned, err := d.Tick(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("expected engine error, got %v", err)
	}
	if pruned != nil {
		t.Fatalf("fail-closed must not report pruned branches on error: %v", pruned)
	}
	if d.Status().Health.State != HolonStateDegraded {
		t.Fatalf("expected degraded after tick failure, got %s", d.Status().Health.State)
	}
}

func TestBranchHygieneDaemon_StartStopLifecycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	pruner := &mockBranchPruner{}
	d, err := NewBranchHygieneDaemon(pruner, WithHygieneInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_branch_hygiene_daemon", "hygiene daemon lifecycle test").StartSimple(func() {
		done <- d.Start(ctx)
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for pruner.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if pruner.calls.Load() == 0 {
		t.Fatal("daemon never invoked prune")
	}
	if d.Status().Health.State != HolonStateRunning {
		t.Fatalf("expected running, got %s", d.Status().Health.State)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("start returned unexpected err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not exit after cancel")
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if d.Status().Health.State != HolonStateStopped {
		t.Fatalf("expected stopped, got %s", d.Status().Health.State)
	}
}

func TestBranchHygieneDaemon_DryRunOption(t *testing.T) {
	var sawDry atomic.Bool
	pruner := &mockBranchPruner{
		pruneFn: func(ctx context.Context, dryRun bool) ([]string, error) {
			sawDry.Store(dryRun)
			return []string{"integration/pri-preview"}, nil
		},
	}
	d, err := NewBranchHygieneDaemon(pruner, WithHygieneDryRun(true))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !sawDry.Load() {
		t.Fatal("expected dry-run prune")
	}
}
