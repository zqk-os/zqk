// Package scheduler: pipeline-based scheduler events aggregation.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.
// Second client flow migrated to the pipeline (after autofix_batch).

package scheduler

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const pipelineKindSchedulerEventsAgg = "scheduler_events_aggregation"

// schedulerEventsAggPayload carries input through INGEST → NORMALIZE → COMMIT.
type schedulerEventsAggPayload struct {
	ProjectRoot string
	EventsPath  string
	SummaryPath string
	Summary     *SchedulerMetricsSummary
	LinesRead   int
}

// schedulerEventsAggPayloadForCommit decodes a payload with a non-nil Summary for the COMMIT stage.
func schedulerEventsAggPayloadForCommit(payload any) (*schedulerEventsAggPayload, bool) {
	in, ok := nildecode.DecodeNonNilPayload[*schedulerEventsAggPayload](payload)
	if !ok {
		return nil, false
	}
	if in.Summary == nil {
		return nil, false
	}
	return in, true
}

// RunAggregationViaPipeline runs scheduler events aggregation through the standardized pipeline
// (INGEST → NORMALIZE → COMMIT → FINALIZE) for observability and lifecycle alignment.
// Returns summary, linesRead, and any error.
func RunAggregationViaPipeline(ctx context.Context, projectRoot, eventsPath, summaryPath string, logger logging.Logger) (*SchedulerMetricsSummary, int, error) {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	pl := pipeline.NewBuilder(pipelineKindSchedulerEventsAgg, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			// Payload is initial input (paths)
			in, ok := nildecode.DecodeNonNilPayload[*schedulerEventsAggPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *schedulerEventsAggPayload, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestEventsPath] = in.EventsPath
				pctx.Outcome[pipeline.OutcomeKeyIngestSummaryPath] = in.SummaryPath
			}
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*schedulerEventsAggPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *schedulerEventsAggPayload, got %T", payload)
			}
			summary, linesRead, err := AggregateEventsFromFile(in.EventsPath)
			if err != nil {
				return nil, err
			}
			summary.WindowEndISO = zqktime.NowRFC3339UTC()
			summary.LinesRead = linesRead
			in.Summary = summary
			in.LinesRead = linesRead
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyNormalizeLinesRead] = linesRead
			}
			return in, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := schedulerEventsAggPayloadForCommit(payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected payload with Summary, got %T", payload)
			}
			if err := WriteSummary(in.Summary, in.SummaryPath); err != nil {
				return nil, err
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyCommitSummaryPath] = in.SummaryPath
			}
			return in, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*schedulerEventsAggPayload](payload)
			if !ok {
				return payload, nil
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyFinalizeLinesRead] = in.LinesRead
			}
			return payload, nil
		}).
		Build()

	initial := &schedulerEventsAggPayload{
		ProjectRoot: projectRoot,
		EventsPath:  eventsPath,
		SummaryPath: summaryPath,
	}
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	if err != nil {
		return nil, 0, err
	}
	return initial.Summary, initial.LinesRead, nil
}
