package community

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRateLimiter_TokenBucketReplenish(t *testing.T) {
	rl := NewProviderRateLimiter()
	providerID := "test-provider"

	// 2 tokens capacity, 10 tokens/sec refill
	rl.ConfigureProvider(providerID, 2.0, 10.0)

	if !rl.Allow(providerID) {
		t.Fatalf("expected token 1 to be allowed")
	}
	if !rl.Allow(providerID) {
		t.Fatalf("expected token 2 to be allowed")
	}
	if rl.Allow(providerID) {
		t.Fatalf("expected token 3 to be rejected immediately (bucket empty)")
	}

	// Wait 150ms to replenish at least 1 token
	time.Sleep(150 * time.Millisecond)

	if !rl.Allow(providerID) {
		t.Fatalf("expected replenished token to be allowed")
	}
}

func TestFairShareDispatcher_RoundRobinDispatch(t *testing.T) {
	dispatcher := NewProviderFairShareDispatcher(10)

	// Enqueue tasks for provider A and provider B
	if err := dispatcher.Enqueue(FairShareTask{ProviderID: "prov-A", Payload: "A-1"}); err != nil {
		t.Fatalf("enqueue A-1 failed: %v", err)
	}
	if err := dispatcher.Enqueue(FairShareTask{ProviderID: "prov-A", Payload: "A-2"}); err != nil {
		t.Fatalf("enqueue A-2 failed: %v", err)
	}
	if err := dispatcher.Enqueue(FairShareTask{ProviderID: "prov-B", Payload: "B-1"}); err != nil {
		t.Fatalf("enqueue B-1 failed: %v", err)
	}
	if err := dispatcher.Enqueue(FairShareTask{ProviderID: "prov-B", Payload: "B-2"}); err != nil {
		t.Fatalf("enqueue B-2 failed: %v", err)
	}

	ctx := context.Background()

	// Task 1 should be prov-A
	t1, err := dispatcher.DispatchNext(ctx)
	if err != nil || t1 == nil || t1.Payload != "A-1" {
		t.Fatalf("expected A-1, got %+v (err: %v)", t1, err)
	}

	// Task 2 should alternate to prov-B
	t2, err := dispatcher.DispatchNext(ctx)
	if err != nil || t2 == nil || t2.Payload != "B-1" {
		t.Fatalf("expected B-1, got %+v (err: %v)", t2, err)
	}

	// Task 3 should alternate back to prov-A
	t3, err := dispatcher.DispatchNext(ctx)
	if err != nil || t3 == nil || t3.Payload != "A-2" {
		t.Fatalf("expected A-2, got %+v (err: %v)", t3, err)
	}

	// Task 4 should alternate back to prov-B
	t4, err := dispatcher.DispatchNext(ctx)
	if err != nil || t4 == nil || t4.Payload != "B-2" {
		t.Fatalf("expected B-2, got %+v (err: %v)", t4, err)
	}

	// Empty queue should return nil, nil
	t5, err := dispatcher.DispatchNext(ctx)
	if err != nil || t5 != nil {
		t.Fatalf("expected nil when queues drained, got: %+v (err: %v)", t5, err)
	}
}

func TestRateLimiter_BurstExhaustionAndQueueFull(t *testing.T) {
	dispatcher := NewProviderFairShareDispatcher(2)

	if err := dispatcher.Enqueue(FairShareTask{ProviderID: "prov-C", Payload: "C-1"}); err != nil {
		t.Fatalf("enqueue C-1 failed: %v", err)
	}
	if err := dispatcher.Enqueue(FairShareTask{ProviderID: "prov-C", Payload: "C-2"}); err != nil {
		t.Fatalf("enqueue C-2 failed: %v", err)
	}

	// 3rd item should hit ErrDispatcherQueueFull
	err := dispatcher.Enqueue(FairShareTask{ProviderID: "prov-C", Payload: "C-3"})
	if !errors.Is(err, ErrDispatcherQueueFull) {
		t.Fatalf("expected ErrDispatcherQueueFull, got: %v", err)
	}
}

func TestFairShareDispatcher_ContextCancellation(t *testing.T) {
	dispatcher := NewProviderFairShareDispatcher(5)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := dispatcher.DispatchNext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
