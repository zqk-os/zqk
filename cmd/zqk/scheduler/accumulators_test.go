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
			t.Errorf("expected projectRoot %s, got %s", tempDir, projectRoot)
		}
		return sub
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
	time.Sleep(30 * time.Millisecond)
	_ = lifecycle.CloseLifecycleWAL(tempDir)
}
