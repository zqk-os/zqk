package eval

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// CRIT-1794260609098143000-359d0511: Static Floor Invariant
// Verifies configuration constraints, required fields, and input validation.
func TestCycle68_BatchRunner_StaticFloorInvariants(t *testing.T) {
	t.Parallel()

	t.Run("empty scenarios rejected", func(t *testing.T) {
		cfg := BatchConfig{
			Concurrency: 4,
			MaxTokens:   5000,
		}
		_, err := NewBatchRunner(cfg, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrZeroScenarios)
	})

	t.Run("invalid concurrency rejected", func(t *testing.T) {
		cfg := BatchConfig{
			Scenarios: []Scenario{
				{ID: "SC-001", Name: "Basic scenario"},
			},
			Concurrency: 0,
			MaxTokens:   5000,
		}
		_, err := NewBatchRunner(cfg, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidConcurrency)
	})

	t.Run("invalid token budget rejected", func(t *testing.T) {
		cfg := BatchConfig{
			Scenarios: []Scenario{
				{ID: "SC-001", Name: "Basic scenario"},
			},
			Concurrency: 2,
			MaxTokens:   0,
		}
		_, err := NewBatchRunner(cfg, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidTokenBudget)
	})

	t.Run("missing scenario ID rejected", func(t *testing.T) {
		cfg := BatchConfig{
			Scenarios: []Scenario{
				{ID: "", Name: "Missing ID"},
			},
			Concurrency: 2,
			MaxTokens:   1000,
		}
		_, err := NewBatchRunner(cfg, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	t.Run("valid configuration passes", func(t *testing.T) {
		cfg := BatchConfig{
			Scenarios: []Scenario{
				{ID: "SC-001", Name: "Scenario 1", InputTokens: 50},
				{ID: "SC-002", Name: "Scenario 2", InputTokens: 100},
			},
			Concurrency: 2,
			MaxTokens:   10000,
			Timeout:     5 * time.Second,
		}
		runner, err := NewBatchRunner(cfg, nil)
		require.NoError(t, err)
		assert.NotNil(t, runner)
	})
}

// CRIT-1794260609098144000-8c87fd46: Operational Dynamic Proof
// Verifies concurrent batch execution, thread-safe token aggregation, and SSE event streaming.
func TestCycle68_BatchRunner_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	scenarios := []Scenario{
		{ID: "SC-001", Name: "Scenario 1", InputTokens: 100},
		{ID: "SC-002", Name: "Scenario 2", InputTokens: 200},
		{ID: "SC-003", Name: "Scenario 3", InputTokens: 300},
		{ID: "SC-004", Name: "Scenario 4", InputTokens: 400},
	}

	cfg := BatchConfig{
		Scenarios:   scenarios,
		Concurrency: 2,
		MaxTokens:   50000,
		Timeout:     5 * time.Second,
	}

	var activeWorkers int64
	var maxConcurrency int64

	mockExecutor := func(ctx context.Context, sc Scenario) (int64, int64, error) {
		current := atomic.AddInt64(&activeWorkers, 1)
		defer atomic.AddInt64(&activeWorkers, -1)

		for {
			max := atomic.LoadInt64(&maxConcurrency)
			if current <= max || atomic.CompareAndSwapInt64(&maxConcurrency, max, current) {
				break
			}
		}

		time.Sleep(20 * time.Millisecond)
		return int64(sc.InputTokens), 50, nil
	}

	runner, err := NewBatchRunner(cfg, mockExecutor)
	require.NoError(t, err)

	stream := make(chan StreamEvent, 20)
	eventsCollected := make([]StreamEvent, 0)
	var streamWg sync.WaitGroup
	streamWg.Add(1)
	goroutinelabels.NewGoroutine("test_stream_drainer", "collect events in test").
		StartSimple(func() {
			defer streamWg.Done()
			for ev := range stream {
				eventsCollected = append(eventsCollected, ev)
			}
		})

	res, err := runner.Run(context.Background(), stream)
	close(stream)
	streamWg.Wait()

	require.NoError(t, err)
	assert.Equal(t, int64(4), res.TotalScenarios)
	assert.Equal(t, int64(4), res.Successful)
	assert.Equal(t, int64(0), res.Failed)
	assert.Equal(t, int64(100+200+300+400+(50*4)), res.TotalTokens)
	assert.True(t, maxConcurrency <= 2, "concurrency exceeded configured limit: %d", maxConcurrency)

	// Verify event stream structure
	require.NotEmpty(t, eventsCollected)
	assert.Equal(t, EventBatchStarted, eventsCollected[0].Type)
	assert.Equal(t, EventBatchCompleted, eventsCollected[len(eventsCollected)-1].Type)

	progressCount := 0
	completedCount := 0
	for _, ev := range eventsCollected {
		if ev.Type == EventScenarioProgress {
			progressCount++
		}
		if ev.Type == EventScenarioCompleted {
			completedCount++
		}
	}
	assert.Equal(t, 4, progressCount)
	assert.Equal(t, 4, completedCount)
}

// CRIT-1794260609098145000-c1bb556d: Negative Adversarial Boundary
// Verifies budget exhaustion, context timeouts, and isolated failure handling.
func TestCycle68_BatchRunner_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	t.Run("token budget exhaustion halts execution", func(t *testing.T) {
		scenarios := []Scenario{
			{ID: "SC-001", Name: "Scenario 1", InputTokens: 500},
			{ID: "SC-002", Name: "Scenario 2", InputTokens: 600},
			{ID: "SC-003", Name: "Scenario 3", InputTokens: 700},
		}

		cfg := BatchConfig{
			Scenarios:   scenarios,
			Concurrency: 1, // Sequential to test budget cap cleanly
			MaxTokens:   800,
		}

		runner, err := NewBatchRunner(cfg, nil)
		require.NoError(t, err)

		res, err := runner.Run(context.Background(), nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTokenBudgetExceeded)
		assert.True(t, res.TotalTokens >= 800)
		assert.True(t, res.Successful < 3, "should not have completed all scenarios")
	})

	t.Run("context cancellation halts gracefully", func(t *testing.T) {
		scenarios := []Scenario{
			{ID: "SC-001", Name: "Scenario 1", InputTokens: 100},
			{ID: "SC-002", Name: "Scenario 2", InputTokens: 200},
		}

		cfg := BatchConfig{
			Scenarios:   scenarios,
			Concurrency: 1,
			MaxTokens:   5000,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // pre-cancel context

		runner, err := NewBatchRunner(cfg, nil)
		require.NoError(t, err)

		_, err = runner.Run(ctx, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("scenario error containment", func(t *testing.T) {
		scenarios := []Scenario{
			{ID: "SC-OK", Name: "Success", InputTokens: 100},
			{ID: "SC-FAIL", Name: "Failure", InputTokens: 100},
		}

		cfg := BatchConfig{
			Scenarios:   scenarios,
			Concurrency: 1,
			MaxTokens:   5000,
		}

		failingExecutor := func(ctx context.Context, sc Scenario) (int64, int64, error) {
			if sc.ID == "SC-FAIL" {
				return 50, 0, errors.New("model inference rate limited")
			}
			return 100, 50, nil
		}

		runner, err := NewBatchRunner(cfg, failingExecutor)
		require.NoError(t, err)

		stream := make(chan StreamEvent, 10)
		events := make([]StreamEvent, 0)
		done := make(chan struct{})
		goroutinelabels.NewGoroutine("test_error_stream_drainer", "collect events for failure containment test").
			StartSimple(func() {
				for ev := range stream {
					events = append(events, ev)
				}
				close(done)
			})

		res, err := runner.Run(context.Background(), stream)
		close(stream)
		<-done

		require.NoError(t, err)
		assert.Equal(t, int64(1), res.Successful)
		assert.Equal(t, int64(1), res.Failed)

		var failedEventFound bool
		for _, ev := range events {
			if ev.Type == EventScenarioFailed && ev.ScenarioID == "SC-FAIL" {
				failedEventFound = true
				assert.Contains(t, ev.Error, "model inference rate limited")
			}
		}
		assert.True(t, failedEventFound, "must emit EventScenarioFailed for errored scenario")
	})
}
