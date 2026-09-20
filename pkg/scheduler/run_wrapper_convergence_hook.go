package scheduler

import (
	"context"
)

// convergenceTickEnvKeysForwardedFromTestBundleJob lists parent test-bundle job env keys copied onto
// the synthetic convergence_session_tick when CONVERGENCE_SESSION_ID is set (see maybeRunConvergenceTickAfterTestBundleHealth).
var convergenceTickEnvKeysForwardedFromTestBundleJob = []string{
	EnvKeyHealthLimit,
	EnvKeyHealthFileMissing,
	EnvKeyHealthFileMissingSkipBudget,
	EnvKeyCurrentPhase,
	EnvKeyFlowVariant,
	EnvKeySkipSessionContext,
	EnvKeyConvergenceTickSpawnFollowupDraft,
	EnvKeyCVSMeasurementEvents,
	EnvKeyConvergenceTickRollup,
	EnvKeyCVSOrchestrateRollupOut,
}

// maybeRunConvergenceTickAfterTestBundleHealth runs one convergence_session_tick when the parent
// test-bundle job sets CONVERGENCE_SESSION_ID in environment_variables. This provides an
// event-shaped path (health.jsonl append from the bundle run → governed tick) without replacing
// periodic scheduler jobs. Optional keys in convergenceTickEnvKeysForwardedFromTestBundleJob are
// forwarded when present (including CONVERGENCE_TICK_SPAWN_FOLLOWUP_DRAFT for terminal-session follow-up drafts).
func (h *RunWrapperHandler) maybeRunConvergenceTickAfterTestBundleHealth(ctx context.Context, job *ScheduledJob) {
	if h == nil || job == nil {
		return
	}
	if h.storage == nil || h.projectRoot == emptyValue {
		return
	}
	if envLookup(job, EnvKeyConvergenceSessionID) == emptyValue {
		return
	}
	tickJob := &ScheduledJob{
		ID:      "SCH-tick-after-" + job.ID,
		JobType: JobTypeConvergenceSessionTick,
		EnvironmentVariables: map[string]string{
			EnvKeyConvergenceSessionID: envLookup(job, EnvKeyConvergenceSessionID),
		},
	}
	for _, k := range convergenceTickEnvKeysForwardedFromTestBundleJob {
		if v := envLookup(job, k); v != emptyValue {
			tickJob.EnvironmentVariables[k] = v
		}
	}
	handler := NewConvergenceSessionTickHandler(h.storage, h.projectRoot)
	if err := handler.Execute(ctx, tickJob); err != nil {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperConvergenceTickAfterTestBundleHealthFailed).
			String("parent_job_id", job.ID).
			SessionID(tickJob.EnvironmentVariables[EnvKeyConvergenceSessionID]).
			WithError(err).
			Log()
	}
}
