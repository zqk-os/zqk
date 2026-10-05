// Config-driven cleanup handler: single input (cleanup config YAML) → execute steps in order.
// See docs/architecture/CLEANUP_MAINTENANCE_DESIGN.md and pkg/cleanup.
package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/cleanup"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Log events for cleanup (config-driven) handler (POL-CODE-007 stable keys).
const (
	LogEventCleanupSkipNoProjectRoot  = JobTypeCleanup + "_skip_no_project_root"
	LogEventCleanupStepNotImplemented = JobTypeCleanup + "_step_not_implemented"
	LogEventCleanupDeletedFile        = JobTypeCleanup + "_deleted_file"
	LogEventCleanupTruncatedFile      = JobTypeCleanup + "_truncated_file"
)

// CleanupConfigHandler runs cleanup steps from a single config file (default .zqk/cleanup/config.yaml).
type CleanupConfigHandler struct {
	projectRoot string
	logger      logging.Logger
}

// NewCleanupConfigHandler creates a handler that loads cleanup config and runs steps.
// NewCleanupConfigHandler creates a new cleanup config handler
func NewCleanupConfigHandler(projectRoot string, logger logging.Logger) CleanupConfigHandlerInterface {
	return &CleanupConfigHandler{
		projectRoot: projectRoot,
		logger:      logger,
	}
}

// Execute loads the cleanup config (single track: one file) and runs each step in order.
func (h *CleanupConfigHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunCleanupConfigViaPipeline(ctx, h, job)
}

// executeCleanupConfigCore loads the cleanup config (single track: one file) and runs each step in order.
// Called from RunCleanupConfigViaPipeline NORMALIZE stage.
func (h *CleanupConfigHandler) executeCleanupConfigCore(ctx context.Context, job *ScheduledJob) error {
	if h.projectRoot == emptyValue {
		SLog(h.logger).Warn(LogEventCleanupSkipNoProjectRoot).
			JobID(job.ID).
			Log()
		return nil
	}
	configPath := cleanup.DefaultConfigPath
	cfg, err := cleanup.LoadFromPath(h.projectRoot, configPath)
	if err != nil {
		return errfmt.Newf("load cleanup config").Wrap(err)
	}
	workDir := h.projectRoot
	if cfg.WorkingDirectory != emptyValue {
		workDir = filepath.Join(h.projectRoot, cfg.WorkingDirectory)
	}
	for i, step := range cfg.Steps {
		if step.Type == emptyValue {
			continue
		}
		if err := h.runStep(ctx, job, workDir, step, i+1); err != nil {
			return errfmt.Errorf("cleanup step %d (%s): %w", i+1, step.Type, err)
		}
	}
	return nil
}

// runStep runs one step (cli, build_target, delete_files, truncate_files). Single track: one executor per type.
func (h *CleanupConfigHandler) runStep(ctx context.Context, job *ScheduledJob, workDir string, step cleanup.Step, index int) error {
	switch step.Type {
	case "cli":
		return h.runCLI(ctx, workDir, step.Params)
	case "build_target":
		return h.runBuildTarget(ctx, workDir, step.Params)
	case "delete_files":
		return h.runDeleteFiles(workDir, step.Params)
	case "truncate_files":
		return h.runTruncateFiles(workDir, step.Params)
	case "reap_stale_locks":
		return h.runReapStaleLocks(workDir, step.Params)
	case "reap_temp_files":
		return h.runReapTempFiles(workDir, step.Params)
	case "enforce_log_retention":
		return h.runEnforceLogRetention(workDir, step.Params)
	case "reap_orphaned_processes":
		return h.runReapOrphanedProcesses(workDir, step.Params)
	default:
		SLog(h.logger).Debug(LogEventCleanupStepNotImplemented).
			JobID(job.ID).
			String("step_type", step.Type).
			Int("step_index", index).
			Log()
		return nil
	}
}

func strParam(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func strSliceParam(m map[string]any, key string) []string {
	if v, ok := m[key].([]any); ok {
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func (h *CleanupConfigHandler) runCLI(ctx context.Context, workDir string, params map[string]any) error {
	command := strParam(params, "command")
	if command == emptyValue {
		return errfmt.Errorf("cli step requires 'command'")
	}
	args := strSliceParam(params, "args")
	if !filepath.IsAbs(command) && workDir != emptyValue {
		command = filepath.Join(workDir, command)
	}
	cmd := execwrap.CommandContext(ctx, command, args...)
	cmd.Dir = workDir
	return execwrap.RunWithStandardStreams(cmd)
}

func (h *CleanupConfigHandler) runBuildTarget(ctx context.Context, workDir string, params map[string]any) error {
	tool := strParam(params, "tool")
	target := strParam(params, "target")
	file := strParam(params, "file")
	if tool == emptyValue {
		return errfmt.Errorf("build_target step requires 'tool'")
	}
	switch tool {
	case "make":
		args := []string{}
		if file != emptyValue {
			args = append(args, "-f", file)
		}
		if target != emptyValue {
			args = append(args, target)
		}
		cmd := execwrap.CommandContext(ctx, "make", args...)
		cmd.Dir = workDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		return errfmt.Errorf("build_target tool %q not implemented (use cli step for ant/gradle/npm)", tool)
	}
}

func (h *CleanupConfigHandler) runDeleteFiles(workDir string, params map[string]any) error {
	pathGlob := strParam(params, "path")
	if pathGlob == emptyValue {
		return errfmt.Errorf("delete_files step requires 'path'")
	}
	if !filepath.IsAbs(pathGlob) {
		pathGlob = filepath.Join(workDir, pathGlob)
	}
	matches, err := filepath.Glob(pathGlob)
	if err != nil {
		return errfmt.Errorf("glob %s: %w", pathGlob, err)
	}
	olderThan := strParam(params, "older_than")
	var cutoff *time.Time
	if olderThan != emptyValue {
		d, err := parseDuration(olderThan)
		if err != nil {
			return errfmt.Errorf("older_than %q: %w", olderThan, err)
		}
		t := time.Now().Add(-d)
		cutoff = &t
	}
	for _, p := range matches {
		if info, err := fileutil.Stat(p); err != nil || info.IsDir() {
			continue
		}
		if cutoff != nil {
			info, errStat := fileutil.Stat(p)
			if errStat != nil {
				continue
			}
			if info.ModTime().After(*cutoff) {
				continue
			}
		}
		if err := fileutil.Remove(p); err != nil {
			return errfmt.Errorf("delete %s: %w", p, err)
		}
		SLog(h.logger).Debug(LogEventCleanupDeletedFile).
			String("path", p).
			Log()
	}
	return nil
}

func (h *CleanupConfigHandler) runTruncateFiles(workDir string, params map[string]any) error {
	path := strParam(params, "path")
	keepLines := intParam(params, "keep_last_lines")
	if path == emptyValue {
		return errfmt.Errorf("truncate_files step requires 'path'")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	if keepLines <= 0 {
		return errfmt.Errorf("truncate_files requires keep_last_lines > 0")
	}
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) <= keepLines {
		return nil
	}
	keep := lines[len(lines)-keepLines:]
	if err := fileutil.WriteFile(path, []byte(strings.Join(keep, "\n")), paths.FilePerm600); err != nil {
		return err
	}
	SLog(h.logger).Debug(LogEventCleanupTruncatedFile).
		String("path", path).
		Int("kept_lines", keepLines).
		Log()
	return nil
}

func intParam(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// parseDuration parses durations like 7d, 24h, 1h30m. "d" = 24h.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return 0, errfmt.Errorf("empty duration")
	}
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil && n >= 0 {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}

func (h *CleanupConfigHandler) runReapStaleLocks(workDir string, params map[string]any) error {
	thresholdStr := strParam(params, "older_than")
	if thresholdStr == emptyValue {
		thresholdStr = strParam(params, "threshold")
	}
	threshold := 15 * time.Minute
	if thresholdStr != emptyValue {
		if d, err := parseDuration(thresholdStr); err == nil {
			threshold = d
		}
	}
	cnt, _, err := resourcehygiene.ReapStaleLocks(workDir, threshold, false)
	if err != nil {
		return err
	}
	if cnt > 0 {
		SLog(h.logger).Info("cleanup_reaped_stale_locks").
			Int("count", cnt).
			Log()
	}
	return nil
}

func (h *CleanupConfigHandler) runReapTempFiles(workDir string, params map[string]any) error {
	thresholdStr := strParam(params, "older_than")
	if thresholdStr == emptyValue {
		thresholdStr = strParam(params, "threshold")
	}
	threshold := 30 * time.Minute
	if thresholdStr != emptyValue {
		if d, err := parseDuration(thresholdStr); err == nil {
			threshold = d
		}
	}
	cnt, bytes, _, err := resourcehygiene.ReapOrphanedTempFiles(workDir, threshold, false)
	if err != nil {
		return err
	}
	if cnt > 0 {
		SLog(h.logger).Info("cleanup_reaped_temp_files").
			Int("count", cnt).
			Int("bytes", int(bytes)).
			Log()
	}
	return nil
}

func (h *CleanupConfigHandler) runEnforceLogRetention(workDir string, params map[string]any) error {
	maxAgeStr := strParam(params, "max_age")
	maxAge := 48 * time.Hour
	if maxAgeStr != emptyValue {
		if d, err := parseDuration(maxAgeStr); err == nil {
			maxAge = d
		}
	}
	maxSize := int64(10 * 1024 * 1024)
	if s := intParam(params, "max_size_bytes"); s > 0 {
		maxSize = int64(s)
	}
	cnt, bytes, _, err := resourcehygiene.EnforceLogRetention(workDir, maxAge, maxSize, false)
	if err != nil {
		return err
	}
	if cnt > 0 {
		SLog(h.logger).Info("cleanup_enforced_log_retention").
			Int("count", cnt).
			Int("bytes", int(bytes)).
			Log()
	}
	return nil
}

func (h *CleanupConfigHandler) runReapOrphanedProcesses(workDir string, params map[string]any) error {
	cnt, _, err := resourcehygiene.ReapOrphanedProcesses(workDir, false)
	if err != nil {
		return err
	}
	if cnt > 0 {
		SLog(h.logger).Info("cleanup_reaped_orphaned_processes").
			Int("count", cnt).
			Log()
	}
	return nil
}
