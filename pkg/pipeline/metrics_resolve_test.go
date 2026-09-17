package pipeline

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
)

func TestNormalizeBucketLabels(t *testing.T) {
	t.Run("nil_empty", func(t *testing.T) {
		if NormalizeBucketLabels(nil) != nil {
			t.Fatal("expected nil")
		}
		if NormalizeBucketLabels(map[string]string{}) != nil {
			t.Fatal("expected nil for empty map")
		}
	})
	t.Run("trim_and_drop_empty", func(t *testing.T) {
		got := NormalizeBucketLabels(map[string]string{
			"  k  ": " v ",
			"":      "x",
			"y":     "",
		})
		if got["k"] != "v" {
			t.Fatalf("got %v", got)
		}
		if _, ok := got[""]; ok {
			t.Fatal("empty key should be dropped")
		}
		if _, ok := got["y"]; ok {
			t.Fatal("empty value should be dropped")
		}
	})
	t.Run("max_keys", func(t *testing.T) {
		in := make(map[string]string, maxMetricBucketKeys+5)
		for i := 0; i < maxMetricBucketKeys+5; i++ {
			in[strings.Repeat("a", i+1)] = "v"
		}
		got := NormalizeBucketLabels(in)
		if len(got) != maxMetricBucketKeys {
			t.Fatalf("got %d keys, want %d", len(got), maxMetricBucketKeys)
		}
	})
	t.Run("sanitize_key", func(t *testing.T) {
		got := NormalizeBucketLabels(map[string]string{"Bad Key!": "1"})
		v, ok := got["bad_key"]
		if !ok || v != "1" {
			t.Fatalf("got %v", got)
		}
	})
}

func TestMetricsResolve_Prepend_NoMetricRecord(t *testing.T) {
	rec := &recordingSink{}
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(context.Background()))
	pl := NewBuilder("m", logger).
		WithMetricsConfig(&MetricsConfig{Sink: rec, Strategy: NoopBucketing{}}).
		AddStage("only", func(_ *Context, in any) (any, error) { return in, nil }).
		Build()
	ctx := &Context{Ctx: context.Background(), Outcome: make(map[string]any)}
	if _, err := pl.Run(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if v, ok := ctx.Outcome[OutcomeKeyMetricsResolve].(bool); !ok || !v {
		t.Fatalf("expected metrics_resolve outcome, got %+v", ctx.Outcome)
	}
	if len(rec.records) != 1 || rec.records[0].stage != "only" {
		t.Fatalf("expected single metric for only: %+v", rec.records)
	}
	for _, r := range rec.records {
		if r.stage == StageMetricsResolve {
			t.Fatalf("METRICS_RESOLVE must not emit metrics: %+v", r)
		}
	}
}
