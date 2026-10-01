// Package closureevidence implements fail-closed machine-checkable evidence validation
// for object completion (scheduler job ID, test bundle log path, and re-read green fingerprint in health.jsonl).
package closureevidence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Evidence holds machine-checkable verification references.
type Evidence struct {
	JobID       string `json:"job_id,omitempty"`
	LogPath     string `json:"log_path,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// HealthRecord represents a single entry in .zqk/logs/scheduler/cvs/test-bundles/health.jsonl.
type HealthRecord struct {
	BundleCommandFingerprint string `json:"bundle_command_fingerprint"`
	EventType                string `json:"event_type"`
	JobID                    string `json:"job_id"`
	LogPath                  string `json:"log_path"`
	Package                  string `json:"package"`
	TestOutcome              string `json:"test_outcome"`
	TestsFailed              int    `json:"tests_failed"`
	Timestamp                string `json:"timestamp"`
}

// ExtractEvidence pulls machine-checkable evidence references from an object map.
func ExtractEvidence(obj map[string]any) (Evidence, bool) {
	if obj == nil {
		return Evidence{}, false
	}
	var ev Evidence
	if raw, ok := obj["evidence"].(map[string]any); ok {
		ev.JobID, _ = raw["job_id"].(string)
		ev.LogPath, _ = raw["log_path"].(string)
		ev.Fingerprint, _ = raw["fingerprint"].(string)
		if ev.Fingerprint == "" {
			ev.Fingerprint, _ = raw["bundle_command_fingerprint"].(string)
		}
	}
	if ev.JobID == "" {
		ev.JobID, _ = obj["job_id"].(string)
	}
	if ev.LogPath == "" {
		ev.LogPath, _ = obj["bundle_log_path"].(string)
		if ev.LogPath == "" {
			ev.LogPath, _ = obj["log_path"].(string)
		}
	}
	if ev.Fingerprint == "" {
		ev.Fingerprint, _ = obj["bundle_command_fingerprint"].(string)
		if ev.Fingerprint == "" {
			ev.Fingerprint, _ = obj["fingerprint"].(string)
		}
	}

	hasAny := strings.TrimSpace(ev.JobID) != "" || strings.TrimSpace(ev.LogPath) != "" || strings.TrimSpace(ev.Fingerprint) != ""
	return ev, hasAny
}

// ValidateEvidence verifies that the cited job id, bundle log, and fingerprint are valid,
// present on disk, and match a green ("pass" outcome with 0 test failures) record in health.jsonl.
func ValidateEvidence(repoRoot string, ev Evidence) error {
	if zqkenv.TestBypassGitevidence().Get() == "1" {
		return nil
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return fmt.Errorf("repo root is required for closure evidence validation")
	}

	jobID := strings.TrimSpace(ev.JobID)
	logPath := strings.TrimSpace(ev.LogPath)
	fingerprint := strings.TrimSpace(ev.Fingerprint)

	if jobID == "" {
		return fmt.Errorf("closure evidence requires a non-empty scheduler job_id")
	}
	if logPath == "" {
		return fmt.Errorf("closure evidence requires a non-empty bundle log_path")
	}
	if fingerprint == "" {
		return fmt.Errorf("closure evidence requires a non-empty bundle_command_fingerprint")
	}

	// 1. Check bundle log on disk
	resolvedLogPath := logPath
	if !filepath.IsAbs(resolvedLogPath) {
		resolvedLogPath = filepath.Join(repoRoot, resolvedLogPath)
	}
	fi, err := fileutil.Stat(resolvedLogPath)
	if err != nil {
		return fmt.Errorf("bundle log %q not found on disk: %w", logPath, err)
	}
	if fi.Size() == 0 {
		return fmt.Errorf("bundle log %q is empty", logPath)
	}

	// 2. Read and verify health.jsonl
	healthPath := filepath.Join(repoRoot, paths.ProjectDataDir, paths.SchedulerLogsDir, paths.SchedulerCVSSubdir, paths.SchedulerTestBundlesSubdir, "health.jsonl")
	healthFile, err := fileutil.Open(healthPath)
	if err != nil {
		return fmt.Errorf("cannot read health.jsonl at %s: %w", healthPath, err)
	}
	defer healthFile.Close()

	var matchingRecords []HealthRecord
	scanner := bufio.NewScanner(healthFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec HealthRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.BundleCommandFingerprint == fingerprint || (rec.JobID != "" && rec.JobID == jobID) {
			matchingRecords = append(matchingRecords, rec)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading health.jsonl: %w", err)
	}

	if len(matchingRecords) == 0 {
		return fmt.Errorf("fingerprint %q (job %q) not found in health.jsonl", fingerprint, jobID)
	}

	// The latest record for this fingerprint must be green
	latest := matchingRecords[len(matchingRecords)-1]
	if latest.TestOutcome != "pass" || latest.TestsFailed > 0 {
		return fmt.Errorf("fingerprint %q latest outcome is %q with %d failed tests (must be green pass)", fingerprint, latest.TestOutcome, latest.TestsFailed)
	}

	return nil
}

// ValidateObjectClosureEvidence validates machine-checkable closure evidence for obj.
func ValidateObjectClosureEvidence(repoRoot string, obj map[string]any) error {
	ev, has := ExtractEvidence(obj)
	if !has {
		return fmt.Errorf("object lacks machine-checkable closure evidence (job_id, bundle_log_path, bundle_command_fingerprint)")
	}
	return ValidateEvidence(repoRoot, ev)
}
