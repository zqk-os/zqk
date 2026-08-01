// Config-driven cleanup handler: single input (cleanup config YAML) → execute steps in order.
// See docs/architecture/CLEANUP_MAINTENANCE_DESIGN.md and pkg/cleanup.
package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/cleanup"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

// Log events for cleanup (config-driven) handler (POLICY-CODE-007 stable keys).
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
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
		cmd := exec.CommandContext(ctx, "make", args...)
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
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			continue
		}
		if cutoff != nil {
			info, errStat := os.Stat(p)
			if errStat != nil {
				continue
			}
			if info.ModTime().After(*cutoff) {
				continue
			}
		}
		if err := os.Remove(p); err != nil {
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
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) <= keepLines {
		return nil
	}
	keep := lines[len(lines)-keepLines:]
	if err := os.WriteFile(path, []byte(strings.Join(keep, "\n")), paths.FilePerm600); err != nil {
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
