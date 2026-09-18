package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/pipeline"
)

type recordingSink struct {
	stages []string
}

func (r *recordingSink) RecordStage(ctx context.Context, kind, stage string, duration time.Duration, err error) {
	r.RecordStageWithBuckets(ctx, kind, stage, duration, err, nil)
}

func (r *recordingSink) RecordStageWithBuckets(ctx context.Context, kind, stage string, duration time.Duration, err error, _ map[string]string) {
	r.stages = append(r.stages, stage)
}

func TestAppendStandardStorageTeardownStages_Order(t *testing.T) {
	tmp := t.TempDir()
	rec := &recordingSink{}
	opts := TeardownOptions{ProjectRoot: tmp}.WithDefaults()

	b := NewTestPipelineBuilder("testkit.order").
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: rec, Strategy: pipeline.NoopBucketing{}}).
		AddStage("ingest", func(pctx *pipeline.Context, payload any) (any, error) {
			return payload, nil
		})
	pl := AppendStandardStorageTeardownStages(b, opts).Build()

	pctx := &pipeline.Context{Ctx: context.Background(), Outcome: make(map[string]any)}
	if _, err := pl.Run(pctx, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{
		"ingest",
		"GLOBAL_LISTING_INDEX_QUEUE_FLUSH",
		"FLUSH_LISTING_INDEX",
		"LISTING_INDEX_QUEUE_SHUTDOWN",
		"WAIT_WAL",
		"FILE_STORAGE_SHUTDOWN",
		"ORPHAN_CLEANUP_QUEUE_SHUTDOWN",
		"FLUSH_AUDIT_BUFFER",
		"TEARDOWN_GLOBAL_AUDIT_BUFFER",
		"STRIP_PROCESS_ARTIFACTS",
		"SCRUB_PROJECT_ROOT",
	}
	if len(rec.stages) != len(want) {
		t.Fatalf("stage count: got %d want %d: %v", len(rec.stages), len(want), rec.stages)
	}
	for i := range want {
		if rec.stages[i] != want[i] {
			t.Fatalf("stage %d: got %q want %q (full=%v)", i, rec.stages[i], want[i], rec.stages)
		}
	}
}

func TestTeardownOptions_DefaultWALTimeout(t *testing.T) {
	o := TeardownOptions{}.WithDefaults()
	if o.WALTimeout != 15*time.Second {
		t.Fatalf("WALTimeout: got %v", o.WALTimeout)
	}
	if o.ShutdownTimeout != 15*time.Second {
		t.Fatalf("ShutdownTimeout: got %v", o.ShutdownTimeout)
	}
}
