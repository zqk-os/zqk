package system

import (
	stdcontext "context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/pipeline"

	"github.com/lanceman/zqk/pkg/objects"
)

const pipelineKindEnqueueValidationForObject = "incremental_validation_enqueue"

// Outcome keys for incremental_validation_enqueue (pipeline observability; wire shape unchanged).
const (
	enqueueValOutcomeKeyObjectID     = "object_id"
	enqueueValOutcomeKeyNormalized   = "normalized"
	enqueueValOutcomeKeyCommitted    = "committed"
	enqueueValOutcomeKeyFinalizeDone = "finalize_done"
)

// noopMetricsSink avoids per-stage Info logs for high-frequency pipeline executions.
type noopMetricsSinkEnqueueValidation struct{}

func (noopMetricsSinkEnqueueValidation) RecordStage(ctx stdcontext.Context, kind, stage string, duration time.Duration, err error) {
}

func (noopMetricsSinkEnqueueValidation) RecordStageWithBuckets(ctx stdcontext.Context, kind, stage string, duration time.Duration, err error, _ map[string]string) {
}

// RunEnqueueValidationForObjectViaPipeline wraps `enqueueValidationForObjectCore` in the canonical
// pipeline lifecycle (INGEST → NORMALIZE → COMMIT → FINALIZE).
//
// This is best-effort (mirrors original behavior): it returns nil unless pipeline construction/execution fails.
func RunEnqueueValidationForObjectViaPipeline(projectRoot, objectID, kind, filePath string) error {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue || objectID == emptyValue {
		return nil
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	pl := pipeline.NewBuilder(pipelineKindEnqueueValidationForObject, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSinkEnqueueValidation{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[enqueueValOutcomeKeyObjectID] = objectID
			pctx.Outcome[objects.FieldKeyKind] = kind
			pctx.Outcome[objects.FieldKeyFilePath] = filePath
			return payload, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			// No normalization step needed yet; keep stage for contract uniformity.
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[enqueueValOutcomeKeyNormalized] = true
			return payload, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, payload any) (any, error) {
			if err := enqueueValidationForObjectCore(projectRoot, objectID, kind, filePath); err != nil {
				// enqueueValidationForObjectCore is best-effort, but if it ever returns non-nil error,
				// propagate so the pipeline contract can surface it.
				return nil, errfmt.Newf("enqueue_validation core failed").Wrap(err)
			}
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[enqueueValOutcomeKeyCommitted] = true
			return payload, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[enqueueValOutcomeKeyFinalizeDone] = true
			return payload, nil
		}).
		Build()

	pctx := &pipeline.Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, struct{}{})
	return err
}
