// Package scheduler: pipeline-based change journal aggregation.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.
// Third client flow migrated to the pipeline (after autofix_batch, scheduler_events_aggregation).

package scheduler

import (
	"context"
	"strconv"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/pipeline"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindChangeJournalAgg = "change_journal_aggregation"

// changeJournalAggPayload carries job config and aggregation result through INGEST → NORMALIZE → COMMIT → FINALIZE.
type changeJournalAggPayload struct {
	Job                 *ScheduledJob
	BatchSize           int
	WindowStart         time.Time
	WindowEnd           time.Time
	DeleteAfterDuration time.Duration
	Service             *storagepkg.ChangeJournalAggregationService
	SecCtx              *pkgctx.SecurityContext
	StorageCtx          *pkgctx.StorageContext
	Result              *storagepkg.ChangeJournalAggregationResult
}

// RunChangeJournalAggregationViaPipeline runs change journal aggregation through the standardized pipeline
// (INGEST → NORMALIZE → COMMIT → FINALIZE). Returns the aggregation result and any error.
// Caller is responsible for error handling (e.g. hash mismatch vs fatal).
func RunChangeJournalAggregationViaPipeline(
	ctx context.Context,
	h *ChangeJournalAggregationHandler,
	job *ScheduledJob,
) (*storagepkg.ChangeJournalAggregationResult, error) {
	if h == nil || job == nil {
		return nil, errfmt.Errorf("change journal aggregation: handler and job required")
	}
	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKindChangeJournalAgg, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *changeJournalAggPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *changeJournalAggPayload with Job, got %T", payload)
			}
			job := in.Job
			// Parse config from job
			batchSize := storagepkg.DefaultBatchSize
			if job.EnvironmentVariables != nil {
				if s, ok := job.EnvironmentVariables[EnvKeyBatchSize]; ok && s != emptyValue {
					if n, err := strconv.Atoi(s); err == nil && n > 0 {
						batchSize = n
					}
				}
			}
			windowDuration := 7 * 24 * time.Hour
			deleteAfterDuration := 30 * 24 * time.Hour
			if job.EnvironmentVariables != nil {
				if s, ok := job.EnvironmentVariables[EnvKeyAggregationWindow]; ok && s != emptyValue {
					// Strict ParseDuration only: bare numbers are not interpreted as hours (ambiguous).
					if d, err := time.ParseDuration(s); err == nil {
						windowDuration = d
					}
				}
				var retentionStr string
				if s, ok := job.EnvironmentVariables[EnvKeyRetentionDuration]; ok && s != emptyValue {
					retentionStr = s
				} else if s, ok := job.EnvironmentVariables[EnvKeyDeleteAfterDays]; ok && s != emptyValue {
					retentionStr = s
				}
				if retentionStr != emptyValue {
					if d, ok := parseDurationOrHoursSuffix(retentionStr); ok {
						deleteAfterDuration = d
					}
				}
			}
			windowEnd := time.Now().UTC()
			windowStart := windowEnd.Add(-windowDuration)
			service := storagepkg.NewChangeJournalAggregationServiceWithBatchSize(h.storage, batchSize)
			secCtx := pkgctx.NewSystemSecurityContext()
			storageCtx := pkgctx.NewStorageContext()

			in.Job = job
			in.BatchSize = batchSize
			in.WindowStart = windowStart
			in.WindowEnd = windowEnd
			in.DeleteAfterDuration = deleteAfterDuration
			in.Service = service
			in.SecCtx = secCtx
			in.StorageCtx = storageCtx

			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = job.ID
				pctx.Outcome[pipeline.OutcomeKeyIngestWindowStart] = windowStart.Format(time.RFC3339)
				pctx.Outcome[pipeline.OutcomeKeyIngestWindowEnd] = windowEnd.Format(time.RFC3339)
			}
			return payload, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*changeJournalAggPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *changeJournalAggPayload, got %T", payload)
			}
			// Health check (advisory)
			if err := h.preExecutionHealthCheck(ctx, in.Job, "change_journal_aggregation_metric"); err != nil {
				ChangeJournalAggregationPipelineLog(logger).Warn(LogEventChangeJournalPipelinePreExecHealthFailed).
					WithFields(jobLogFieldsWithErr(in.Job, err)...).
					Log()
			}
			// Enforce 3k limit: aggressive cleanup if at or above limit
			metricFilter := storagepkg.ListFilter{Kind: "change_journal_aggregation_metric"}
			metricCount, countErr := h.storage.Count(ctx, in.SecCtx, metricFilter)
			if countErr == nil && metricCount >= MaxAggregationObjectCountPerKind {
				ChangeJournalAggregationPipelineLog(logger).Warn(LogEventChangeJournalPipelineObjectCountAggressiveCleanup).
					JobID(in.Job.ID).
					Int("current_count", metricCount).
					Int("limit", MaxAggregationObjectCountPerKind).
					Log()
				aggressiveRetention := in.DeleteAfterDuration / 2
				cutoff := time.Now().UTC().Add(-aggressiveRetention)
				oldIDs, qErr := in.Service.QueryOldAggregatedEntries(ctx, in.SecCtx, in.StorageCtx, cutoff)
				if qErr == nil && len(oldIDs) > 0 {
					deleted, err := in.Service.CleanupAggregatedEntries(ctx, in.SecCtx, oldIDs, false)
					if err != nil {
						ChangeJournalAggregationPipelineLog(logger).Warn(LogEventChangeJournalPipelineCleanupFailed).
							JobID(in.Job.ID).
							WithError(err).
							Log()
					}
					ChangeJournalAggregationPipelineLog(logger).Info(LogEventChangeJournalPipelineAggressiveCleanupCompleted).
						JobID(in.Job.ID).
						Int("deleted_count", deleted).
						Log()
				}
			}
			result, err := in.Service.AggregateChangeJournalEntries(ctx, in.SecCtx, in.StorageCtx, in.WindowStart, in.WindowEnd)
			if err != nil {
				return nil, err
			}
			in.Result = result
			if pctx.Outcome != nil {
				if result != nil {
					pctx.Outcome[pipeline.OutcomeKeyNormalizeEntryCount] = result.EntryCount
					pctx.Outcome[pipeline.OutcomeKeyNormalizeMetricsCreated] = result.MetricsCreated
					pctx.Outcome[pipeline.OutcomeKeyNormalizeMetricID] = result.MetricID
				} else {
					pctx.Outcome[pipeline.OutcomeKeyNormalizeEntryCount] = 0
				}
			}
			return in, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*changeJournalAggPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *changeJournalAggPayload, got %T", payload)
			}
			// Optional: delete just-aggregated entries if DELETE_ENABLED
			if in.Result != nil && len(in.Result.EntriesProcessed) > 0 && in.Job.EnvironmentVariables != nil {
				if s, ok := in.Job.EnvironmentVariables[EnvKeyDeleteEnabled]; ok && s == "true" {
					if _, err := in.Service.CleanupAggregatedEntries(ctx, in.SecCtx, in.Result.EntriesProcessed, false); err != nil {
						ChangeJournalAggregationPipelineLog(logger).Warn(LogEventChangeJournalPipelineCleanupFailed).
							JobID(in.Job.ID).
							WithError(err).
							Log()
					}
				}
			}
			// Cleanup old aggregated entries (metrics) by retention
			if in.DeleteAfterDuration > 0 {
				cutoff := time.Now().UTC().Add(-in.DeleteAfterDuration)
				oldIDs, err := in.Service.QueryOldAggregatedEntries(ctx, in.SecCtx, in.StorageCtx, cutoff)
				if err == nil && len(oldIDs) > 0 {
					if _, err := in.Service.CleanupAggregatedEntries(ctx, in.SecCtx, oldIDs, false); err != nil {
						ChangeJournalAggregationPipelineLog(logger).Warn(LogEventChangeJournalPipelineCleanupFailed).
							JobID(in.Job.ID).
							WithError(err).
							Log()
					}
				}
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyCommitDone] = true
			}
			return in, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*changeJournalAggPayload](payload)
			if !ok {
				return payload, nil
			}
			if pctx.Outcome != nil && in.Result != nil {
				pctx.Outcome[pipeline.OutcomeKeyFinalizeEntryCount] = in.Result.EntryCount
				pctx.Outcome[pipeline.OutcomeKeyFinalizeMetricID] = in.Result.MetricID
			}
			return payload, nil
		}).
		Build()

	initial := &changeJournalAggPayload{Job: job}
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	if err != nil {
		return initial.Result, err
	}
	return initial.Result, nil
}
