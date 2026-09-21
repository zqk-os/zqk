package scheduler

import (
	"bufio"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// gatherHealthSignals reads kernel health signals from the scheduler's health log
// and other observable sources, returning a populated KernelHealthSignals struct.
// It is designed to be fast and non-blocking — errors are silently ignored so the
// sentinel can always produce a prompt even if health data is unavailable.
func gatherHealthSignals(projectRoot string) agentprompt.KernelHealthSignals {
	var signals agentprompt.KernelHealthSignals

	// Read health.jsonl from the scheduler state directory.
	healthPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerSubdir, "health.jsonl")
	f, err := fileutil.Open(healthPath)
	if err != nil {
		// File absent is normal; return zero signals.
		return signals
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	driftSeen := map[string]bool{}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		level, _ := entry["level"].(string)
		event, _ := entry["event"].(string)

		// Count test failures.
		if level == "error" && event == "test_failure" {
			signals.FailingTestCount++
		}

		// Collect unique drift indicators from error messages.
		if level == "error" {
			msg, _ := entry["message"].(string)
			if msg == "" {
				msg, _ = entry["msg"].(string)
			}
			if msg != "" && !driftSeen[msg] {
				indicator := buildDriftIndicator(event, msg)
				if indicator != "" {
					driftSeen[msg] = true
					signals.DriftIndicators = append(signals.DriftIndicators, indicator)
				}
			}
		}
	}

	return signals
}

// buildDriftIndicator formats a log entry into a human-readable drift indicator.
// Returns empty string for non-actionable entries.
func buildDriftIndicator(event, msg string) string {
	// Skip generic noise.
	if strings.Contains(msg, "context canceled") || strings.Contains(msg, "signal:") {
		return ""
	}
	if event != "" {
		return event + ": " + msg
	}
	return msg
}
