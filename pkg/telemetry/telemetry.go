package telemetry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
)

// Tracker provides hooks to record execution telemetry and metrics.
type Tracker interface {
	RecordExecution(ctx context.Context, tool string, args []string, duration time.Duration, exitCode int, err error)
	RecordRateLimitExceeded(ctx context.Context, tool string)
	RecordCacheHit(ctx context.Context, cacheName string)
	RecordCacheMiss(ctx context.Context, cacheName string)
	RecordIPCLatency(ctx context.Context, operation string, latency time.Duration)
	RecordAgentTokenUsage(ctx context.Context, agentName string, inputTokens, outputTokens int64)
	RecordGhostDriftMTTR(ctx context.Context, driftType string, mttr time.Duration)
	RecordPersonaSkillInvocation(ctx context.Context, personaID string, skillID string, latency time.Duration, success bool)
}

// DefaultTracker implements Tracker using ZQK's structured logging.
type DefaultTracker struct {
	logger        logging.Logger
	mu            sync.Mutex
	ipcHistograms map[string]*metrics.PrometheusHistogram
}

// NewTracker creates a new telemetry tracker.
func NewTracker(logger logging.Logger) *DefaultTracker {
	return &DefaultTracker{
		logger:        logger,
		ipcHistograms: make(map[string]*metrics.PrometheusHistogram),
	}
}

// RecordExecution logs execution details to the Knowledge Kernel.
func (t *DefaultTracker) RecordExecution(ctx context.Context, tool string, args []string, duration time.Duration, exitCode int, err error) {
	entry := logging.Fluent(t.logger).Info("Recorded CLI execution telemetry").
		String("event_type", "osmosis_execution").
		String("tool", tool).
		String("duration", duration.String()).
		Int("exit_code", exitCode).
		String("execution_id", fmt.Sprintf("%d", time.Now().UnixNano()))

	if err != nil {
		entry = entry.String("error", err.Error())
	}

	entry.Log()
}

// RecordRateLimitExceeded logs when rate limits are hit.
func (t *DefaultTracker) RecordRateLimitExceeded(ctx context.Context, tool string) {
	logging.Fluent(t.logger).Warn("Rate limit exceeded for tool").
		String("event_type", "rate_limit_exceeded").
		String("tool", tool).
		Log()
}

// RecordCacheHit logs a successful cache hit.
func (t *DefaultTracker) RecordCacheHit(ctx context.Context, cacheName string) {
	logging.Fluent(t.logger).Info("Recorded cache hit").
		String("event_type", "cache_hit").
		String("cache_name", cacheName).
		Log()
}

// RecordCacheMiss logs a cache miss.
func (t *DefaultTracker) RecordCacheMiss(ctx context.Context, cacheName string) {
	logging.Fluent(t.logger).Info("Recorded cache miss").
		String("event_type", "cache_miss").
		String("cache_name", cacheName).
		Log()
}

// RecordIPCLatency records the latency of an IPC operation.
func (t *DefaultTracker) RecordIPCLatency(ctx context.Context, operation string, latency time.Duration) {
	t.mu.Lock()
	h, ok := t.ipcHistograms[operation]
	if !ok {
		// Prometheus-style histogram buckets in seconds
		buckets := []float64{0.001, 0.005, 0.010, 0.050, 0.100, 0.500, 1.0, 5.0, 10.0}
		h = metrics.NewPrometheusHistogram(buckets)
		t.ipcHistograms[operation] = h
	}
	t.mu.Unlock()

	h.Observe(latency.Seconds())
}

// RecordAgentTokenUsage logs token usage for an agent prompt.
func (t *DefaultTracker) RecordAgentTokenUsage(ctx context.Context, agentName string, inputTokens, outputTokens int64) {
	logging.Fluent(t.logger).Info("Recorded agent token usage").
		String("event_type", "agent_token_usage").
		String("agent_name", agentName).
		Int("input_tokens", int(inputTokens)).
		Int("output_tokens", int(outputTokens)).
		Log()
}

// RecordGhostDriftMTTR logs the Mean Time To Recovery for a Ghost Drift metric.
func (t *DefaultTracker) RecordGhostDriftMTTR(ctx context.Context, driftType string, mttr time.Duration) {
	logging.Fluent(t.logger).Info("Recorded ghost drift MTTR").
		String("event_type", "ghost_drift_mttr").
		String("drift_type", driftType).
		String("mttr", mttr.String()).
		Log()
}

// RecordPersonaSkillInvocation logs metrics when a persona uses a skill.
func (t *DefaultTracker) RecordPersonaSkillInvocation(ctx context.Context, personaID string, skillID string, latency time.Duration, success bool) {
	t.mu.Lock()
	operation := fmt.Sprintf("persona_%s_skill_%s", personaID, skillID)
	h, ok := t.ipcHistograms[operation]
	if !ok {
		buckets := []float64{0.1, 0.5, 1.0, 5.0, 10.0, 30.0, 60.0}
		h = metrics.NewPrometheusHistogram(buckets)
		t.ipcHistograms[operation] = h
	}
	t.mu.Unlock()

	h.Observe(latency.Seconds())

	entry := logging.Fluent(t.logger).Info("Recorded persona skill invocation telemetry").
		String("event_type", "persona_skill_invocation").
		String("persona_id", personaID).
		String("skill_id", skillID).
		String("latency", latency.String()).
		Bool("success", success)

	entry.Log()
}
