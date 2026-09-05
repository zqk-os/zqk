package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

const (
	healthFileMissingModeFail          = "fail"
	healthFileMissingModeSkip          = "skip"
	healthFileMissingModeEmptyBaseline = "empty_baseline"
)

// Streak file: per-job consecutive skips when health.jsonl is missing (mode=skip).
// Reset when health.jsonl exists and is read successfully. Bounded by HEALTH_FILE_MISSING_SKIP_BUDGET.
const convergenceHealthMissingStreakFile = "convergence_health_missing_streak.json"

var healthMissingStreakMu sync.Mutex

func healthMissingStreakFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, convergenceHealthMissingStreakFile)
}

func resetHealthMissingStreakForJob(projectRoot, jobID string) {
	if projectRoot == "" || jobID == "" {
		return
	}
	healthMissingStreakMu.Lock()
	defer healthMissingStreakMu.Unlock()
	p := healthMissingStreakFilePath(projectRoot)
	b, err := fileutil.ReadFile(p)
	if err != nil || len(b) == 0 {
		return
	}
	var m map[string]int
	if json.Unmarshal(b, &m) != nil || m == nil {
		return
	}
	if _, ok := m[jobID]; !ok {
		return
	}
	delete(m, jobID)
	if len(m) == 0 {
		if err := fileutil.Remove(p); err != nil && !fileutil.IsNotExist(err) {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to remove empty health missing streak file").WithError(err).Log()
		}
		return
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	if err := fileutil.WriteSecureFile(p, out); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to update health missing streak file").WithError(err).Log()
	}
}

// bumpHealthMissingSkipStreak increments the per-job streak and returns the new value.
func bumpHealthMissingSkipStreak(projectRoot, jobID string) int {
	healthMissingStreakMu.Lock()
	defer healthMissingStreakMu.Unlock()
	p := healthMissingStreakFilePath(projectRoot)
	m := map[string]int{}
	if b, err := fileutil.ReadFile(p); err == nil && len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to unmarshal existing health missing streak file").WithError(err).Log()
		}
	}
	if m == nil {
		m = map[string]int{}
	}
	m[jobID]++
	n := m[jobID]
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return n
	}
	if err := fileutil.EnsureDir(filepath.Dir(p)); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to create directory for health missing streak file").WithError(err).Log()
	}
	if err := fileutil.WriteSecureFile(p, b); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to write updated health missing streak file").WithError(err).Log()
	}
	return n
}

func parseHealthFileMissingSkipBudget(job *ScheduledJob) int {
	s := strings.TrimSpace(envLookup(job, EnvKeyHealthFileMissingSkipBudget))
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// resolveTestBundleHealthLinesForTick loads health.jsonl tail lines, or handles a missing file per
// HEALTH_FILE_MISSING (fail | skip | empty_baseline). Returns (lines, done, err): when done is true,
// the handler should return err (which may be nil for successful skip).
func (h *ConvergenceSessionTickHandler) resolveTestBundleHealthLinesForTick(ctx context.Context, job *ScheduledJob, limit int) ([]map[string]any, bool, error) {
	path := TestBundlesHealthFilePath(h.projectRoot)
	if _, err := fileutil.Stat(path); err != nil {
		if !fileutil.IsNotExist(err) {
			return nil, false, errfmt.Newf("convergence_session_tick: stat health").Wrap(err)
		}
		mode := strings.ToLower(strings.TrimSpace(envLookup(job, EnvKeyHealthFileMissing)))
		if mode == "" {
			mode = healthFileMissingModeFail
		}
		switch mode {
		case healthFileMissingModeFail:
			return nil, false, errfmt.Errorf("convergence_session_tick: read health: open %s: no such file or directory", path)
		case healthFileMissingModeSkip:
			budget := parseHealthFileMissingSkipBudget(job)
			streak := bumpHealthMissingSkipStreak(h.projectRoot, job.ID)
			if budget > 0 && streak > budget {
				return nil, false, errfmt.Errorf("convergence_session_tick: health.jsonl missing skip budget exceeded (%d consecutive skips; budget=%d)", streak, budget)
			}
			if h.logger != nil {
				ConvergenceSessionTickLog(h.logger).Info(LogEventConvergenceSessionTickSkippedHealthJSONLMissing).
					JobID(job.ID).
					Int("consecutive_skips", streak).
					Int("skip_budget", budget).
					Log()
			}
			return nil, true, nil
		case healthFileMissingModeEmptyBaseline:
			return nil, false, nil
		default:
			return nil, false, errfmt.Errorf("convergence_session_tick: invalid %s=%q (want %s, %s, %s)",
				EnvKeyHealthFileMissing, mode, healthFileMissingModeFail, healthFileMissingModeSkip, healthFileMissingModeEmptyBaseline)
		}
	}

	resetHealthMissingStreakForJob(h.projectRoot, job.ID)

	lines, err := ReadTestBundleHealthTailLines(ctx, h.projectRoot, limit)
	if err != nil {
		return nil, false, errfmt.Newf("convergence_session_tick: read health").Wrap(err)
	}
	return lines, false, nil
}
