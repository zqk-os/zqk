package community

import (
	"context"
	"errors"
	"testing"
)

func TestDegradationCoordinator_PrimarySuccess(t *testing.T) {
	coord := NewDegradationCoordinator(10)
	coord.RegisterPlan(ProviderDegradationPlan{
		PrimaryID: "provider-primary",
		Policy:    FallbackPolicyFailover,
	})
	coord.RegisterAction("provider-primary", func(ctx context.Context) (any, error) {
		return "primary-data", nil
	})

	res, err := coord.Execute(context.Background(), "provider-primary")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res != "primary-data" {
		t.Fatalf("expected 'primary-data', got %v", res)
	}
	if len(coord.EventHistory()) != 0 {
		t.Fatalf("expected 0 degradation events on success, got %d", len(coord.EventHistory()))
	}
}

func TestDegradationCoordinator_FailoverSuccess(t *testing.T) {
	coord := NewDegradationCoordinator(10)
	coord.RegisterPlan(ProviderDegradationPlan{
		PrimaryID:   "provider-primary",
		FallbackIDs: []string{"provider-secondary", "provider-tertiary"},
		Policy:      FallbackPolicyFailover,
	})

	coord.RegisterAction("provider-primary", func(ctx context.Context) (any, error) {
		return nil, errors.New("primary timeout")
	})
	coord.RegisterAction("provider-secondary", func(ctx context.Context) (any, error) {
		return "secondary-fallback-data", nil
	})

	res, err := coord.Execute(context.Background(), "provider-primary")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res != "secondary-fallback-data" {
		t.Fatalf("expected 'secondary-fallback-data', got %v", res)
	}

	events := coord.EventHistory()
	if len(events) != 1 {
		t.Fatalf("expected 1 degradation event, got %d", len(events))
	}
	if !events[0].Success || events[0].FallbackProvider != "provider-secondary" {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}

func TestDegradationCoordinator_DegradedStatic(t *testing.T) {
	coord := NewDegradationCoordinator(10)
	coord.RegisterPlan(ProviderDegradationPlan{
		PrimaryID:      "provider-primary",
		Policy:         FallbackPolicyDegradedStatic,
		StaticResponse: "cached-static-fallback",
	})

	coord.RegisterAction("provider-primary", func(ctx context.Context) (any, error) {
		return nil, errors.New("fatal connection reset")
	})

	res, err := coord.Execute(context.Background(), "provider-primary")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res != "cached-static-fallback" {
		t.Fatalf("expected 'cached-static-fallback', got %v", res)
	}

	events := coord.EventHistory()
	if len(events) != 1 {
		t.Fatalf("expected 1 degradation event, got %d", len(events))
	}
	if events[0].Policy != FallbackPolicyDegradedStatic {
		t.Fatalf("expected degraded_static policy, got %v", events[0].Policy)
	}
}

func TestDegradationCoordinator_ExhaustedFallbacks(t *testing.T) {
	coord := NewDegradationCoordinator(10)
	coord.RegisterPlan(ProviderDegradationPlan{
		PrimaryID:   "provider-primary",
		FallbackIDs: []string{"provider-secondary"},
		Policy:      FallbackPolicyFailover,
	})

	coord.RegisterAction("provider-primary", func(ctx context.Context) (any, error) {
		return nil, errors.New("primary down")
	})
	coord.RegisterAction("provider-secondary", func(ctx context.Context) (any, error) {
		return nil, errors.New("secondary also down")
	})

	_, err := coord.Execute(context.Background(), "provider-primary")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrAllFallbacksFailed) {
		t.Fatalf("expected ErrAllFallbacksFailed, got %v", err)
	}

	events := coord.EventHistory()
	if len(events) != 1 {
		t.Fatalf("expected 1 failure event, got %d", len(events))
	}
}
