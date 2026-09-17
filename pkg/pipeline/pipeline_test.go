package pipeline

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/circuitbreaker"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

func TestEnvelope_Fields(t *testing.T) {
	now := time.Now()
	env := &Envelope{
		TraceID:        "trace-1",
		Source:         "scheduler",
		ReceivedAt:     now,
		IdempotencyKey: "autofix_batch:/repo:AUTOFIX-abc.json",
		PartitionKey:   "/repo",
		Payload:        []byte("raw"),
	}
	if env.TraceID != "trace-1" || env.Source != "scheduler" || env.IdempotencyKey == emptyValue || env.PartitionKey != "/repo" {
		t.Fatalf("envelope fields not set correctly: %+v", env)
	}
	if env.ReceivedAt != now {
		t.Fatalf("ReceivedAt: got %v", env.ReceivedAt)
	}
	if p, ok := env.Payload.([]byte); !ok || string(p) != "raw" {
		t.Fatalf("Payload: got %v", env.Payload)
	}
}

type recordingSink struct {
	records []record
}

type record struct {
	kind     string
	stage    string
	duration time.Duration
	err      error
	buckets  map[string]string
}

func (r *recordingSink) RecordStage(ctx context.Context, kind, stage string, duration time.Duration, err error) {
	r.RecordStageWithBuckets(ctx, kind, stage, duration, err, nil)
}

func (r *recordingSink) RecordStageWithBuckets(ctx context.Context, kind, stage string, duration time.Duration, err error, buckets map[string]string) {
	r.records = append(r.records, record{
		kind:     kind,
		stage:    stage,
		duration: duration,
		err:      err,
		buckets:  buckets,
	})
}

func TestPipeline_Run_SucceedsAndOrdersStages(t *testing.T) {
	rec := &recordingSink{}
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))

	pl := NewBuilder("test_flow", logger).
		WithMetricsConfig(&MetricsConfig{Sink: rec, Strategy: NoopBucketing{}}).
		AddStage("ingest", func(pctx *Context, payload any) (any, error) {
			if payload.(int) != 1 {
				t.Fatalf("expected initial payload 1, got %v", payload)
			}
			return 2, nil
		}).
		AddStage("normalize", func(pctx *Context, payload any) (any, error) {
			if payload.(int) != 2 {
				t.Fatalf("expected payload 2, got %v", payload)
			}
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[OutcomeKeyNormalized] = true
			return 3, nil
		}).
		Build()

	ctx := &Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	out, err := pl.Run(ctx, 1)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if out.(int) != 3 {
		t.Fatalf("expected output 3, got %v", out)
	}
	if v, ok := ctx.Outcome[OutcomeKeyNormalized].(bool); !ok || !v {
		t.Fatalf("expected Outcome[\"normalized\"] to be true, got %v", ctx.Outcome[OutcomeKeyNormalized])
	}
	if v, ok := ctx.Outcome[OutcomeKeyMetricsResolve].(bool); !ok || !v {
		t.Fatalf("expected Outcome[\"metrics_resolve\"] from METRICS_RESOLVE preflight, got %v", ctx.Outcome[OutcomeKeyMetricsResolve])
	}
	if len(rec.records) != 2 {
		t.Fatalf("expected 2 metric records, got %d", len(rec.records))
	}
	if rec.records[0].stage != "ingest" || rec.records[1].stage != "normalize" {
		t.Fatalf("unexpected stage order: %+v", rec.records)
	}
	for _, r := range rec.records {
		if r.kind != "test_flow" {
			t.Fatalf("expected pipeline kind test_flow for stage %s, got %q", r.stage, r.kind)
		}
		if r.duration < 0 {
			t.Fatalf("expected non-negative duration for stage %s, got %v", r.stage, r.duration)
		}
		if r.err != nil {
			t.Fatalf("expected nil error for stage %s, got %v", r.stage, r.err)
		}
	}
}

func TestPipeline_Run_StopsOnError(t *testing.T) {
	rec := &recordingSink{}
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	sentinel := errors.New("sentinel")

	pl := NewBuilder("test_flow", logger).
		WithMetricsConfig(&MetricsConfig{Sink: rec, Strategy: NoopBucketing{}}).
		AddStage("ingest", func(pctx *Context, payload any) (any, error) {
			return payload, nil
		}).
		AddStage("decide", func(pctx *Context, payload any) (any, error) {
			return nil, sentinel
		}).
		AddStage("commit", func(pctx *Context, payload any) (any, error) {
			t.Fatalf("commit stage should not be executed after error")
			return payload, nil
		}).
		Build()

	ctx := &Context{Ctx: pkgctx.NewSystemContext()}
	out, err := pl.Run(ctx, "payload")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output on error, got %v", out)
	}
	if v, ok := ctx.Outcome[OutcomeKeyMetricsResolve].(bool); !ok || !v {
		t.Fatalf("expected Outcome[\"metrics_resolve\"] from METRICS_RESOLVE preflight, got %v", ctx.Outcome[OutcomeKeyMetricsResolve])
	}
	if len(rec.records) != 2 {
		t.Fatalf("expected 2 metric records (including error), got %d", len(rec.records))
	}
	if rec.records[1].stage != "decide" || !errors.Is(rec.records[1].err, sentinel) {
		t.Fatalf("unexpected error record: %+v", rec.records[1])
	}
}

func TestNewBuilder_KindAndStages(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	b := NewBuilder("autofix_batch", logger)
	if b == nil {
		t.Fatal("NewBuilder returned nil")
	}
	pl := b.AddStage("INGEST", func(_ *Context, in any) (any, error) { return in, nil }).Build()
	if pl == nil {
		t.Fatal("Build returned nil")
	}
	ctx := &Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	out, err := pl.Run(ctx, "x")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "x" {
		t.Fatalf("expected x, got %v", out)
	}
}

func TestPipeline_Run_ShortCircuitsMetricsWithoutMetricsConfig(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	pl := NewBuilder("no_metrics", logger).
		AddStage("only", func(_ *Context, in any) (any, error) { return in, nil }).
		Build()
	ctx := &Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	out, err := pl.Run(ctx, 1)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != 1 {
		t.Fatalf("expected payload 1, got %v", out)
	}
}

func TestPipeline_BucketingStrategyPassesBuckets(t *testing.T) {
	rec := &recordingSink{}
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	pl := NewBuilder("bucketed", logger).
		WithMetricsConfig(&MetricsConfig{
			Sink:     rec,
			Strategy: StandardPipelineBucketing{},
		}).
		AddStage("ingest", func(_ *Context, in any) (any, error) { return in, nil }).
		Build()
	ctx := &Context{
		Ctx:            pkgctx.NewSystemContext(),
		PartitionKey:   "part-1",
		IdempotencyKey: "idem-abc",
		Outcome:        make(map[string]any),
	}
	if _, err := pl.Run(ctx, 1); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := ctx.Outcome[OutcomeKeyMetricsResolve].(bool); !ok || !v {
		t.Fatalf("expected Outcome[\"metrics_resolve\"] from METRICS_RESOLVE preflight, got %v", ctx.Outcome[OutcomeKeyMetricsResolve])
	}
	if len(rec.records) != 1 {
		t.Fatalf("expected 1 metric record, got %d", len(rec.records))
	}
	b := rec.records[0].buckets
	if b["pipeline_kind"] != "bucketed" || b["stage"] != "ingest" {
		t.Fatalf("unexpected buckets: %v", b)
	}
	if b["partition_key"] != "part-1" {
		t.Fatalf("partition_key: got %q", b["partition_key"])
	}
	if b["idempotency_key"] != "idem-abc" {
		t.Fatalf("idempotency_key: got %q", b["idempotency_key"])
	}
}

func TestDefaultMetricsConfigWithStrategy(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	nilStrat := DefaultMetricsConfigWithStrategy(logger, nil)
	if nilStrat == nil || nilStrat.Sink == nil {
		t.Fatal("expected sink")
	}
	if _, ok := nilStrat.Strategy.(StandardPipelineBucketing); !ok {
		t.Fatalf("nil strategy: expected StandardPipelineBucketing, got %T", nilStrat.Strategy)
	}
	custom := DefaultMetricsConfigWithStrategy(logger, NoopBucketing{})
	if _, ok := custom.Strategy.(NoopBucketing); !ok {
		t.Fatalf("custom strategy: got %T", custom.Strategy)
	}
}

func TestPipeline_Run_RetriesOnSentinel(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	sentinel := errors.New("sentinel")
	attempts := 0
	pl := NewBuilder("retry_flow", logger).
		AddRetryStage("retry_stage", 2, 0, sentinel, func(pctx *Context, payload any) (any, error) {
			attempts++
			if attempts <= 2 {
				return nil, sentinel
			}
			return payload, nil
		}).
		Build()

	ctx := &Context{Ctx: pkgctx.NewSystemContext()}
	out, err := pl.Run(ctx, "payload")
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if out != "payload" {
		t.Fatalf("expected payload, got %v", out)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestPipeline_Run_RetriesOnSentinel_FailsAfterMax(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	sentinel := errors.New("sentinel")
	attempts := 0
	pl := NewBuilder("retry_flow", logger).
		AddRetryStage("retry_stage", 2, 0, sentinel, func(pctx *Context, payload any) (any, error) {
			attempts++
			return nil, sentinel
		}).
		Build()

	ctx := &Context{Ctx: pkgctx.NewSystemContext()}
	out, err := pl.Run(ctx, "payload")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output, got %v", out)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestPipeline_Run_RetriesOnSentinel_FailsImmediatelyOnNonSentinel(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	sentinel := errors.New("sentinel")
	otherErr := errors.New("other")
	attempts := 0
	pl := NewBuilder("retry_flow", logger).
		AddRetryStage("retry_stage", 2, 0, sentinel, func(pctx *Context, payload any) (any, error) {
			attempts++
			return nil, otherErr
		}).
		Build()

	ctx := &Context{Ctx: pkgctx.NewSystemContext()}
	out, err := pl.Run(ctx, "payload")
	if !errors.Is(err, otherErr) {
		t.Fatalf("expected other error, got %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output, got %v", out)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestPipeline_AddTPMStage(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	limiter := circuitbreaker.NewTPMRateLimiter(600) // 600 TPM = 10 tokens per second

	// Pre-consume tokens
	if err := limiter.Wait(pkgctx.NewSystemContext(), 600); err != nil {
		t.Fatalf("failed to wait: %v", err)
	}

	pl := NewBuilder("tpm_flow", logger).
		AddTPMStage("tpm_stage", limiter, func(p any) int {
			return p.(int)
		}).
		Build()

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 50*time.Millisecond)
	defer cancel()

	pctx := &Context{Ctx: ctx}

	// Try to consume 100 tokens, should block and then fail due to context timeout
	_, err := pl.Run(pctx, 100)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestPipeline_AddIdlePreventionStage(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))

	opts := IdlePreventionOptions{
		IdleThreshold: 10 * time.Minute,
	}

	pl := NewBuilder("idle_flow", logger).
		AddIdlePreventionStage("idle_stage", "agent-123", opts, func(pctx *Context, payload any) (any, error) {
			return payload, nil
		}).
		Build()

	if len(pl.stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(pl.stages))
	}
}
