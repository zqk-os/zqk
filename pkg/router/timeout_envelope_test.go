package router

import (
	"context"
	"testing"
	"time"
)

func TestTimeoutEnvelope_BaseTimeouts(t *testing.T) {
	t.Parallel()

	env := NewTimeoutEnvelope()

	// Base defaults without request parameters
	fastDur := env.ComputeTimeout(TierFast, nil)
	stdDur := env.ComputeTimeout(TierStandard, nil)
	heavyDur := env.ComputeTimeout(TierHeavy, nil)
	localDur := env.ComputeTimeout(TierLocal, nil)

	if fastDur != DefaultFastTimeout+DefaultTimeoutPadding {
		t.Errorf("fastDur = %v, expected %v", fastDur, DefaultFastTimeout+DefaultTimeoutPadding)
	}
	if stdDur != DefaultStandardTimeout+DefaultTimeoutPadding {
		t.Errorf("stdDur = %v, expected %v", stdDur, DefaultStandardTimeout+DefaultTimeoutPadding)
	}
	if heavyDur != DefaultHeavyTimeout+DefaultTimeoutPadding {
		t.Errorf("heavyDur = %v, expected %v", heavyDur, DefaultHeavyTimeout+DefaultTimeoutPadding)
	}
	if localDur != DefaultLocalTimeout+DefaultTimeoutPadding {
		t.Errorf("localDur = %v, expected %v", localDur, DefaultLocalTimeout+DefaultTimeoutPadding)
	}
}

func TestTimeoutEnvelope_AdaptiveHeadroom(t *testing.T) {
	t.Parallel()

	env := NewTimeoutEnvelope().
		WithBaseTimeout(TierFast, 10*time.Second).
		WithTokenHeadroom(10*time.Millisecond).
		WithByteHeadroom(1*time.Microsecond).
		WithBounds(1*time.Second, 60*time.Second)

	req := &ModelRequest{
		Model:     "test-fast",
		Prompt:    "hello world prompt with some length",
		MaxTokens: 500, // 500 * 10ms = 5000ms = 5s
	}

	dur := env.ComputeTimeout(TierFast, req)
	expectedMin := 10*time.Second + 5*time.Second + DefaultTimeoutPadding
	if dur < expectedMin {
		t.Errorf("computed timeout %v is less than expected minimum %v", dur, expectedMin)
	}
}

func TestTimeoutEnvelope_BoundsClamping(t *testing.T) {
	t.Parallel()

	const minLimit = 5 * time.Second
	const maxLimit = 15 * time.Second

	env := NewTimeoutEnvelope().
		WithBounds(minLimit, maxLimit)

	// Extremely short base timeout should be clamped to minLimit
	env.WithBaseTimeout(TierFast, 100*time.Millisecond)
	fastDur := env.ComputeTimeout(TierFast, nil)
	if fastDur < minLimit {
		t.Errorf("expected clamped to min %v, got %v", minLimit, fastDur)
	}

	// Extremely long request should be clamped to maxLimit
	reqHuge := &ModelRequest{
		Model:     "heavy-model",
		MaxTokens: 1000000,
	}
	heavyDur := env.ComputeTimeout(TierHeavy, reqHuge)
	if heavyDur > maxLimit {
		t.Errorf("expected clamped to max %v, got %v", maxLimit, heavyDur)
	}
}

func TestTimeoutEnvelope_WithTimeoutContext(t *testing.T) {
	t.Parallel()

	env := NewTimeoutEnvelope().
		WithBaseTimeout(TierFast, 100*time.Millisecond).
		WithBounds(50*time.Millisecond, 2*time.Second)

	ctx := context.Background()
	req := &ModelRequest{Model: "fast-model"}

	subCtx, cancel, dur := env.WithTimeoutContext(ctx, TierFast, req)
	defer cancel()

	deadline, hasDeadline := subCtx.Deadline()
	if !hasDeadline {
		t.Fatalf("expected context to have deadline")
	}

	timeRemaining := time.Until(deadline)
	if timeRemaining <= 0 || timeRemaining > dur+100*time.Millisecond {
		t.Errorf("unexpected time remaining %v for duration %v", timeRemaining, dur)
	}
}
