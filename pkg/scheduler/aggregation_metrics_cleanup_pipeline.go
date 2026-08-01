// Package scheduler: pipeline-based aggregation metrics cleanup.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/pipeline"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

const pipelineKindAggregationMetricsCleanup = "aggregation_metrics_cleanup"

type aggregationMetricsCleanupPayload struct {
	Job *ScheduledJob
}

// RunAggregationMetricsCleanupViaPipeline runs aggregation metrics cleanup through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunAggregationMetricsCleanupViaPipeline(ctx context.Context, h *AggregationMetricsCleanupHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("aggregation metrics cleanup: handler and job required")
	}
	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	pl := pipeline.NewBuilder(pipelineKindAggregationMetricsCleanup, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *aggregationMetricsCleanupPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *aggregationMetricsCleanupPayload with Job, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = in.Job.ID
			}
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*aggregationMetricsCleanupPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *aggregationMetricsCleanupPayload, got %T", payload)
			}
			err := h.executeAggregationMetricsCleanupCore(pctx.Ctx, in.Job)
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyNormalizeDone] = true
				if err != nil {
					pctx.Outcome[pipeline.OutcomeKeyNormalizeError] = err.Error()
				}
			}
			return in, err
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyFinalizeDone] = true
			}
			return payload, nil
		}).
		Build()
	initial := &aggregationMetricsCleanupPayload{Job: job}
	ctx = storagepkg.WithCLIOperation(ctx)
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}
