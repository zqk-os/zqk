package pipeline

import (
	"github.com/zqk-os/zqk/pkg/logging"
)

const (
	bucketPipelineKind = "pipeline_kind"
	bucketStage        = "stage"
	bucketPartitionKey = "partition_key"
	bucketIdempotency  = "idempotency_key"
	maxIdempotencyLen  = 128
)

// MetricsConfig is the pipeline-level metrics contract: a sink plus a bucketing strategy that
// breaks down per-stage telemetry (partition key, idempotency hints, etc.). When nil on the
// built Pipeline, Run short-circuits and does not emit per-stage metrics.
//
// If Sink is non-nil and Strategy is nil, Build resolves Strategy to [StandardPipelineBucketing].
type MetricsConfig struct {
	Sink     MetricsSink
	Strategy BucketingStrategy
}

func metricsConfigSinkMissing(cfg *MetricsConfig) bool {
	return cfg == nil || cfg.Sink == nil
}

func metricsConfigStrategyMissing(cfg *MetricsConfig) bool {
	return cfg == nil || cfg.Strategy == nil
}

// BucketingStrategy produces bounded label dimensions for each stage record from the pipeline
// context and stable names. Implementations should avoid high-cardinality values (full paths,
// raw IDs) unless truncated or hashed.
type BucketingStrategy interface {
	Buckets(pctx *Context, pipelineKind, stageName string) map[string]string
}

// StandardPipelineBucketing adds pipeline_kind, stage, and optional partition_key / idempotency_key
// from [Context] when set.
type StandardPipelineBucketing struct{}

// Buckets implements [BucketingStrategy].
func (StandardPipelineBucketing) Buckets(pctx *Context, pipelineKind, stageName string) map[string]string {
	m := map[string]string{
		bucketPipelineKind: pipelineKind,
		bucketStage:        stageName,
	}
	if pctx != nil {
		if pctx.PartitionKey != emptyValue {
			m[bucketPartitionKey] = pctx.PartitionKey
		}
		if pctx.IdempotencyKey != emptyValue {
			m[bucketIdempotency] = truncateMetricLabel(pctx.IdempotencyKey, maxIdempotencyLen)
		}
	}
	return m
}

// NoopBucketing returns no extra bucket dimensions (RecordStageWithBuckets receives nil buckets).
type NoopBucketing struct{}

// Buckets implements [BucketingStrategy].
func (NoopBucketing) Buckets(*Context, string, string) map[string]string {
	return nil
}

func truncateMetricLabel(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

// DefaultMetricsConfig returns logger-backed per-stage metrics with [StandardPipelineBucketing].
func DefaultMetricsConfig(logger logging.Logger) *MetricsConfig {
	return &MetricsConfig{
		Sink:     LoggerMetricsSink{logger: logger},
		Strategy: StandardPipelineBucketing{},
	}
}

// DefaultMetricsConfigWithStrategy returns logger-backed per-stage metrics with a custom [BucketingStrategy].
// Use this when a pipeline needs extra low-cardinality labels (e.g. attention_mode) without forking [LoggerMetricsSink].
// If strategy is nil, it behaves like [DefaultMetricsConfig].
func DefaultMetricsConfigWithStrategy(logger logging.Logger, strategy BucketingStrategy) *MetricsConfig {
	if strategy == nil {
		return DefaultMetricsConfig(logger)
	}
	return &MetricsConfig{
		Sink:     LoggerMetricsSink{logger: logger},
		Strategy: strategy,
	}
}

func resolveMetricsConfig(cfg *MetricsConfig) *MetricsConfig {
	if metricsConfigSinkMissing(cfg) {
		return nil
	}
	out := *cfg
	if out.Strategy == nil {
		out.Strategy = StandardPipelineBucketing{}
	}
	return &out
}
