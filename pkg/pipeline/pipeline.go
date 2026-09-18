// Package pipeline provides a builder and runtime for the standardized data pipeline lifecycle.
// Use this package for simple sequential step execution. For complex state machine transitions and branching logic,
// see pkg/workflow. For distributed locking, leader election, and signaling, see pkg/coordination.
// See docs/architecture/data-pipeline-lifecycle.md for the canonical stage contract (INGEST → NORMALIZE → DECIDE → COMMIT → PARK/AGGREGATE/TRIGGER → FINALIZE), legend (Envelope, Outcome, idempotency key, partition key), and invariants.
package pipeline

import (
	"context"
	"errors"
	"time"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// Context carries execution-scoped data through the pipeline.
// It wraps a base context and adds idempotency and partitioning hints.
type Context struct {
	Ctx            context.Context
	IdempotencyKey string
	PartitionKey   string
	// Outcome is populated by DECIDE/COMMIT/PARK/AGGREGATE/TRIGGER/FINALIZE stages as needed.
	Outcome map[string]any
}

// StageFunc is the shape for each pipeline stage.
// The input/output contracts are intentionally generic (any) so flows can adapt without
// re-generating types; concrete pilots should wrap these with typed helpers.
type StageFunc func(*Context, any) (any, error)

// Pipeline represents a composed set of stages.
type Pipeline struct {
	logger        logging.Logger
	stages        []namedStage
	metricsConfig *MetricsConfig
	kind          string
	profile       string
}

type namedStage struct {
	name string
	fn   StageFunc
}

const (
	logMsgPipelineStageFailed    = "Pipeline stage failed"
	logMsgPipelineStageCompleted = "Pipeline stage completed"
	fieldPipelineKind            = "pipeline_kind"
	fieldStage                   = "stage"
	fieldDuration                = "duration"
)

// MetricsSink receives per-stage telemetry for observability.
// Implementations may emit to command metrics, scheduler metrics, or logs.
// Sinks that also implement [BucketingMetricsSink] receive bucket labels when [MetricsConfig.Strategy] is set.
type MetricsSink interface {
	RecordStage(ctx context.Context, kind, stage string, duration time.Duration, err error)
}

// BucketingMetricsSink extends [MetricsSink] with bucket labels from [BucketingStrategy].
type BucketingMetricsSink interface {
	MetricsSink
	RecordStageWithBuckets(ctx context.Context, kind, stage string, duration time.Duration, err error, buckets map[string]string)
}

// LoggerMetricsSink is a minimal MetricsSink that logs per-stage timing.
type LoggerMetricsSink struct {
	logger logging.Logger
}

func (l LoggerMetricsSink) RecordStage(ctx context.Context, kind, stage string, duration time.Duration, err error) {
	l.RecordStageWithBuckets(ctx, kind, stage, duration, err, nil)
}

// RecordStageWithBuckets implements [BucketingMetricsSink]. When buckets is nil or empty, logging matches legacy RecordStage fields only.
func (l LoggerMetricsSink) RecordStageWithBuckets(ctx context.Context, kind, stage string, duration time.Duration, err error, buckets map[string]string) {
	if l.logger == nil {
		return
	}
	fields := []logging.Field{
		logging.String(fieldPipelineKind, kind),
		logging.String(fieldStage, stage),
		logging.String(fieldDuration, duration.String()),
	}
	for k, v := range buckets {
		if v == emptyValue {
			continue
		}
		if k == fieldPipelineKind || k == fieldStage {
			continue
		}
		fields = append(fields, logging.String(k, v))
	}
	if err != nil {
		logging.Fluent(l.logger).Warn(logMsgPipelineStageFailed).
			WithFields(fields...).
			WithError(err).
			Log()
	} else {
		logging.Fluent(l.logger).Info(logMsgPipelineStageCompleted).
			WithFields(fields...).
			Log()
	}
}

// Builder composes a Pipeline.
type Builder struct {
	logger        logging.Logger
	metricsConfig *MetricsConfig
	kind          string
	profile       string
	stages        []namedStage
}

// NewBuilder creates a builder for a named pipeline kind (e.g. "autofix_batch").
// Per-stage metrics are disabled until you call [Builder.WithMetricsConfig] (e.g.
// [DefaultMetricsConfig] for logger-backed standard bucketing, or a noop sink with [NoopBucketing]).
// Call sites prefer that explicit form over [NewInstrumentedBuilder] for consistency.
func NewBuilder(kind string, logger logging.Logger) *Builder {
	return &Builder{
		logger:        logger,
		kind:          kind,
		metricsConfig: nil,
	}
}

// NewInstrumentedBuilder is equivalent to NewBuilder followed by
// WithMetricsConfig(DefaultMetricsConfig(logger)) (logger-backed per-stage metrics with standard bucketing).
// Prefer spelling that out at call sites so noop and instrumented pipelines share the same pattern.
func NewInstrumentedBuilder(kind string, logger logging.Logger) *Builder {
	b := NewBuilder(kind, logger)
	b.metricsConfig = DefaultMetricsConfig(logger)
	return b
}

// WithProfile annotates the pipeline with a logging profile (human, system, etc.).
func (b *Builder) WithProfile(profile string) *Builder {
	b.profile = profile
	return b
}

// WithMetricsConfig sets the full metrics contract (sink + bucketing). Nil clears metrics:
// Run will not record per-stage telemetry (short-circuit).
func (b *Builder) WithMetricsConfig(cfg *MetricsConfig) *Builder {
	b.metricsConfig = cfg
	return b
}

// WithMetrics sets only the sink and uses [NoopBucketing] (legacy compatibility).
// Prefer [WithMetricsConfig] with an explicit [BucketingStrategy] for new code.
func (b *Builder) WithMetrics(sink MetricsSink) *Builder {
	if sink == nil {
		return b
	}
	b.metricsConfig = &MetricsConfig{
		Sink:     sink,
		Strategy: NoopBucketing{},
	}
	return b
}

// AddStage appends a stage with a stable name.
func (b *Builder) AddStage(name string, fn StageFunc) *Builder {
	if fn != nil && name != emptyValue {
		b.stages = append(b.stages, namedStage{name: name, fn: fn})
	}
	return b
}

// AddRetryStage appends a stage that will be retried if it fails with the specified sentinel error.
func (b *Builder) AddRetryStage(name string, maxRetries int, backoff time.Duration, sentinel error, fn StageFunc) *Builder {
	if fn == nil || name == emptyValue {
		return b
	}
	retryFn := func(ctx *Context, payload any) (any, error) {
		var lastErr error
		var result any
		for i := 0; i <= maxRetries; i++ {
			if ctx.Ctx.Err() != nil {
				return nil, ctx.Ctx.Err()
			}
			result, lastErr = fn(ctx, payload)
			if lastErr == nil {
				return result, nil
			}
			if sentinel != nil && !errors.Is(lastErr, sentinel) {
				return nil, lastErr
			}
			if i < maxRetries {
				if r, ok := logging.TryFluentEvent(b.logger); ok {
					r.Warn("pipeline stage retrying").
						String("stage", name).
						RetryAttempt(i + 1).
						RetryMaxAttempts(maxRetries).
						WithError(lastErr).
						Log()
				}
				select {
				case <-ctx.Ctx.Done():
					return nil, ctx.Ctx.Err()
				case <-time.After(backoff):
				}
			}
		}
		return nil, lastErr
	}
	b.stages = append(b.stages, namedStage{name: name, fn: retryFn})
	return b
}

// AddTPMStage adds a stage that blocks execution until the required tokens are available
// from the provided circuitbreaker.TPMRateLimiter, ensuring exact Tokens Per Minute (TPM) tracking.
func (b *Builder) AddTPMStage(name string, limiter *circuitbreaker.TPMRateLimiter, tokenEstimator func(any) int) *Builder {
	if limiter == nil || name == emptyValue {
		return b
	}
	tpmFn := func(ctx *Context, payload any) (any, error) {
		tokens := 1
		if tokenEstimator != nil {
			tokens = tokenEstimator(payload)
		}
		if err := limiter.Wait(ctx.Ctx, tokens); err != nil {
			return nil, err
		}
		return payload, nil
	}
	b.stages = append(b.stages, namedStage{name: name, fn: tpmFn})
	return b
}

// AddIdlePreventionStage appends a stage wrapped with IdlePreventionMiddleware to monitor and prevent agent idle stalling.
func (b *Builder) AddIdlePreventionStage(name, agentID string, opts IdlePreventionOptions, fn StageFunc) *Builder {
	if fn == nil || name == emptyValue {
		return b
	}
	wrappedFn := IdlePreventionMiddleware(agentID, opts, fn)
	b.stages = append(b.stages, namedStage{name: name, fn: wrappedFn})
	return b
}

// Build constructs the Pipeline. When [MetricsConfig] resolves to a non-nil sink, a reserved
// [StageMetricsResolve] stage is prepended.
func (b *Builder) Build() *Pipeline {
	cfg := resolveMetricsConfig(b.metricsConfig)
	stages := b.stages
	if cfg != nil {
		stages = prependMetricsResolveStage(stages, b.kind, cfg)
	}
	return &Pipeline{
		logger:        b.logger,
		stages:        stages,
		metricsConfig: cfg,
		kind:          b.kind,
		profile:       b.profile,
	}
}

// Run executes the pipeline over the provided payload.
// Stages run in the order they were added; the output of each stage becomes the input
// to the next stage.
func (p *Pipeline) Run(ctx *Context, payload any) (any, error) {
	if ctx == nil {
		ctx = &Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	} else if ctx.Ctx == nil {
		ctx.Ctx = pkgctx.NewSystemContext()
	}
	current := payload
	for _, st := range p.stages {
		if ctx.Ctx.Err() != nil {
			return current, ctx.Ctx.Err()
		}
		start := time.Now()
		next, err := st.fn(ctx, current)
		duration := time.Since(start)
		recordPipelineStageMetrics(p.metricsConfig, ctx, p.kind, st.name, duration, err)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func recordPipelineStageMetrics(cfg *MetricsConfig, pctx *Context, pipelineKind, stageName string, duration time.Duration, err error) {
	if metricsConfigSinkMissing(cfg) {
		return
	}
	if stageName == StageMetricsResolve {
		return
	}
	stdCtx := context.Background()
	if pctx != nil && pctx.Ctx != nil {
		stdCtx = pctx.Ctx
	}
	var buckets map[string]string
	if cfg.Strategy != nil {
		buckets = NormalizeBucketLabels(cfg.Strategy.Buckets(pctx, pipelineKind, stageName))
	}
	if bms, ok := cfg.Sink.(BucketingMetricsSink); ok {
		bms.RecordStageWithBuckets(stdCtx, pipelineKind, stageName, duration, err, buckets)
		return
	}
	cfg.Sink.RecordStage(stdCtx, pipelineKind, stageName, duration, err)
}
