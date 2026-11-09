package eval

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Common error invariants for batch evaluation runner.
var (
	ErrInvalidConfig       = errors.New("batch_runner: invalid configuration")
	ErrTokenBudgetExceeded = errors.New("batch_runner: token budget exceeded")
	ErrZeroScenarios       = errors.New("batch_runner: scenarios list cannot be empty")
	ErrInvalidConcurrency  = errors.New("batch_runner: concurrency must be greater than zero")
	ErrInvalidTokenBudget  = errors.New("batch_runner: max token budget must be greater than zero")
)

// StreamEventType denotes the phase of execution reported over the stream.
type StreamEventType string

const (
	EventBatchStarted      StreamEventType = "batch_started"
	EventScenarioProgress  StreamEventType = "scenario_progress"
	EventScenarioCompleted StreamEventType = "scenario_completed"
	EventScenarioFailed    StreamEventType = "scenario_failed"
	EventBatchCompleted    StreamEventType = "batch_completed"
)

// StreamEvent represents a real-time event emitted during scenario execution.
type StreamEvent struct {
	Type        StreamEventType `json:"type"`
	ScenarioID  string          `json:"scenario_id,omitempty"`
	Timestamp   time.Time       `json:"timestamp"`
	TokensUsed  int64           `json:"tokens_used"`
	TotalTokens int64           `json:"total_tokens"`
	Payload     map[string]any  `json:"payload,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// Scenario represents a single evaluation scenario to be run.
type Scenario struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Model       string         `json:"model"`
	InputTokens int            `json:"input_tokens"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// BatchConfig specifies the execution invariants and resource limits for a batch run.
type BatchConfig struct {
	Scenarios   []Scenario    `json:"scenarios"`
	Concurrency int           `json:"concurrency"`
	MaxTokens   int64         `json:"max_tokens"`
	Timeout     time.Duration `json:"timeout"`
}

// Validate checks whether the BatchConfig satisfies all static invariants.
func (c *BatchConfig) Validate() error {
	if len(c.Scenarios) == 0 {
		return ErrZeroScenarios
	}
	if c.Concurrency <= 0 {
		return ErrInvalidConcurrency
	}
	if c.MaxTokens <= 0 {
		return ErrInvalidTokenBudget
	}
	for i, sc := range c.Scenarios {
		if sc.ID == "" {
			return fmt.Errorf("%w: scenario at index %d missing id", ErrInvalidConfig, i)
		}
	}
	return nil
}

// BatchMetrics aggregates thread-safe telemetry across all scenarios in a batch run.
type BatchMetrics struct {
	TotalPromptTokens     int64
	TotalCompletionTokens int64
	TotalCachedTokens     int64
	TotalScenariosRun     int64
	TotalFailures         int64
	mu                    sync.RWMutex
}

// RecordTokens atomically increments token usage and returns current cumulative total.
func (m *BatchMetrics) RecordTokens(prompt, completion, cached int64) int64 {
	atomic.AddInt64(&m.TotalPromptTokens, prompt)
	atomic.AddInt64(&m.TotalCompletionTokens, completion)
	atomic.AddInt64(&m.TotalCachedTokens, cached)
	return atomic.LoadInt64(&m.TotalPromptTokens) + atomic.LoadInt64(&m.TotalCompletionTokens)
}

// TotalTokens returns the total cumulative tokens consumed.
func (m *BatchMetrics) TotalTokens() int64 {
	return atomic.LoadInt64(&m.TotalPromptTokens) + atomic.LoadInt64(&m.TotalCompletionTokens)
}

// ScenarioOutcome records the result of a single evaluated scenario.
type ScenarioOutcome struct {
	ScenarioID       string        `json:"scenario_id"`
	Success          bool          `json:"success"`
	PromptTokens     int64         `json:"prompt_tokens"`
	CompletionTokens int64         `json:"completion_tokens"`
	Duration         time.Duration `json:"duration"`
	Error            string        `json:"error,omitempty"`
}

// BatchResult encapsulates the full summary of a completed batch evaluation.
type BatchResult struct {
	TotalScenarios int64             `json:"total_scenarios"`
	Successful     int64             `json:"successful"`
	Failed         int64             `json:"failed"`
	TotalTokens    int64             `json:"total_tokens"`
	Duration       time.Duration     `json:"duration"`
	Outcomes       []ScenarioOutcome `json:"outcomes"`
}

// ScenarioExecutor is a mockable function executing an individual scenario.
type ScenarioExecutor func(ctx context.Context, sc Scenario) (promptTokens, completionTokens int64, err error)

// BatchRunner coordinates the concurrent execution of scenarios with real-time SSE streaming.
type BatchRunner struct {
	config   BatchConfig
	executor ScenarioExecutor
	metrics  BatchMetrics
}

// NewBatchRunner constructs and statically validates a new BatchRunner.
func NewBatchRunner(cfg BatchConfig, executor ScenarioExecutor) (*BatchRunner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if executor == nil {
		executor = DefaultScenarioExecutor
	}
	return &BatchRunner{
		config:   cfg,
		executor: executor,
	}, nil
}

// DefaultScenarioExecutor simulates an evaluation execution with synthetic token consumption.
func DefaultScenarioExecutor(ctx context.Context, sc Scenario) (int64, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}
	p := int64(sc.InputTokens)
	if p <= 0 {
		p = 100
	}
	c := int64(50)
	return p, c, nil
}

// Run executes all configured scenarios using a bounded worker pool and streams events.
func (r *BatchRunner) Run(ctx context.Context, stream chan<- StreamEvent) (*BatchResult, error) {
	if stream != nil {
		stream <- StreamEvent{
			Type:      EventBatchStarted,
			Timestamp: time.Now().UTC(),
			Payload: map[string]any{
				"total_scenarios": len(r.config.Scenarios),
				"concurrency":     r.config.Concurrency,
				"max_tokens":      r.config.MaxTokens,
			},
		}
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if r.config.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.config.Timeout)
		defer cancel()
	}

	start := time.Now()
	scenarios := r.config.Scenarios
	workCh := make(chan Scenario, len(scenarios))
	for _, sc := range scenarios {
		workCh <- sc
	}
	close(workCh)

	outcomesMu := sync.Mutex{}
	outcomes := make([]ScenarioOutcome, 0, len(scenarios))
	var budgetExceeded atomic.Bool

	var wg sync.WaitGroup
	workers := r.config.Concurrency
	if workers > len(scenarios) {
		workers = len(scenarios)
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("batch_eval_worker", "process evaluation scenarios concurrently").
			StartSimple(func() {
				defer wg.Done()
				for sc := range workCh {
					if runCtx.Err() != nil || budgetExceeded.Load() {
						return
					}

					scStart := time.Now()
					if stream != nil {
						stream <- StreamEvent{
							Type:        EventScenarioProgress,
							ScenarioID:  sc.ID,
							Timestamp:   time.Now().UTC(),
							TotalTokens: r.metrics.TotalTokens(),
						}
					}

					pTokens, cTokens, err := r.executor(runCtx, sc)
					dur := time.Since(scStart)
					totTokens := r.metrics.RecordTokens(pTokens, cTokens, 0)

					outcome := ScenarioOutcome{
						ScenarioID:       sc.ID,
						PromptTokens:     pTokens,
						CompletionTokens: cTokens,
						Duration:         dur,
					}

					if err != nil {
						atomic.AddInt64(&r.metrics.TotalFailures, 1)
						outcome.Success = false
						outcome.Error = err.Error()
						if stream != nil {
							stream <- StreamEvent{
								Type:        EventScenarioFailed,
								ScenarioID:  sc.ID,
								Timestamp:   time.Now().UTC(),
								TokensUsed:  pTokens + cTokens,
								TotalTokens: totTokens,
								Error:       err.Error(),
							}
						}
					} else {
						atomic.AddInt64(&r.metrics.TotalScenariosRun, 1)
						outcome.Success = true
						if stream != nil {
							stream <- StreamEvent{
								Type:        EventScenarioCompleted,
								ScenarioID:  sc.ID,
								Timestamp:   time.Now().UTC(),
								TokensUsed:  pTokens + cTokens,
								TotalTokens: totTokens,
							}
						}
					}

					outcomesMu.Lock()
					outcomes = append(outcomes, outcome)
					outcomesMu.Unlock()

					if totTokens >= r.config.MaxTokens {
						budgetExceeded.Store(true)
						return
					}
				}
			})
	}

	wg.Wait()
	duration := time.Since(start)

	res := &BatchResult{
		TotalScenarios: int64(len(r.config.Scenarios)),
		Successful:     atomic.LoadInt64(&r.metrics.TotalScenariosRun),
		Failed:         atomic.LoadInt64(&r.metrics.TotalFailures),
		TotalTokens:    r.metrics.TotalTokens(),
		Duration:       duration,
		Outcomes:       outcomes,
	}

	if stream != nil {
		stream <- StreamEvent{
			Type:        EventBatchCompleted,
			Timestamp:   time.Now().UTC(),
			TotalTokens: res.TotalTokens,
			Payload: map[string]any{
				"successful": res.Successful,
				"failed":     res.Failed,
				"duration":   duration.String(),
			},
		}
	}

	if budgetExceeded.Load() {
		return res, ErrTokenBudgetExceeded
	}
	if runCtx.Err() != nil {
		return res, runCtx.Err()
	}

	return res, nil
}
