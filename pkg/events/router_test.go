package events_test

import (
	"context"
	"github.com/zqk-os/zqk/pkg/objects"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/events"
)

func TestRouter_BitmaskSubscription(t *testing.T) {
	router := events.NewRouter()

	inspector := events.NewBitmaskInspector(events.EventTypeSuspensionTriggered)

	ch := make(chan events.Shape, 10)
	router.Subscribe(inspector, ch)

	router.Publish(events.Shape{Type: events.EventTypeSuspensionTriggered, TargetID: "obj-1"})
	router.Publish(events.Shape{Type: events.EventTypeUnknown, TargetID: "obj-2"})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	select {
	case received := <-ch:
		if received.TargetID != "obj-1" {
			t.Errorf("expected obj-1, got %s", received.TargetID)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}

	select {
	case <-ch:
		t.Fatal("should not receive second event")
	default:
	}
}

func TestRouter_PolicyInspectorCascade(t *testing.T) {
	router := events.NewRouter()

	cfg := events.PolicyInspectorConfig{
		Kind:               "milestone",
		Namespace:          "zqk:kernel",
		DisallowedStatuses: []string{objects.ObjectStatusBlocked, objects.ObjectStatusError},
	}
	inspector := events.NewPolicyInspector(cfg)

	// Create a mock publisher that captures cascade shapes
	var captured []events.Shape
	var mu sync.Mutex
	publisher := &mockPublisher{
		onPublish: func(s events.Shape) {
			mu.Lock()
			defer mu.Unlock()
			captured = append(captured, s)
		},
	}

	router.SubscribeWithCascade(inspector, publisher)

	// 1. Mutate a milestone to blocked (violates policy)
	router.Publish(events.Shape{
		Type:      events.EventTypeObjectMutated,
		TargetID:  "milestone-1",
		SourceID:  "user-1",
		Kind:      "milestone",
		Namespace: "zqk:kernel",
		Status:    objects.ObjectStatusBlocked,
	})

	// 2. Mutate a milestone to active (valid state)
	router.Publish(events.Shape{
		Type:      events.EventTypeObjectMutated,
		TargetID:  "milestone-2",
		SourceID:  "user-1",
		Kind:      "milestone",
		Namespace: "zqk:kernel",
		Status:    objects.ObjectStatusActive,
	})

	// 3. Mutate a goal to blocked (different kind)
	router.Publish(events.Shape{
		Type:      events.EventTypeObjectMutated,
		TargetID:  "goal-1",
		SourceID:  "user-1",
		Kind:      "goal",
		Namespace: "zqk:kernel",
		Status:    objects.ObjectStatusBlocked,
	})

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("expected exactly 1 cascade event, got %d", len(captured))
	}

	if captured[0].Type != events.EventTypeSuspensionTriggered {
		t.Errorf("expected EventTypeSuspensionTriggered, got %v", captured[0].Type)
	}
	if captured[0].TargetID != "milestone-1" {
		t.Errorf("expected target milestone-1, got %s", captured[0].TargetID)
	}
}

type mockPublisher struct {
	onPublish func(events.Shape)
}

func (m *mockPublisher) Publish(s events.Shape) {
	if m.onPublish != nil {
		m.onPublish(s)
	}
}
