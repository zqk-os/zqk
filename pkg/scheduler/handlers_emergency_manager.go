package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TRACK: out-of-band CAP / steward emergency monitor.

const (
	emergencyManagerStateFile   = "emergency_manager_last.json"
	defaultCAPFailThreshold     = 3
	defaultCAPStaleAfter        = 45 * time.Minute
	emergencyStashMessagePrefix = "emergency-manager: auto-stash while CAP unhealthy"
)

// EmergencyManagerHandler monitors CAP orchestrator health and optionally
// performs a gated git stash when the working tree is dirty during prolonged CAP failure.
type EmergencyManagerHandler struct {
	projectRoot string
	logger      logging.Logger
	// deps injectable for tests
	now            func() time.Time
	readFailure    func() (capFailureSnapshot, error)
	lastCAPSuccess func() (time.Time, bool)
	gitStatusDirty func() (bool, error)
	gitStash       func(msg string) error
}

type capFailureSnapshot struct {
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastFailure         string `json:"last_failure"`
	LastStage           string `json:"last_stage"`
	LastError           string `json:"last_error"`
}

type emergencyManagerReport struct {
	CheckedAt           string `json:"checked_at"`
	CAPConsecutiveFails int    `json:"cap_consecutive_failures"`
	CAPLastFailure      string `json:"cap_last_failure,omitempty"`
	CAPLastSuccess      string `json:"cap_last_success,omitempty"`
	CAPUnhealthy        bool   `json:"cap_unhealthy"`
	Reason              string `json:"reason,omitempty"`
	WorktreeDirty       bool   `json:"worktree_dirty"`
	StashAttempted      bool   `json:"stash_attempted"`
	StashPerformed      bool   `json:"stash_performed"`
	StashSkippedReason  string `json:"stash_skipped_reason,omitempty"`
	AllowStash          bool   `json:"allow_stash"`
}

// NewEmergencyManagerHandler creates the out-of-band emergency manager.
func NewEmergencyManagerHandler(projectRoot string, logger logging.Logger) JobHandler {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	h := &EmergencyManagerHandler{
		projectRoot: projectRoot,
		logger:      logger,
		now:         func() time.Time { return time.Now().UTC() },
	}
	h.readFailure = h.readCAPFailureTracker
	h.lastCAPSuccess = h.readLastCAPSuccessFromEvents
	h.gitStatusDirty = h.gitStatusDirtyDefault
	h.gitStash = h.gitStashDefault
	return h
}

func (h *EmergencyManagerHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	if h == nil || h.projectRoot == "" {
		return errfmt.Errorf("emergency_manager: project root required")
	}
	allowStash := emergencyAllowStash(job)
	failSnap, _ := h.readFailure()
	lastOK, haveOK := h.lastCAPSuccess()
	now := h.now()

	unhealthy := false
	reason := ""
	if failSnap.ConsecutiveFailures >= defaultCAPFailThreshold {
		unhealthy = true
		reason = fmt.Sprintf("cap consecutive_failures=%d (threshold=%d)", failSnap.ConsecutiveFailures, defaultCAPFailThreshold)
	} else if haveOK && now.Sub(lastOK) > defaultCAPStaleAfter {
		unhealthy = true
		reason = fmt.Sprintf("cap last success %s ago (stale after %s)", now.Sub(lastOK).Round(time.Minute), defaultCAPStaleAfter)
	} else if !haveOK && failSnap.LastFailure != "" {
		unhealthy = true
		reason = "cap has failure history and no recent success event"
	}

	dirty, dirtyErr := h.gitStatusDirty()
	if dirtyErr != nil {
		h.logger.Warn("emergency_manager_git_status_failed", logging.Error(dirtyErr))
		dirty = false
	}

	report := emergencyManagerReport{
		CheckedAt:           now.Format(time.RFC3339),
		CAPConsecutiveFails: failSnap.ConsecutiveFailures,
		CAPLastFailure:      failSnap.LastFailure,
		CAPUnhealthy:        unhealthy,
		Reason:              reason,
		WorktreeDirty:       dirty,
		AllowStash:          allowStash,
	}
	if haveOK {
		report.CAPLastSuccess = lastOK.Format(time.RFC3339)
	}

	if unhealthy {
		h.logger.Warn("emergency_manager_cap_unhealthy",
			logging.String("reason", reason),
			logging.Int("consecutive_failures", failSnap.ConsecutiveFailures),
			logging.Bool("worktree_dirty", dirty),
			logging.Bool("allow_stash", allowStash),
		)
		h.appendEmergencyChat(reason, failSnap)
		if dirty && allowStash {
			report.StashAttempted = true
			msg := fmt.Sprintf("%s at %s", emergencyStashMessagePrefix, now.Format(time.RFC3339))
			if err := h.gitStash(msg); err != nil {
				report.StashSkippedReason = err.Error()
				h.logger.Error("emergency_manager_stash_failed", err)
			} else {
				report.StashPerformed = true
				h.logger.Info("emergency_manager_stash_performed", logging.String("message", msg))
			}
		} else if dirty && !allowStash {
			report.StashSkippedReason = "allow_stash not set on job metadata (set metadata.allow_stash=true to enable stash)"
		} else if !dirty {
			report.StashSkippedReason = "worktree clean"
		}
	}

	h.writeReport(report)
	_ = ctx
	return nil
}

func emergencyAllowStash(job *ScheduledJob) bool {
	if job == nil || job.Metadata == nil {
		return false
	}
	switch v := job.Metadata["allow_stash"].(type) {
	case bool:
		return v
	case string:
		return v == "1" || strings.EqualFold(v, "true")
	default:
		return false
	}
}

func (h *EmergencyManagerHandler) statePath(name string) string {
	return filepath.Join(h.projectRoot, paths.ProjectDataDir, paths.StateDir, name)
}

func (h *EmergencyManagerHandler) readCAPFailureTracker() (capFailureSnapshot, error) {
	b, err := fileutil.ReadFile(h.statePath(capFailureTrackerFile))
	if err != nil {
		return capFailureSnapshot{}, err
	}
	var snap capFailureSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return capFailureSnapshot{}, err
	}
	return snap, nil
}

func (h *EmergencyManagerHandler) readLastCAPSuccessFromEvents() (time.Time, bool) {
	p := filepath.Join(h.projectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler", CapOrchestratorJobID, CapOrchestratorJobID+".events.jsonl")
	b, err := fileutil.ReadFile(p)
	if err != nil {
		return time.Time{}, false
	}
	lines := strings.Split(string(b), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		et, _ := ev[objects.FieldKeyEventType].(string)
		if et != "completed" {
			continue
		}
		ts, _ := ev["timestamp"].(string)
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return t.UTC(), true
		}
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func (h *EmergencyManagerHandler) gitStatusDirtyDefault() (bool, error) {
	cmd := execwrap.Command("git", "-C", h.projectRoot, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (h *EmergencyManagerHandler) gitStashDefault(msg string) error {
	// Stash including untracked; never hard-reset or clean -fd here.
	cmd := execwrap.Command("git", "-C", h.projectRoot, "stash", "push", "-u", "-m", msg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Errorf("git stash push: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (h *EmergencyManagerHandler) writeReport(report emergencyManagerReport) {
	stateDir := filepath.Join(h.projectRoot, paths.ProjectDataDir, paths.StateDir)
	_ = fileutil.EnsureDir(stateDir)
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return
	}
	_ = fileutil.WriteSecureFile(h.statePath(emergencyManagerStateFile), b)
}

func (h *EmergencyManagerHandler) appendEmergencyChat(reason string, snap capFailureSnapshot) {
	eventPath := datacell.AgentChatChannelEventsJSONLPath(h.projectRoot)
	if err := fileutil.MkdirAll(filepath.Dir(eventPath), paths.DirPerm755); err != nil {
		return
	}
	f, err := fileutil.OpenFile(eventPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return
	}
	defer f.Close()
	msg := fmt.Sprintf("EMERGENCY MANAGER: CAP unhealthy — %s. consecutive_failures=%d last_stage=%s. Review %s; stash gated (metadata.allow_stash).",
		reason, snap.ConsecutiveFailures, snap.LastStage, CapOrchestratorJobID)
	_ = json.NewEncoder(f).Encode(map[string]any{
		"timestamp": h.now().Format(time.RFC3339),
		"sender":    "emergency_manager",
		"message":   msg,
	})
}
