package scheduler

import (
	"context"
	"sort"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// SyncCVSPipelineTickJobOnLifecycle updates SCH-cvs-pipeline-tick's CONVERGENCE_SESSION_ID when:
//   - The job pointed at a convergence_session that just reached a terminal status → repoint to the best
//     matching active session (see PIPELINE_TICK_TITLE_SUBSTRING).
//   - A convergence_session becomes active while the job still targets a terminal session → repoint to this
//     session when its title matches the optional substring.
//
// Best-effort: returns nil always so storage lifecycle commits are never rolled back.
func SyncCVSPipelineTickJobOnLifecycle(ctx context.Context, storage storagepkg.ObjectStorageProvider, fromState, toState string, sessionObj map[string]any) error {
	if storage == nil || len(sessionObj) == 0 {
		return nil
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	job, err := storage.Read(ctx, secCtx, CVSPipelineTickJobID)
	if err != nil || job == nil {
		return nil
	}
	if FieldAsString(job[objects.FieldKeyJobType]) != JobTypeConvergenceSessionTick {
		return nil
	}
	env, ok := job[objects.FieldKeyEnvironmentVariables].(map[string]any)
	if !ok || len(env) == 0 {
		return nil
	}
	if !pipelineTickAutoRepointEnabled(env) {
		return nil
	}

	sessionID, _ := sessionObj[objects.FieldKeyID].(string)
	if sessionID == "" {
		return nil
	}
	titleSub := strings.TrimSpace(FieldAsString(env[EnvKeyPipelineTickTitleSubstring]))

	ll := objects.GetGlobalLifecycleLoader()

	switch {
	case shouldRepointOnTerminalTransition(ll, toState):
		current := strings.TrimSpace(FieldAsString(env[EnvKeyConvergenceSessionID]))
		if current == "" || current != sessionID {
			return nil
		}
		replacement, ok := pickReplacementConvergenceSessionID(ctx, storage, secCtx, titleSub)
		if !ok || replacement == "" || replacement == sessionID {
			CVSPipelineTickSyncLog(logger).Info(LogEventCVSPipelineTickSyncNoReplacementActive).
				String("completed_session_id", sessionID).
				String("title_substring_filter", titleSub).
				Log()
			return nil
		}
		if err := updatePipelineTickConvergenceSessionID(ctx, storage, secCtx, env, replacement); err != nil {
			CVSPipelineTickSyncLog(logger).Warn(LogEventCVSPipelineTickSyncRepointAfterCompletedFailed).
				String("session_id", sessionID).
				String("replacement_session_id", replacement).
				WithError(err).
				Log()
			return nil
		}
		CVSPipelineTickSyncLog(logger).Info(LogEventCVSPipelineTickSyncRepointedAfterCompleted).
			String("completed_session_id", sessionID).
			String("replacement_session_id", replacement).
			Log()
		return nil

	case shouldRepointOnActivateTransition(fromState, toState):
		if titleSub == "" {
			return nil
		}
		title := FieldAsString(sessionObj[objects.FieldKeyTitle])
		if !strings.Contains(strings.ToLower(title), strings.ToLower(titleSub)) {
			return nil
		}
		current := strings.TrimSpace(FieldAsString(env[EnvKeyConvergenceSessionID]))
		if current == sessionID {
			return nil
		}
		if current == "" {
			if err := updatePipelineTickConvergenceSessionID(ctx, storage, secCtx, env, sessionID); err != nil {
				CVSPipelineTickSyncLog(logger).Warn(LogEventCVSPipelineTickSyncSetTargetOnActivateFailed).
					String("session_id", sessionID).
					WithError(err).
					Log()
				return nil
			}
			CVSPipelineTickSyncLog(logger).Info(LogEventCVSPipelineTickSyncSetTargetOnActivate).
				String("session_id", sessionID).
				Log()
			return nil
		}
		prev, err := storage.Read(ctx, secCtx, current)
		if err != nil || len(prev) == 0 {
			if err := updatePipelineTickConvergenceSessionID(ctx, storage, secCtx, env, sessionID); err != nil {
				CVSPipelineTickSyncLog(logger).Warn(LogEventCVSPipelineTickSyncRepointMissingPriorFailed).
					String("prior_target", current).
					String("session_id", sessionID).
					WithError(err).
					Log()
				return nil
			}
			CVSPipelineTickSyncLog(logger).Info(LogEventCVSPipelineTickSyncRepointedPriorMissing).
				String("replacement_session_id", sessionID).
				Log()
			return nil
		}
		prevStatus := FieldAsString(prev[objects.FieldKeyStatus])
		terminal, terr := ll.IsTerminalStatusForKind(objects.KindConvergenceSession, prevStatus)
		if terr != nil || !terminal {
			return nil
		}
		if err := updatePipelineTickConvergenceSessionID(ctx, storage, secCtx, env, sessionID); err != nil {
			CVSPipelineTickSyncLog(logger).Warn(LogEventCVSPipelineTickSyncRepointTerminalPriorFailed).
				String("prior_target", current).
				String("session_id", sessionID).
				WithError(err).
				Log()
			return nil
		}
		CVSPipelineTickSyncLog(logger).Info(LogEventCVSPipelineTickSyncRepointedTerminalPrior).
			String("prior_target", current).
			String("replacement_session_id", sessionID).
			Log()
	default:
		return nil
	}
	return nil
}

func pipelineTickAutoRepointEnabled(env map[string]any) bool {
	if env == nil {
		return true
	}
	v := strings.TrimSpace(strings.ToLower(FieldAsString(env[EnvKeyPipelineTickAutoRepoint])))
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}

func shouldRepointOnTerminalTransition(ll *objects.LifecycleLoader, toState string) bool {
	if ll == nil {
		return false
	}
	terminal, err := ll.IsTerminalStatusForKind(objects.KindConvergenceSession, toState)
	return err == nil && terminal
}

func shouldRepointOnActivateTransition(fromState, toState string) bool {
	if toState != objects.ObjectStatusActive {
		return false
	}
	switch fromState {
	case "draft", "paused":
		return true
	default:
		return false
	}
}

func pickReplacementConvergenceSessionID(ctx context.Context, storage storagepkg.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, titleSubstring string) (string, bool) {
	storCtx := pkgctx.NewStorageContext()
	res, err := storage.List(ctx, secCtx, storCtx, storagepkg.ListFilter{
		Kind:    objects.KindConvergenceSession,
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive},
		Limit:   500,
	})
	if err != nil || res == nil || len(res.Objects) == 0 {
		return "", false
	}
	candidates := filterConvergenceSessionsByTitle(res.Objects, titleSubstring)
	if len(candidates) == 0 {
		return "", false
	}
	if titleSubstring == "" && len(candidates) != 1 {
		return "", false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return convergenceSessionUpdatedAt(candidates[i]).After(convergenceSessionUpdatedAt(candidates[j]))
	})
	id := FieldAsString(candidates[0][objects.FieldKeyID])
	return id, id != ""
}

func filterConvergenceSessionsByTitle(objs []map[string]any, titleSubstring string) []map[string]any {
	if titleSubstring == "" {
		return objs
	}
	sub := strings.ToLower(titleSubstring)
	var out []map[string]any
	for _, o := range objs {
		t := FieldAsString(o[objects.FieldKeyTitle])
		if strings.Contains(strings.ToLower(t), sub) {
			out = append(out, o)
		}
	}
	return out
}

func convergenceSessionUpdatedAt(obj map[string]any) time.Time {
	s := FieldAsString(obj[objects.FieldKeyUpdatedAt])
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func updatePipelineTickConvergenceSessionID(ctx context.Context, storage storagepkg.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, env map[string]any, newID string) error {
	newEnv := make(map[string]any, len(env)+1)
	for k, v := range env {
		newEnv[k] = v
	}
	newEnv[EnvKeyConvergenceSessionID] = newID
	return storage.Update(ctx, secCtx, CVSPipelineTickJobID, map[string]any{
		objects.FieldKeyEnvironmentVariables: newEnv,
	})
}
