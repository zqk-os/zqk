// Package scheduler: pipeline-based autofix batch cleanup.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

const pipelineKindAutofixBatchCleanup = "autofix_batch_cleanup"

type autofixBatchCleanupPayload struct {
	Job *ScheduledJob
}

// RunAutofixBatchCleanupViaPipeline runs autofix batch cleanup through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunAutofixBatchCleanupViaPipeline(ctx context.Context, h *AutofixBatchCleanupHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("autofix batch cleanup: handler and job required")
	}
	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	pl := pipeline.NewBuilder(pipelineKindAutofixBatchCleanup, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *autofixBatchCleanupPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *autofixBatchCleanupPayload with Job, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = in.Job.ID
			}
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*autofixBatchCleanupPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *autofixBatchCleanupPayload, got %T", payload)
			}
			err := h.executeAutofixBatchCleanupCore(pctx.Ctx, in.Job)
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
	initial := &autofixBatchCleanupPayload{Job: job}
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}
