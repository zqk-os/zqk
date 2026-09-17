package scheduler

import (
	"encoding/json"
	"path/filepath"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const dataCellEnvelopeTickJSONL = "data_cell_envelope_tick.jsonl"

// appendDataCellEnvelopeTickJSONL appends one JSON line per tick when the handler runs (POL-OBS-001 async pattern;
// mirrors cmd/zqk/system data_cells_stages.jsonl). Best-effort: errors are ignored by callers.
// resolvedJobTypesJoined must match the handler’s effective resolution (incl. token policy), not a second
// call to [datacell.ResolvedSchedulerJobTypesFromDiscoveryTokens] — that would desync metrics from dispatch.
func appendDataCellEnvelopeTickJSONL(projectRoot, jobID, kindAugJoined, kindOvJoined, resolvedJobTypesJoined string, summaries map[datacell.StorageProfile]string, dispatch EnvelopeTickDispatchOutcome) error {
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}
	p := filepath.Join(dir, dataCellEnvelopeTickJSONL)
	row := map[string]any{
		"ts_rfc3339":                                        time.Now().UTC().Format(time.RFC3339Nano),
		objects.FieldKeyJobType:                             JobTypeDataCellEnvelopeTick,
		"job_id":                                            jobID,
		"envelope_tick_scheduler_job_id":                    datacell.EnvelopeTickSchedulerJobID,
		"envelope_tick_job_type":                            datacell.EnvelopeTickJobType,
		"kind_operational_envelope_augments":                kindAugJoined,
		"kind_operational_envelope_overrides":               kindOvJoined,
		"operational_envelope_resolved_scheduler_job_types": resolvedJobTypesJoined,
	}
	for _, prof := range datacell.KnownStorageProfiles {
		if s, ok := summaries[prof]; ok {
			row["operational_envelope_"+string(prof)] = s
		}
	}
	if dispatch.Evaluated {
		row["envelope_tick_dispatch_evaluated"] = true
		row["envelope_tick_dispatch_mode"] = dispatch.Mode
		row["envelope_tick_dispatch_expand"] = dispatch.Expand
		row["envelope_tick_dispatch_skip_denied"] = dispatch.SkipDenied
		row["envelope_tick_dispatch_skip_denied_env"] = dispatch.SkipDeniedEnv
		row["envelope_tick_dispatch_skip_not_allowlisted"] = dispatch.SkipNotAllowlisted
		row["envelope_tick_dispatch_skip_no_registered_job"] = dispatch.SkipNoRegisteredJob
		row["envelope_tick_dispatch_shadow_would_trigger"] = dispatch.ShadowWouldTrigger
		row["envelope_tick_dispatch_triggered_ok"] = dispatch.TriggeredOK
		row["envelope_tick_dispatch_trigger_failed"] = dispatch.TriggerFailed
		row["envelope_tick_dispatch_skip_rate_limited"] = dispatch.SkipRateLimited
		row["envelope_tick_dispatch_duplicate_job_type_inputs_dropped"] = dispatch.DuplicateResolvedJobTypesDropped
		row["envelope_tick_dispatch_skip_duplicate_followup_job_id"] = dispatch.SkipDuplicateFollowupJobID
		row["envelope_tick_dispatch_skip_target_running"] = dispatch.SkipTargetRunning
		row["envelope_tick_dispatch_skip_target_pending_trigger_queue"] = dispatch.SkipTargetPendingTriggerQueue
		if dispatch.AllowlistExtraN > 0 {
			row["envelope_tick_dispatch_allowlist_extra_n"] = dispatch.AllowlistExtraN
		}
		if dispatch.DenyExtraN > 0 {
			row["envelope_tick_dispatch_deny_extra_n"] = dispatch.DenyExtraN
		}
		if dispatch.TokenPolicyDenyJSONPresent {
			row["envelope_tick_dispatch_token_deny_json"] = true
			row["envelope_tick_dispatch_token_deny_n"] = dispatch.TokenPolicyDenyEntriesN
		}
		if dispatch.TokenPolicyAllowJSONPresent {
			row["envelope_tick_dispatch_token_allow_json"] = true
			row["envelope_tick_dispatch_token_allow_n"] = dispatch.TokenPolicyAllowEntriesN
		}
	} else {
		row["envelope_tick_dispatch_evaluated"] = false
	}
	line, err := json.Marshal(row)
	if err != nil {
		return err
	}
	f, err := fileutil.OpenFile(p, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to close datacell envelope tick metrics log").WithError(err).Log()
		}
	}()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}
