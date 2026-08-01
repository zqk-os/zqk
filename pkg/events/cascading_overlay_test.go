package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/events"
)

// TestRouter_BitmaskSubscription already exists — kept for regression coverage.

// TestShape_KindAndNamespace verifies that Shape carries kind/namespace for
// localized policy inspector matching.
func TestShape_KindAndNamespace(t *testing.T) {
	s := events.Shape{
		Type:      events.EventTypeObjectMutated,
		TargetID:  "GOAL-001",
		SourceID:  "storage",
		Kind:      "goal",
		Namespace: "zqk:kernel",
	}
	if s.Kind != "goal" {
		t.Errorf("expected kind=goal, got %s", s.Kind)
	}
	if s.Namespace != "zqk:kernel" {
		t.Errorf("expected namespace=zqk:kernel, got %s", s.Namespace)
	}
}

// TestPolicyInspector_MatchesKindAndStatus verifies the PolicyInspector fires
// only when a shape matches its registered kind + disallowed status.
func TestPolicyInspector_MatchesKindAndStatus(t *testing.T) {
	insp := events.NewPolicyInspector(events.PolicyInspectorConfig{
		Kind:               "goal",
		DisallowedStatuses: []string{"deleted"},
	})

	match := insp.Inspect(events.Shape{
		Type:   events.EventTypeObjectMutated,
		Kind:   "goal",
		Status: "deleted",
	})
	if !match {
		t.Error("expected PolicyInspector to match deleted goal")
	}

	noMatch := insp.Inspect(events.Shape{
		Type:   events.EventTypeObjectMutated,
		Kind:   "goal",
		Status: "active",
	})
	if noMatch {
		t.Error("expected PolicyInspector to NOT match active goal")
	}

	wrongKind := insp.Inspect(events.Shape{
		Type:   events.EventTypeObjectMutated,
		Kind:   "milestone",
		Status: "deleted",
	})
	if wrongKind {
		t.Error("expected PolicyInspector to NOT match wrong kind")
	}
}

// TestCascadingSuspension verifies that when a PolicyInspector fires on a shape,
// it re-publishes EventTypeSuspensionTriggered to downstream subscribers.
func TestCascadingSuspension(t *testing.T) {
	router := events.NewRouter()

	// Policy inspector: any "goal" that becomes "deleted" triggers a suspension.
	policyInsp := events.NewPolicyInspector(events.PolicyInspectorConfig{
		Kind:               "goal",
		DisallowedStatuses: []string{"deleted"},
	})

	// Downstream listener: watches for suspension events.
	suspCh := make(chan events.Shape, 10)
	router.Subscribe(events.NewBitmaskInspector(events.EventTypeSuspensionTriggered), suspCh)

	// Wire the policy inspector so violations cascade into the router.
	router.SubscribeWithCascade(policyInsp, router)

	// Publish a mutation that violates the policy.
	router.Publish(events.Shape{
		Type:     events.EventTypeObjectMutated,
		TargetID: "GOAL-001",
		Kind:     "goal",
		Status:   "deleted",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	select {
	case received := <-suspCh:
		if received.Type != events.EventTypeSuspensionTriggered {
			t.Errorf("expected SuspensionTriggered, got %v", received.Type)
		}
		if received.TargetID != "GOAL-001" {
			t.Errorf("expected TargetID=GOAL-001, got %s", received.TargetID)
		}
	case <-ctx.Done():
		t.Fatal("timeout: cascading suspension event never received")
	}
}

// TestRouter_Unsubscribe verifies that unsubscribing a channel stops future deliveries.
func TestRouter_Unsubscribe(t *testing.T) {
	router := events.NewRouter()
	insp := events.NewBitmaskInspector(events.EventTypeSuspensionTriggered)
	ch := make(chan events.Shape, 10)

	router.Subscribe(insp, ch)
	router.Unsubscribe(insp, ch)

	router.Publish(events.Shape{Type: events.EventTypeSuspensionTriggered, TargetID: "x"})

	select {
	case <-ch:
		t.Fatal("should not receive after unsubscribe")
	default:
	}
}
