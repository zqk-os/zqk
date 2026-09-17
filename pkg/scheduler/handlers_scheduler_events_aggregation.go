package scheduler

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// SchedulerEventsAggregationHandler runs aggregation of diagnostics.jsonl into scheduler-metrics-summary.json.
// It is invoked by the timer job SCH-evag (every 15 minutes). No manual "events aggregate" command is required.
type SchedulerEventsAggregationHandler struct {
	projectRoot string
	logger      logging.Logger
}

// NewSchedulerEventsAggregationHandler creates a handler that aggregates events and writes the summary file.
// NewSchedulerEventsAggregationHandler creates a new scheduler events aggregation handler
func NewSchedulerEventsAggregationHandler(projectRoot string, logger logging.Logger) JobHandler {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &SchedulerEventsAggregationHandler{
		projectRoot: projectRoot,
		logger:      logger,
	}
}

// Execute reads .zqk/scheduler/diagnostics.jsonl, aggregates job execution events, and writes scheduler-metrics-summary.json.
// Uses the standardized pipeline (INGEST → NORMALIZE → COMMIT → FINALIZE) per data-pipeline-lifecycle.
func (h *SchedulerEventsAggregationHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	eventsPath := EventsPath(h.projectRoot)
	summaryPath := SummaryPath(h.projectRoot)
	_, linesRead, err := RunAggregationViaPipeline(ctx, h.projectRoot, eventsPath, summaryPath, h.logger)
	if err != nil {
		return errfmt.Newf("scheduler events aggregation").Wrap(err)
	}
	SchedulerEventsAggregationLog(h.logger).Debug(LogEventSchedulerEventsAggregationAggregated).
		JobID(job.ID).
		Int("lines_read", linesRead).
		Log()
	return nil
}
