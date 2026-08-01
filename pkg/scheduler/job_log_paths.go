package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/paths"
)

// maxSchedulerJobLogFilenameBytes caps a single path component length so logs work on common filesystems
// (e.g. NAME_MAX) and stay manageable in UIs.
const maxSchedulerJobLogFilenameBytes = 200

// TestBundleJobIDPrefix is the prefix for scan-tests run_wrapper job IDs (SCH-run-<bundle>).
// Jobs with this prefix use a single shared folder to avoid one directory per job and reduce lock contention.
const TestBundleJobIDPrefix = "SCH-run-"

// IsTestBundleJob returns true when jobID is a test-bundle job (e.g. from scan-tests).
// Such jobs use JobLogsTestBundlesDir so all test bundle logs live in one folder.
func IsTestBundleJob(jobID string) bool {
	return strings.HasPrefix(jobID, TestBundleJobIDPrefix)
}

// IsChurnStyleSchedulerJobID reports true for legacy one-off validation-style IDs:
// SCH-<unix_seconds>-<kind-with-dashes>-<object-id> (anything after the first dash following the timestamp).
// Examples: SCH-1771968798-zqk-session-ZQK-028, SCH-1771968703-scheduler-job-...-BST-102.
// Excludes SCH-<digits> only (e.g. scheduler submit), SCH-val, SCH-007, SCH-run-*, etc.
func IsChurnStyleSchedulerJobID(jobID string) bool {
	if jobID == emptyValue || IsTestBundleJob(jobID) {
		return false
	}
	if !strings.HasPrefix(jobID, "SCH-") {
		return false
	}
	rest := jobID[len("SCH-"):]
	idx := strings.IndexByte(rest, '-')
	if idx <= 0 {
		return false
	}
	ts := rest[:idx]
	for _, c := range ts {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(rest) > idx+1
}

// JobLogFileStem returns a filesystem-safe stem for log files (events, stdout, stderr).
// Churn-style IDs (SCH-<unix>-<kind>-<object-id>) always use a short stem SCH-<unix>-<8-hex>
// so nested names like scheduler-job-SCH-...-backlog-item-ITEM-102 do not become huge path
// components; the full job_id is still recorded inside JSONL lines.
// Other IDs: unchanged if len <= maxSchedulerJobLogFilenameBytes; otherwise SCH-LONG-<hash>.
func JobLogFileStem(jobID string) string {
	if jobID == emptyValue {
		return "unknown"
	}
	if IsChurnStyleSchedulerJobID(jobID) {
		return churnStyleJobLogFileStem(jobID)
	}
	if len(jobID) <= maxSchedulerJobLogFilenameBytes {
		return jobID
	}
	h := sha256.Sum256([]byte(jobID))
	return fmt.Sprintf("SCH-LONG-%s", hex.EncodeToString(h[:8]))
}

// churnFallbackLogStem is used when a churn-style ID cannot be parsed (missing SCH- prefix,
// no dash after timestamp, or non-numeric timestamp segment).
func churnFallbackLogStem(jobID string) string {
	h := sha256.Sum256([]byte(jobID))
	return fmt.Sprintf("SCH-CHURN-%s", hex.EncodeToString(h[:6]))
}

// churnStyleJobLogFileStem maps a churn-style job ID to a short deterministic filename stem.
func churnStyleJobLogFileStem(jobID string) string {
	const prefix = "SCH-"
	if !strings.HasPrefix(jobID, prefix) {
		return churnFallbackLogStem(jobID)
	}
	rest := jobID[len(prefix):]
	idx := strings.IndexByte(rest, '-')
	if idx <= 0 {
		return churnFallbackLogStem(jobID)
	}
	ts := rest[:idx]
	for _, c := range ts {
		if c < '0' || c > '9' {
			return churnFallbackLogStem(jobID)
		}
	}
	h := sha256.Sum256([]byte(jobID))
	short := hex.EncodeToString(h[:4])
	return fmt.Sprintf("SCH-%s-%s", ts, short)
}

// JobLogDir returns the directory for a job's log files.
// For test-bundle jobs (SCH-run-*): .zqk/logs/scheduler/cvs/test-bundles/
// For churn-style one-off IDs: .zqk/logs/scheduler/churn-runs/ (shared; files named per job stem)
// For other jobs: .zqk/logs/scheduler/<jobID>/ (or a hashed dirname if jobID is extremely long).
func JobLogDir(projectRoot, jobID string) string {
	if IsTestBundleJob(jobID) {
		return JobLogsTestBundlesDir(projectRoot)
	}
	if IsChurnStyleSchedulerJobID(jobID) {
		return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, paths.SchedulerChurnRunsSubdir)
	}
	dirName := jobID
	if len(dirName) > maxSchedulerJobLogFilenameBytes {
		dirName = JobLogFileStem(jobID)
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, dirName)
}

var legacySchedulerTestBundlesMigrateMu sync.Mutex

// JobLogsCVSDir returns .zqk/logs/scheduler/cvs/ — CONV-level JSONL (measurement handoff) and the parent of test-bundle logs.
// On first access, migrates legacy .zqk/logs/scheduler/test-bundles/ to cvs/test-bundles/ when the
// canonical dir is absent; when both dirs exist (partial upgrade), merges loose files from the
// legacy flat dir into cvs/test-bundles/ and removes the legacy dir if empty.
func JobLogsCVSDir(projectRoot string) string {
	base := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir)
	newCVSRoot := filepath.Join(base, paths.SchedulerCVSSubdir)
	maybeMigrateLegacySchedulerTestBundlesDir(base, newCVSRoot)
	return newCVSRoot
}

// JobLogsTestBundlesDir returns the single directory for all test-bundle job logs: .zqk/logs/scheduler/cvs/test-bundles/
// Use when writing test bundle logs (events, stdout, stderr) or the go test redirect so everything stays in one folder.
func JobLogsTestBundlesDir(projectRoot string) string {
	return filepath.Join(JobLogsCVSDir(projectRoot), paths.SchedulerTestBundlesSubdir)
}

func maybeMigrateLegacySchedulerTestBundlesDir(base, newCVSRoot string) {
	legacySchedulerTestBundlesMigrateMu.Lock()
	defer legacySchedulerTestBundlesMigrateMu.Unlock()

	newTB := filepath.Join(newCVSRoot, paths.SchedulerTestBundlesSubdir)
	// Legacy flat dir (pre-cvs): .zqk/logs/scheduler/test-bundles/
	oldTB := filepath.Join(base, paths.SchedulerTestBundlesSubdir)

	if _, err := os.Stat(newTB); err == nil {
		maybeMigrateCVSMeasurementFileFromTestBundles(newCVSRoot, newTB)
		mergeLegacyFlatTestBundleFilesIntoCanonical(oldTB, newTB)
		return
	}
	if _, err := os.Stat(oldTB); err != nil {
		return
	}
	_ = os.MkdirAll(newCVSRoot, paths.DirPerm755)
	if err := os.Rename(oldTB, newTB); err != nil {
		return
	}
	maybeMigrateCVSMeasurementFileFromTestBundles(newCVSRoot, newTB)
}

// legacyFlatSchedulerTestBundlesDir is the pre-namespace layout:
// .zqk/logs/scheduler/test-bundles/ (same as JobLogsTestBundlesDir but without …/cvs/).
func legacyFlatSchedulerTestBundlesDir(projectRoot string) string {
	base := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir)
	return filepath.Join(base, paths.SchedulerTestBundlesSubdir)
}

// mergeLegacyFlatTestBundleFilesIntoCanonical moves loose files from the legacy flat dir into
// cvs/test-bundles/ when both exist (migration v1 only renamed the whole dir when cvs was absent).
func mergeLegacyFlatTestBundleFilesIntoCanonical(legacyFlatDir, canonicalDir string) {
	entries, err := os.ReadDir(legacyFlatDir)
	if err != nil {
		return
	}
	_ = os.MkdirAll(canonicalDir, paths.DirPerm755)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		oldPath := filepath.Join(legacyFlatDir, e.Name())
		newPath := filepath.Join(canonicalDir, e.Name())
		if _, err := os.Stat(newPath); err == nil {
			continue
		}
		_ = os.Rename(oldPath, newPath)
	}
	entries2, err := os.ReadDir(legacyFlatDir)
	if err != nil || len(entries2) > 0 {
		return
	}
	_ = os.Remove(legacyFlatDir)
}

// NormalizeTestBundleRedirectLogPath maps a persisted run_wrapper redirect path that still points at
// the legacy flat .zqk/logs/scheduler/test-bundles/ tree to JobLogsTestBundlesDir (…/cvs/test-bundles/).
// Scheduler jobs created before the cvs/ layout may embed the old absolute path; without this,
// copyStreamFilesToRedirectTarget would recreate the legacy directory on every run.
func NormalizeTestBundleRedirectLogPath(projectRoot, absTarget string) string {
	if projectRoot == "" || absTarget == "" {
		return absTarget
	}
	legacy := legacyFlatSchedulerTestBundlesDir(projectRoot)
	canonical := JobLogsTestBundlesDir(projectRoot)
	t := filepath.Clean(absTarget)
	leg := filepath.Clean(legacy)
	rel, err := filepath.Rel(leg, t)
	if err != nil || strings.HasPrefix(rel, "..") {
		return absTarget
	}
	return filepath.Join(canonical, rel)
}

func maybeMigrateCVSMeasurementFileFromTestBundles(cvsRoot, testBundlesDir string) {
	oldFile := filepath.Join(testBundlesDir, "cvs_measurement_events.jsonl")
	newFile := filepath.Join(cvsRoot, "cvs_measurement_events.jsonl")
	if _, err := os.Stat(newFile); err == nil {
		return
	}
	if _, err := os.Stat(oldFile); err != nil {
		return
	}
	_ = os.Rename(oldFile, newFile)
}

// JobLogsBaseDir returns the base directory for all scheduler job logs: .zqk/logs/scheduler/
func JobLogsBaseDir(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir)
}

// JobStdoutFilePath returns the path to the job's streaming stdout capture file.
func JobStdoutFilePath(projectRoot, jobID string) string {
	return filepath.Join(JobLogDir(projectRoot, jobID), JobLogFileStem(jobID)+".stdout")
}

// JobStderrFilePath returns the path to the job's streaming stderr capture file.
func JobStderrFilePath(projectRoot, jobID string) string {
	return filepath.Join(JobLogDir(projectRoot, jobID), JobLogFileStem(jobID)+".stderr")
}

// JobEventsFilePath returns the per-job JSONL events file path.
// Legacy compatibility: if <stem>.events.json exists and .events.jsonl does not, it is renamed.
func JobEventsFilePath(projectRoot, jobID string) string {
	logDir := JobLogDir(projectRoot, jobID)
	stem := JobLogFileStem(jobID)
	newPath := filepath.Join(logDir, stem+".events.jsonl")
	legacyPath := filepath.Join(logDir, stem+".events.json")
	_ = migrateLegacyPath(legacyPath, newPath)
	// Older layouts used the raw job ID as the filename stem inside the per-job directory.
	if stem != jobID {
		legacyStemPath := filepath.Join(logDir, jobID+".events.jsonl")
		_ = migrateLegacyPath(legacyStemPath, newPath)
	}
	return newPath
}

// TestBundlesEventsFilePath returns the shared test-bundle JSONL events file path.
// Legacy compatibility: if events.json exists and events.jsonl does not, it is renamed.
func TestBundlesEventsFilePath(projectRoot string) string {
	dir := JobLogsTestBundlesDir(projectRoot)
	newPath := filepath.Join(dir, "events.jsonl")
	legacyPath := filepath.Join(dir, "events.json")
	_ = migrateLegacyPath(legacyPath, newPath)
	return newPath
}

// TestBundlesHealthFilePath returns the rolling health timeline for test-bundle runs (pass/fail + rerun hints).
func TestBundlesHealthFilePath(projectRoot string) string {
	return filepath.Join(JobLogsTestBundlesDir(projectRoot), "health.jsonl")
}

// TestBundlesProgressFilePath returns the path to the live progress JSONL file for test-bundle runs.
func TestBundlesProgressFilePath(projectRoot string) string {
	return filepath.Join(JobLogsTestBundlesDir(projectRoot), "progress.jsonl")
}

// CVSMeasurementEventsFilePath returns JSONL for convergence_session_tick "measure applied" signals (IDE watches,
// agent handoff hints). Best-effort append-only; see AppendCVSMeasurementEvent.
// Path: .zqk/logs/scheduler/cvs/cvs_measurement_events.jsonl (not under test-bundles/).
func CVSMeasurementEventsFilePath(projectRoot string) string {
	return filepath.Join(JobLogsCVSDir(projectRoot), "cvs_measurement_events.jsonl")
}

func migrateLegacyPath(legacyPath, newPath string) error {
	if legacyPath == emptyValue || newPath == emptyValue || legacyPath == newPath {
		return nil
	}
	if _, err := os.Stat(legacyPath); err != nil {
		return nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return nil
	}
	return os.Rename(legacyPath, newPath)
}
