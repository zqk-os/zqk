package scheduler

import (
	"context"
	"fmt"
	"os"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func criteriaAutoValidateDisabled() bool {
	v := strings.TrimSpace(os.Getenv(zqkenv.DisableCriteriaAutoValidate()))
	switch strings.ToLower(v) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// applyCriterionValidatedFromTestBundle returns didUpdate true when storage.Update ran successfully;
// noop (false, nil) when the criterion was already validated/complete.
func applyCriterionValidatedFromTestBundle(ctx context.Context, stor storagepkg.ObjectStorageProvider, critID string) (didUpdate bool, err error) {
	if stor == nil || critID == emptyValue {
		return false, nil
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	cur, err := stor.Read(ctx, secCtx, critID)
	if err != nil {
		return false, err
	}
	kind, _ := cur[objects.FieldKeyKind].(string)
	if kind != objects.KindCriteria {
		return false, errfmt.Errorf("expected kind criteria, got %q", kind)
	}
	st, _ := cur[objects.FieldKeyStatus].(string)
	if st == objects.ObjectStatusValidated || st == objects.ObjectStatusCompleted || st == "complete" {
		return false, nil
	}
	updCtx := pkgctx.WithForceLifecycleOverride(ctx)
	if err := stor.Update(updCtx, secCtx, critID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusValidated,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// maybeAutoValidateCriteriaFromSatisfiedTestBundle mirrors criteria_verification_evidence satisfaction:
// green SCH-run-* bundles with bundle metadata criteria_refs promote those criteria to validated so lifecycle
// hooks can emit WAL events (milestones, backlog_item completion). Opt out: ZQK_DISABLE_CRITERIA_AUTO_VALIDATE.
func (h *RunWrapperHandler) maybeAutoValidateCriteriaFromSatisfiedTestBundle(ctx context.Context, job *ScheduledJob, healthOutcome string, testsFailed int) {
	if h == nil || h.storage == nil || job == nil || h.projectRoot == emptyValue {
		return
	}
	satisfied := testsFailed == 0 && healthOutcome != runWrapperTestOutcomeTF
	crit, errRefs := criteriaAndTestCaseRefsFromMetadata(job.Metadata)
	if len(errRefs) > 0 {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to extract criteria refs from metadata").
			String("errors", fmt.Sprintf("%v", errRefs)).
			Log()
	}
	if !satisfied || len(crit) == 0 {
		return
	}
	if criteriaAutoValidateDisabled() {
		metrics.AppendProcessLifecycleJSONL(h.projectRoot, map[string]any{
			objects.FieldKeyEventType:         metrics.ProcessLifecycleEventCriteriaAutovalidateBatch,
			"job_id":                          job.ID,
			"skipped_disabled":                true,
			"criteria_refs_count":             len(crit),
			"criteria_validation_opt_out_env": zqkenv.DisableCriteriaAutoValidate(),
		})
		return
	}
	var appliedCount, noopCount, failedCount int
	for _, critID := range crit {
		if critID == emptyValue {
			continue
		}
		did, err := applyCriterionValidatedFromTestBundle(ctx, h.storage, critID)
		if err != nil {
			failedCount++
			RunWrapperLog(h.logger).Warn(LogEventRunWrapperCriteriaAutovalidateFailed).
				String("criteria_id", critID).
				JobID(job.ID).
				WithError(err).
				Log()
			continue
		}
		if did {
			appliedCount++
		} else {
			noopCount++
		}
	}
	var flushErrStr string
	if err := storagepkg.EnsureCLIObjectMutationVisibleForProvider(ctx, h.storage, h.projectRoot, []string{objects.KindCriteria}); err != nil {
		flushErrStr = metrics.TruncateProcessLifecycleDetail(err.Error(), 512)
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCriteriaAutovalidateFailed).
			JobID(job.ID).
			String("phase", "durability_flush").
			WithError(err).
			Log()
	}
	row := map[string]any{
		objects.FieldKeyEventType:                  metrics.ProcessLifecycleEventCriteriaAutovalidateBatch,
		"job_id":                                   job.ID,
		"criteria_refs_count":                      len(crit),
		"criteria_autovalidate_applied":            appliedCount,
		"criteria_autovalidate_noop_already_valid": noopCount,
		"criteria_autovalidate_failed":             failedCount,
	}
	if flushErrStr != emptyValue {
		row["durability_flush_error"] = flushErrStr
	}
	metrics.AppendProcessLifecycleJSONL(h.projectRoot, row)
}
