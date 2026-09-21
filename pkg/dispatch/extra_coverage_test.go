package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestDispatch_Run_NilItem(t *testing.T) {
	err := Run(context.Background(), nil, func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for nil work item")
	}
}

func TestDispatch_Run_SuccessAndError(t *testing.T) {
	item := &WorkItem{
		OperationType: "unit_test_op",
		ProjectRoot:   t.TempDir(),
		Profile:       "system",
	}

	executed := false
	err := Run(context.Background(), item, func(ctx context.Context) error {
		executed = true
		if fn := pkgctx.GetValidationProgress(ctx); fn != nil {
			fn("stage_running", "doing work")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executed {
		t.Fatal("expected runner to be executed")
	}

	// Runner error
	customErr := errors.New("runner failed")
	err = Run(context.Background(), item, func(ctx context.Context) error {
		return customErr
	})
	if !errors.Is(err, customErr) {
		t.Fatalf("expected custom error, got: %v", err)
	}
}

func TestProgressState_SetGet(t *testing.T) {
	state := &progressState{}
	state.set("init", "starting process")
	stage, msg := state.get()
	if stage != "init" || msg != "starting process" {
		t.Fatalf("expected ('init', 'starting process'), got (%q, %q)", stage, msg)
	}
}

func TestRunProgressHeartbeat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := &progressState{}
	state.set("loading", "parsing specs")

	ec := coordination.GetCoordinator()
	var coord *coordination.Coordinator
	if c, ok := ec.(*coordination.Coordinator); ok {
		coord = c
	}
	helper := coordination.NewProgressHelper(coord, t.TempDir(), "op-1", "test_heartbeat", "system")

	// Run with very short interval so ticker fires
	go runProgressHeartbeat(ctx, helper, "test_heartbeat", state, 10*time.Millisecond)

	time.Sleep(25 * time.Millisecond)

	state.set("", "only message")
	time.Sleep(20 * time.Millisecond)

	state.set("only_stage", "")
	time.Sleep(20 * time.Millisecond)

	state.set("", "")
	time.Sleep(20 * time.Millisecond)

	cancel()
	time.Sleep(10 * time.Millisecond)
}
