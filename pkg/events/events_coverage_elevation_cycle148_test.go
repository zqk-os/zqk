package events_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/events"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestCycle148_BitmaskInspector_AllBranches tests all branches of BitmaskInspector.
func TestCycle148_BitmaskInspector_AllBranches(t *testing.T) {
	t.Parallel()

	// Zero mask never matches non-zero type
	zeroInsp := events.NewBitmaskInspector(0)
	require.NotNil(t, zeroInsp)
	assert.False(t, zeroInsp.Inspect(events.Shape{Type: events.EventTypeSuspensionTriggered}))
	assert.False(t, zeroInsp.Inspect(events.Shape{Type: events.EventTypeObjectMutated}))

	// Multi-bit mask matches either bit
	multiMask := events.EventTypeSuspensionTriggered | events.EventTypeObjectMutated
	multiInsp := events.NewBitmaskInspector(multiMask)
	assert.True(t, multiInsp.Inspect(events.Shape{Type: events.EventTypeSuspensionTriggered}))
	assert.True(t, multiInsp.Inspect(events.Shape{Type: events.EventTypeObjectMutated}))
	assert.False(t, multiInsp.Inspect(events.Shape{Type: events.EventTypeUnknown}))
	assert.False(t, multiInsp.Inspect(events.Shape{Type: 1 << 5}))
}

// TestCycle148_PolicyInspector_NamespaceBranches tests all namespace filter branches.
func TestCycle148_PolicyInspector_NamespaceBranches(t *testing.T) {
	t.Parallel()

	cfg := events.PolicyInspectorConfig{
		Kind:               "goal",
		Namespace:          "zqk:custom",
		DisallowedStatuses: []string{objects.ObjectStatusBlocked},
	}
	insp := events.NewPolicyInspector(cfg)
	require.NotNil(t, insp)

	// Non-matching namespace must return false
	resWrongNs := insp.Inspect(events.Shape{
		Kind:      "goal",
		Namespace: "zqk:other",
		Status:    objects.ObjectStatusBlocked,
	})
	assert.False(t, resWrongNs)

	// Matching namespace and disallowed status must return true
	resMatch := insp.Inspect(events.Shape{
		Kind:      "goal",
		Namespace: "zqk:custom",
		Status:    objects.ObjectStatusBlocked,
	})
	assert.True(t, resMatch)

	// Matching namespace but allowed status must return false
	resAllowed := insp.Inspect(events.Shape{
		Kind:      "goal",
		Namespace: "zqk:custom",
		Status:    objects.ObjectStatusActive,
	})
	assert.False(t, resAllowed)
}

// TestCycle148_PolicyInspector_EmptyFilters tests wildcards on kind and namespace.
func TestCycle148_PolicyInspector_EmptyFilters(t *testing.T) {
	t.Parallel()

	// Empty kind and empty namespace matches all kinds and namespaces
	insp := events.NewPolicyInspector(events.PolicyInspectorConfig{
		DisallowedStatuses: []string{objects.ObjectStatusFailed},
	})

	assert.True(t, insp.Inspect(events.Shape{
		Kind:      "any_kind",
		Namespace: "any_namespace",
		Status:    objects.ObjectStatusFailed,
	}))

	assert.False(t, insp.Inspect(events.Shape{
		Kind:      "any_kind",
		Namespace: "any_namespace",
		Status:    objects.ObjectStatusCompleted,
	}))
}

// TestCycle148_Router_BufferFullDrop tests the non-blocking publish branch when channel is full.
func TestCycle148_Router_BufferFullDrop(t *testing.T) {
	t.Parallel()

	router := events.NewRouter()
	require.NotNil(t, router)

	insp := events.NewBitmaskInspector(events.EventTypeObjectMutated)
	// Channel with buffer 1
	ch := make(chan events.Shape, 1)
	router.Subscribe(insp, ch)

	// First publish succeeds and fills the buffer
	router.Publish(events.Shape{
		Type:     events.EventTypeObjectMutated,
		TargetID: "target-1",
	})

	// Second publish drops event without blocking or deadlocking
	router.Publish(events.Shape{
		Type:     events.EventTypeObjectMutated,
		TargetID: "target-2",
	})

	// Only target-1 was buffered
	select {
	case received := <-ch:
		assert.Equal(t, "target-1", received.TargetID)
	default:
		t.Fatal("expected target-1 in channel")
	}

	// Channel is now empty
	select {
	case <-ch:
		t.Fatal("unexpected extra event in channel")
	default:
	}
}

// TestCycle148_Router_UnsubscribeEdgeCases tests unsubscribing missing channels and middle channels.
func TestCycle148_Router_UnsubscribeEdgeCases(t *testing.T) {
	t.Parallel()

	router := events.NewRouter()
	insp := events.NewBitmaskInspector(events.EventTypeSuspensionTriggered)

	ch1 := make(chan events.Shape, 5)
	ch2 := make(chan events.Shape, 5)
	ch3 := make(chan events.Shape, 5)
	missingCh := make(chan events.Shape, 5)

	router.Subscribe(insp, ch1)
	router.Subscribe(insp, ch2)
	router.Subscribe(insp, ch3)

	// Unsubscribe missing channel is a safe no-op
	router.Unsubscribe(insp, missingCh)

	// Unsubscribe ch2 from middle of the slice
	router.Unsubscribe(insp, ch2)

	// Publish an event
	router.Publish(events.Shape{
		Type:     events.EventTypeSuspensionTriggered,
		TargetID: "target-sub",
	})

	// ch1 and ch3 receive it, ch2 does not
	assert.Equal(t, "target-sub", (<-ch1).TargetID)
	assert.Equal(t, "target-sub", (<-ch3).TargetID)

	select {
	case <-ch2:
		t.Fatal("unsubscribed ch2 received event")
	default:
	}

	// Unsubscribe from unregistered inspector is a safe no-op
	unregInsp := events.NewBitmaskInspector(events.EventTypeObjectMutated)
	router.Unsubscribe(unregInsp, ch1)
}

// TestCycle148_Router_CascadeWithPayload tests cascading suspension and opaque payload propagation.
func TestCycle148_Router_CascadeWithPayload(t *testing.T) {
	t.Parallel()

	router := events.NewRouter()

	insp := events.NewPolicyInspector(events.PolicyInspectorConfig{
		Kind:               "backlog_item",
		Namespace:          "zqk:kernel",
		DisallowedStatuses: []string{objects.ObjectStatusBlocked},
	})

	var mu sync.Mutex
	var cascaded []events.Shape
	mockPub := &mockPublisher{
		onPublish: func(s events.Shape) {
			mu.Lock()
			defer mu.Unlock()
			cascaded = append(cascaded, s)
		},
	}

	router.SubscribeWithCascade(insp, mockPub)

	type testPayload struct {
		Reason string
	}

	router.Publish(events.Shape{
		Type:      events.EventTypeObjectMutated,
		TargetID:  "BLI-001",
		SourceID:  "scheduler",
		Kind:      "backlog_item",
		Namespace: "zqk:kernel",
		Status:    objects.ObjectStatusBlocked,
		Payload:   testPayload{Reason: "dependency failed"},
	})

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, cascaded, 1)
	assert.Equal(t, events.EventTypeSuspensionTriggered, cascaded[0].Type)
	assert.Equal(t, "BLI-001", cascaded[0].TargetID)
	assert.Equal(t, "scheduler", cascaded[0].SourceID)
	assert.Equal(t, "backlog_item", cascaded[0].Kind)
	assert.Equal(t, "zqk:kernel", cascaded[0].Namespace)
}

// TestCycle148_Router_ConcurrentPublishSubscribe verifies thread safety under high concurrency.
func TestCycle148_Router_ConcurrentPublishSubscribe(t *testing.T) {
	t.Parallel()

	router := events.NewRouter()
	insp := events.NewBitmaskInspector(events.EventTypeObjectMutated)

	const workerCount = 10
	var wg sync.WaitGroup
	wg.Add(workerCount * 2)

	for i := 0; i < workerCount; i++ {
		goroutinelabels.NewGoroutine("cycle148_test", "subscriber worker").StartSimple(func() {
			defer wg.Done()
			ch := make(chan events.Shape, 50)
			router.Subscribe(insp, ch)
			time.Sleep(2 * time.Millisecond)
			router.Unsubscribe(insp, ch)
		})
	}

	for i := 0; i < workerCount; i++ {
		goroutinelabels.NewGoroutine("cycle148_test", "publisher worker").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				router.Publish(events.Shape{
					Type:     events.EventTypeObjectMutated,
					TargetID: "concurrent-target",
				})
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("cycle148_test", "wait coordinator").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("concurrent publish/subscribe test timed out")
	}
}
