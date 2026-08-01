package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

const schedulerSkipWindowFile = "skip_window.yaml"

// SchedulerSkipWindow is optional YAML under .zqk/scheduler/skip_window.yaml. When active (now.Before(Until)),
// matching jobs are skipped at policy evaluation (ExecutionDecision skip) so handlers do not run.
type SchedulerSkipWindow struct {
	SchemaVersion string    `yaml:"schema_version,omitempty"`
	Until         time.Time `yaml:"until"`
	MatchAll      bool      `yaml:"match_all,omitempty"`
	JobTypes      []string  `yaml:"job_types,omitempty"`
	TriggerTypes  []string  `yaml:"trigger_types,omitempty"`
	TitleContains string    `yaml:"title_contains,omitempty"`
}

func schedulerSkipWindowPath(schedulerDir string) string {
	return filepath.Join(schedulerDir, schedulerSkipWindowFile)
}

// LoadSchedulerSkipWindow reads skip_window.yaml from the scheduler directory (.zqk/scheduler).
// Returns (nil, nil) if missing, expired, unmarshaling fails, or Until is zero.
func LoadSchedulerSkipWindow(schedulerDir string, now time.Time) (*SchedulerSkipWindow, error) {
	if strings.TrimSpace(schedulerDir) == "" {
		return nil, nil
	}
	p := schedulerSkipWindowPath(schedulerDir)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		// Treat I/O errors as "no window" so policy evaluation never blocks all jobs.
		return nil, nil
	}
	var sw SchedulerSkipWindow
	if err := yaml.Unmarshal(b, &sw); err != nil {
		return nil, nil
	}
	if sw.Until.IsZero() || !now.Before(sw.Until) {
		return nil, nil
	}
	return &sw, nil
}

// JobMatchesSchedulerSkipWindow reports whether a scheduled job matches the skip window filters.
func JobMatchesSchedulerSkipWindow(job *ScheduledJob, sw *SchedulerSkipWindow) bool {
	if sw == nil || job == nil {
		return false
	}
	if sw.MatchAll {
		return true
	}
	hasFilter := len(sw.JobTypes) > 0 || len(sw.TriggerTypes) > 0 || strings.TrimSpace(sw.TitleContains) != ""
	if !hasFilter {
		return false
	}
	if len(sw.JobTypes) > 0 && !stringSliceContainsFold(sw.JobTypes, job.JobType) {
		return false
	}
	if len(sw.TriggerTypes) > 0 && !stringSliceContainsFold(sw.TriggerTypes, job.TriggerType) {
		return false
	}
	if tc := strings.TrimSpace(sw.TitleContains); tc != "" {
		if !strings.Contains(strings.ToLower(job.Title), strings.ToLower(tc)) {
			return false
		}
	}
	return true
}

func stringSliceContainsFold(haystack []string, needle string) bool {
	n := strings.TrimSpace(needle)
	if n == "" {
		return false
	}
	for _, h := range haystack {
		if strings.EqualFold(strings.TrimSpace(h), n) {
			return true
		}
	}
	return false
}

// SaveSchedulerSkipWindow writes skip_window.yaml atomically under schedulerDir.
func SaveSchedulerSkipWindow(schedulerDir string, sw *SchedulerSkipWindow) error {
	if sw == nil {
		return errfmt.Errorf("skip window required")
	}
	if err := fileutil.EnsureDir(schedulerDir); err != nil {
		return errfmt.Errorf("mkdir scheduler dir: %w", err)
	}
	if sw.SchemaVersion == "" {
		sw.SchemaVersion = executionPolicySchemaVersion
	}
	b, err := yaml.Marshal(sw)
	if err != nil {
		return errfmt.Errorf("marshal skip_window: %w", err)
	}
	p := schedulerSkipWindowPath(schedulerDir)
	tmp, err := os.CreateTemp(schedulerDir, "skip_window-*.yaml.tmp")
	if err != nil {
		return errfmt.Errorf("create temp skip_window: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return errfmt.Errorf("write temp skip_window: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, p); err != nil {
		_ = os.Remove(tmpPath)
		return errfmt.Errorf("rename skip_window: %w", err)
	}
	return nil
}

// ClearSchedulerSkipWindow removes skip_window.yaml if present.
func ClearSchedulerSkipWindow(schedulerDir string) error {
	if strings.TrimSpace(schedulerDir) == "" {
		return nil
	}
	p := schedulerSkipWindowPath(schedulerDir)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return errfmt.Errorf("remove skip_window: %w", err)
	}
	return nil
}

// FormatSkipWindowReason returns a short reason string for policy decisions.
func FormatSkipWindowReason(sw *SchedulerSkipWindow) string {
	if sw == nil {
		return "scheduler skip_window"
	}
	return fmt.Sprintf("scheduler skip_window until %s", sw.Until.UTC().Format(time.RFC3339))
}
