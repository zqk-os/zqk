package scheduler

import (
	"context"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/shirou/gopsutil/v3/process"
)

// HourglassCleanupHandler enforces the Hourglass protocol by terminating stalled agent builds natively.
type HourglassCleanupHandler struct {
	logger logging.Logger
	slog   *SchedulerLogRoot
}

// NewHourglassCleanupHandler creates a new handler.
func NewHourglassCleanupHandler() *HourglassCleanupHandler {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	return &HourglassCleanupHandler{
		logger: logger,
		slog:   SLog(logger),
	}
}

func (h *HourglassCleanupHandler) schedulerLog() *SchedulerLogRoot {
	if h.slog != nil {
		return h.slog
	}
	return SLog(h.logger)
}

func (h *HourglassCleanupHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	h.schedulerLog().Info("hourglass_cleanup_job_start").
		JobID(job.ID).
		Log()

	procs, err := process.Processes()
	if err != nil {
		h.schedulerLog().Error("hourglass_cleanup_proc_list_failed", err).Log()
		return err
	}

	cutoffMinutes := 5
	targetSubstrings := []string{"go test", "mcp.test"}

	if job != nil && job.EnvironmentVariables != nil {
		if val, ok := job.EnvironmentVariables["HOURGLASS_CUTOFF_MINUTES"]; ok && val != emptyValue {
			if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
				cutoffMinutes = parsed
			}
		}
		if val, ok := job.EnvironmentVariables["HOURGLASS_TARGET_SUBSTRINGS"]; ok && val != emptyValue {
			parts := strings.Split(val, ",")
			targetSubstrings = make([]string, 0, len(parts))
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					targetSubstrings = append(targetSubstrings, trimmed)
				}
			}
		}
	}

	cutoff := time.Now().Add(-time.Duration(cutoffMinutes) * time.Minute)
	killed := 0

	for _, p := range procs {
		cmdline, err := p.Cmdline()
		if err != nil || cmdline == "" {
			continue
		}

		isTarget := false
		for _, sub := range targetSubstrings {
			if strings.Contains(cmdline, sub) {
				isTarget = true
				break
			}
		}

		if isTarget {
			createTime, err := p.CreateTime()
			if err != nil {
				continue
			}

			startTime := time.UnixMilli(createTime)
			if startTime.Before(cutoff) {
				// Time to reap
				h.schedulerLog().Info("hourglass_cleanup_killing_stalled_process").
					Int("pid", int(p.Pid)).
					String("cmdline", cmdline).
					Log()

				// Terminate hanging process
				err = p.Kill()
				if err == nil {
					killed++
				}
			}
		}
	}

	h.schedulerLog().Info("hourglass_cleanup_job_completed").
		JobID(job.ID).
		Int("killed_processes", killed).
		Log()

	return nil
}
