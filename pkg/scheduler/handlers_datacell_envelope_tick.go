package scheduler

import (
	"context"
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
)

// Log events for data_cell_envelope_tick handler (POL-CODE-007 stable keys; dashboards may join on message).
const (
	// LogEventDataCellEnvelopeTick is the primary Info line per tick (same wire as [JobTypeDataCellEnvelopeTick]).
	LogEventDataCellEnvelopeTick             = JobTypeDataCellEnvelopeTick
	LogEventDataCellEnvelopeTickStewardDrain = "data_cell_envelope_tick_steward_drain"
)

// DataCellEnvelopeTickHandler is the v1 operational-envelope job: structured log + success.
// Extensible for per-profile envelope actions (cleanup, retention probes, etc.).
type DataCellEnvelopeTickHandler struct {
	projectRoot string
	logger      logging.Logger
	scheduler   *Scheduler // optional; when set, optional follow-up dispatch (see envelope_tick_dispatch.go)
}

// NewDataCellEnvelopeTickHandler constructs the handler (job_type data_cell_envelope_tick).
// NewDataCellEnvelopeTickHandler creates a new data cell envelope tick handler
func NewDataCellEnvelopeTickHandler(projectRoot string, logger logging.Logger, scheduler *Scheduler) DataCellEnvelopeTickHandlerInterface {
	return &DataCellEnvelopeTickHandler{projectRoot: projectRoot, logger: logger, scheduler: scheduler}
}

// Execute logs compact operational-envelope summaries for every known storage profile (CAS entity,
// light file, stream) plus per-kind augments and per-kind override rows — v1 observability for the
// full data-cell model on each tick without running heavy maintenance work (that remains profile-specific handlers).
func (h *DataCellEnvelopeTickHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	if job == nil {
		return nil
	}
	if h.projectRoot != "" {
		drained, dErr := datacell.DrainStewardEnqueueLog(h.projectRoot, h.logger, job.ID)
		if dErr != nil && h.logger != nil {
			DataCellEnvelopeTickLog(h.logger).Warn(LogEventDataCellEnvelopeTickStewardDrain).
				JobID(job.ID).
				WithError(dErr).
				Log()
		} else if h.logger != nil && drained > 0 {
			DataCellEnvelopeTickLog(h.logger).Info(LogEventDataCellEnvelopeTickStewardDrain).
				JobID(job.ID).
				Int("drained_lines", drained).
				Log()
		}
	}
	kindAug := datacell.EnvelopeTickKindAugmentSummaries()
	kindAugJoined := strings.Join(kindAug, " | ")
	kindOv := datacell.EnvelopeTickKindOverrideSummaries()
	kindOvJoined := strings.Join(kindOv, " | ")
	summaries := datacell.AllOperationalEnvelopeCompactSummaries()
	resolved := datacell.ResolvedSchedulerJobTypesFromDiscoveryTokens()
	denyTok, allowTok, denyTokKey, allowTokKey, tokPolErr := envelopeTickTokenPolicyFromEnv(job.EnvironmentVariables)
	if tokPolErr != nil && h.logger != nil {
		DataCellEnvelopeTickLog(h.logger).Warn("data_cell_envelope_tick_token_policy_json_invalid").
			JobID(job.ID).
			WithError(tokPolErr).
			Log()
	}
	if tokPolErr == nil && (denyTokKey || allowTokKey) {
		resolved = datacell.ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy(
			datacell.ResolvedEnvelopeTokenJobEdges(),
			denyTok,
			allowTok,
			allowTokKey,
		)
	}
	resolvedJT := strings.Join(resolved, ",")
	if h.logger != nil {
		fields := []logging.Field{
			logging.JobIDField(job.ID),
			logging.String("category", job.Category),
			logging.String("envelope_tick_scheduler_job_id", datacell.EnvelopeTickSchedulerJobID),
			logging.String("envelope_tick_job_type", datacell.EnvelopeTickJobType),
			logging.String("kind_operational_envelope_augments", kindAugJoined),
			logging.String("kind_operational_envelope_overrides", kindOvJoined),
			logging.String("operational_envelope_resolved_scheduler_job_types", resolvedJT),
		}
		// Fixed order (not map range) so log lines are stable for diffing and dashboards.
		for _, p := range datacell.KnownStorageProfiles {
			if summary, ok := summaries[p]; ok {
				fields = append(fields, logging.String("operational_envelope_"+string(p), summary))
			}
		}
		DataCellEnvelopeTickLog(h.logger).Info(LogEventDataCellEnvelopeTick).
			WithFields(fields...).
			Log()
	}
	dispatchOutcome := EnvelopeTickDispatchOutcome{}
	if h.scheduler != nil {
		// REQ-STREAM-001: The data cell steward is responsible for triggering aggregation and retention.
		// We append them to the resolved list so they are dispatched as part of the envelope tick.
		resolvedWithStewardship := append(resolved, JobTypeAuditEventAggregation, JobTypeRetentionTolerance)
		dispatchOutcome = h.scheduler.DispatchEnvelopeTickResolvedJobs(ctx, job, resolvedWithStewardship)
	}
	if tokPolErr == nil {
		if denyTokKey {
			dispatchOutcome.TokenPolicyDenyJSONPresent = true
			dispatchOutcome.TokenPolicyDenyEntriesN = len(denyTok)
		}
		if allowTokKey {
			dispatchOutcome.TokenPolicyAllowJSONPresent = true
			dispatchOutcome.TokenPolicyAllowEntriesN = len(allowTok)
		}
	}
	// JSONL is durable operator signal (like test-bundles health); not gated on metricsrecording.
	if h.projectRoot != "" {
		if err := appendDataCellEnvelopeTickJSONL(h.projectRoot, job.ID, kindAugJoined, kindOvJoined, resolvedJT, summaries, dispatchOutcome); err != nil {
			SLog(h.logger).Debug("Failed to append datacell envelope tick JSONL").WithError(err).Log()
		}
	}
	return nil
}
