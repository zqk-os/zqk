package accumulator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type mockSubscriber struct {
	started atomic.Bool
}

func (m *mockSubscriber) StartBackgroundWALSubscriber(ctx context.Context, updateCh chan<- struct{}) {
	m.started.Store(true)
}

func TestAccumulatorRegistry(t *testing.T) {
	mock1 := &mockSubscriber{}
	mock2 := &mockSubscriber{}

	RegisterSubscriber("mock_1", func(projectRoot string) WALSubscriber {
		if projectRoot != "/test/project" {
			t.Errorf("expected projectRoot /test/project, got %s", projectRoot)
		}
		return mock1
	})

	RegisterSubscriber("mock_2", func(projectRoot string) WALSubscriber {
		return mock2
	})

	registered := RegisteredSubscribers()
	hasMock1 := false
	hasMock2 := false
	for _, name := range registered {
		if name == "mock_1" {
			hasMock1 = true
		}
		if name == "mock_2" {
			hasMock2 = true
		}
	}
	if !hasMock1 || !hasMock2 {
		t.Fatalf("expected registered to include mock_1 and mock_2, got %v", registered)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	subs := StartAllSubscribers(ctx, "/test/project")
	if len(subs) < 2 {
		t.Fatalf("expected at least 2 subscribers started, got %d", len(subs))
	}

	if !mock1.started.Load() {
		t.Errorf("expected mock1 to be started")
	}
	if !mock2.started.Load() {
		t.Errorf("expected mock2 to be started")
	}

	// Nil or empty projectRoot should return nil
	emptySubs := StartAllSubscribers(ctx, "")
	if emptySubs != nil {
		t.Errorf("expected nil for empty projectRoot, got %v", emptySubs)
	}
}
