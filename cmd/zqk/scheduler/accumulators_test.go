package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/accumulator"
	"github.com/zqk-os/zqk/pkg/lifecycle"
)

type testSchedulerSubscriber struct {
	started bool
}

func (s *testSchedulerSubscriber) StartBackgroundWALSubscriber(ctx context.Context, updateCh chan<- struct{}) {
	s.started = true
}

func TestStartSupervisedAccumulators(t *testing.T) {
	tempDir := t.TempDir()

	sub := &testSchedulerSubscriber{}
	accumulator.RegisterSubscriber("scheduler_test_subscriber", func(projectRoot string) accumulator.WALSubscriber {
		if projectRoot != tempDir {
			return nil
		}
		return sub
	})
	defer accumulator.UnregisterSubscriber("scheduler_test_subscriber")
	t.Cleanup(func() {
		accumulator.UnregisterSubscriber("scheduler_test_subscriber")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	subscribers := StartSupervisedAccumulators(ctx, tempDir)
	if len(subscribers) == 0 {
		t.Fatalf("expected subscribers to be started")
	}

	if !sub.started {
		t.Errorf("expected test subscriber to be started by StartSupervisedAccumulators")
	}

	cancel()
	time.Sleep(300 * time.Millisecond)
	_ = lifecycle.CloseLifecycleWAL(tempDir)
}
