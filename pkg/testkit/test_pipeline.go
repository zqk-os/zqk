package testkit

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

const testPipelineKindPrefix = "test."

type noopTestPipelineMetrics struct{}

func (noopTestPipelineMetrics) RecordStage(context.Context, string, string, time.Duration, error) {}

func (noopTestPipelineMetrics) RecordStageWithBuckets(context.Context, string, string, time.Duration, error, map[string]string) {
}

// TestStage defines one stage for [RunTestPipeline].
type TestStage struct {
	Name string
	Fn   pipeline.StageFunc
}

// NamedTestStep is a simple named step for linear test setup/cleanup flows.
type NamedTestStep struct {
	Name string
	Fn   func() error
}

// NewTestPipelineBuilder returns a test-focused pipeline builder with:
// - "test." kind prefix (for clear observability separation)
// - discard logger
// - no-op metrics sink/bucketing
// - test profile
func NewTestPipelineBuilder(kind string) *pipeline.Builder {
	normalizedKind := strings.TrimSpace(kind)
	if normalizedKind == emptyValue {
		normalizedKind = "unnamed"
	}
	if !strings.HasPrefix(normalizedKind, testPipelineKindPrefix) {
		normalizedKind = testPipelineKindPrefix + normalizedKind
	}

	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(context.Background()))
	return pipeline.NewBuilder(normalizedKind, logger).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopTestPipelineMetrics{}, Strategy: pipeline.NoopBucketing{}}).
		WithProfile("test")
}

// RunTestPipeline executes test stages with standard test-focused defaults.
// This keeps tests to a one-liner setup instead of repeating logger/metrics boilerplate.
func RunTestPipeline(ctx context.Context, kind string, payload any, stages ...TestStage) (any, error) {
	b := NewTestPipelineBuilder(kind)
	for _, s := range stages {
		b.AddStage(s.Name, s.Fn)
	}
	pl := b.Build()

	runCtx := ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	return pl.Run(&pipeline.Context{Ctx: runCtx, Outcome: make(map[string]any)}, payload)
}

// RunNamedTestSteps runs named no-payload steps in order through the standard test pipeline.
// Useful for one-liner setup/cleanup flows shared across test files.
func RunNamedTestSteps(ctx context.Context, kind string, steps ...NamedTestStep) error {
	stages := make([]TestStage, 0, len(steps))
	for _, step := range steps {
		s := step
		stages = append(stages, TestStage{
			Name: s.Name,
			Fn: func(_ *pipeline.Context, payload any) (any, error) {
				if s.Fn == nil {
					return payload, nil
				}
				if err := s.Fn(); err != nil {
					return payload, err
				}
				return payload, nil
			},
		})
	}
	_, err := RunTestPipeline(ctx, kind, nil, stages...)
	return err
}
