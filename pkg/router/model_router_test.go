package router

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestModelRouter_RouteDirectModel(t *testing.T) {
	t.Parallel()

	router := NewModelRouter(nil)
	const modelName = "custom-gemini"

	router.RegisterModel(modelName, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return &ModelResponse{
			Content:    "direct-response",
			TokensUsed: 12,
		}, nil
	})

	resp, err := router.RouteModel(context.Background(), &ModelRequest{
		Model: modelName,
	})
	if err != nil {
		t.Fatalf("RouteModel failed: %v", err)
	}
	if resp.Content != "direct-response" {
		t.Errorf("expected Content=%q, got %q", "direct-response", resp.Content)
	}
	if resp.AttemptCount != 1 {
		t.Errorf("expected AttemptCount=1, got %d", resp.AttemptCount)
	}
}

func TestModelRouter_RouteTierHandler(t *testing.T) {
	t.Parallel()

	router := NewModelRouter(nil)
	router.RegisterTier(TierFast, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return &ModelResponse{
			Content: "fast-tier-response",
		}, nil
	})

	resp, err := router.RouteModel(context.Background(), &ModelRequest{
		Model: "gemini-2.5-flash", // classifies to TierFast
	})
	if err != nil {
		t.Fatalf("RouteModel failed: %v", err)
	}
	if resp.Content != "fast-tier-response" {
		t.Errorf("expected Content=%q, got %q", "fast-tier-response", resp.Content)
	}
	if resp.Tier != TierFast {
		t.Errorf("expected Tier=%q, got %q", TierFast, resp.Tier)
	}
}

func TestModelRouter_RouteGradientFallback(t *testing.T) {
	t.Parallel()

	router := NewModelRouter(nil)

	// Primary fast tier fails
	router.RegisterTier(TierFast, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return nil, errors.New("upstream fast provider saturated (503)")
	})

	// Fallback standard tier succeeds
	router.RegisterTier(TierStandard, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return &ModelResponse{
			Content: "standard-tier-fallback-response",
		}, nil
	})

	req := &ModelRequest{
		Model:         "flash-model",
		PreferredTier: TierFast,
		FallbackChain: []ModelTier{TierStandard, TierLocal},
	}

	resp, err := router.RouteModel(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful fallback, got error: %v", err)
	}
	if resp.Content != "standard-tier-fallback-response" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
	if resp.Tier != TierStandard {
		t.Errorf("expected Tier=%q, got %q", TierStandard, resp.Tier)
	}
	if resp.AttemptCount != 2 {
		t.Errorf("expected AttemptCount=2, got %d", resp.AttemptCount)
	}
}

func TestModelRouter_BoundaryErrors(t *testing.T) {
	t.Parallel()

	router := NewModelRouter(nil)

	// 1. Nil request
	if _, err := router.RouteModel(context.Background(), nil); !errors.Is(err, ErrNilRequest) {
		t.Errorf("expected ErrNilRequest, got %v", err)
	}

	// 2. Empty model name
	if _, err := router.RouteModel(context.Background(), &ModelRequest{Model: ""}); !errors.Is(err, ErrEmptyModel) {
		t.Errorf("expected ErrEmptyModel, got %v", err)
	}

	// 3. All tiers failed
	router.RegisterTier(TierFast, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return nil, errors.New("fast offline")
	})
	router.RegisterTier(TierStandard, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return nil, errors.New("standard offline")
	})

	_, err := router.RouteModel(context.Background(), &ModelRequest{
		Model:         "fast-model",
		PreferredTier: TierFast,
		FallbackChain: []ModelTier{TierStandard},
	})
	if !errors.Is(err, ErrAllTiersFailed) {
		t.Errorf("expected ErrAllTiersFailed, got %v", err)
	}
}

func TestModelRouter_AdaptiveTimeoutEnforcement(t *testing.T) {
	t.Parallel()

	// Short envelope for testing
	env := NewTimeoutEnvelope().
		WithBaseTimeout(TierFast, 30*time.Millisecond).
		WithBounds(10*time.Millisecond, 100*time.Millisecond)

	router := NewModelRouter(env)
	router.RegisterTier(TierFast, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return &ModelResponse{Content: "late"}, nil
		}
	})

	_, err := router.RouteModel(context.Background(), &ModelRequest{
		Model:         "fast-agent",
		PreferredTier: TierFast,
	})
	if !errors.Is(err, ErrAllTiersFailed) && !errors.Is(err, ErrModelTimeout) {
		t.Fatalf("expected timeout or all tiers failed error, got: %v", err)
	}
}
