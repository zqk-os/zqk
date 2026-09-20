// Package scheduler: proactive issue detector.
// Writes .zqk/scheduler/issues.json when there are problems (job failures, timeouts) or routine maintenance items
// (e.g. test bundle ran successfully but some tests failed). The latter are not logged as errors; they are
// appended here so they can be worked in between priority plan issues and categorized as maintenance.
// When healthy (no recent issues), file is status "ok" or absent so logs stay quiet.

package scheduler

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	issuesFileName     = "issues.json"
	issuesMaxEntries   = 20
	issuesClearAfter   = 1 * time.Hour // clear "issues" status when oldest issue is older than this
	issuesStatusOK     = "ok"
	issuesStatusIssues = "issues"
)

// Issue is a single reported problem (e.g. job failure or timeout).
type Issue struct {
	JobID   string `json:"job_id"`
	JobType string `json:"job_type"`
	Error   string `json:"error"`
	At      string `json:"at"` // RFC3339
}

// IssuesPayload is the JSON written to issues.json.
type IssuesPayload struct {
	Status    string  `json:"status"`
	UpdatedAt string  `json:"updated_at"`
	Issues    []Issue `json:"issues,omitempty"`
}

var issuesMu sync.Mutex

// issuesFilePath returns the path to .zqk/scheduler/issues.json.
func issuesFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, issuesFileName)
}

// ReportIssue records a job failure or timeout so it is visible in .zqk/scheduler/issues.json.
// Call this when a job fails or times out. Safe to call from any goroutine.
func ReportIssue(projectRoot, jobID, jobType, errMsg string) {
	if projectRoot == emptyValue || jobID == emptyValue {
		return
	}
	issuesMu.Lock()
	defer issuesMu.Unlock()

	path := issuesFilePath(projectRoot)
	dir := filepath.Dir(path)
	if err := fileutil.EnsureDir(dir); err != nil {
		return
	}

	now := time.Now().UTC()
	entry := Issue{JobID: jobID, JobType: jobType, Error: errMsg, At: now.Format(time.RFC3339)}

	var payload IssuesPayload
	if data, err := fileutil.ReadFile(path); err == nil {
		if errUnmarshal := json.Unmarshal(data, &payload); errUnmarshal != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to unmarshal existing issues file").WithError(errUnmarshal).Log()
		}
	}
	if payload.Issues == nil {
		payload.Issues = []Issue{}
	}
	payload.Issues = append(payload.Issues, entry)
	if len(payload.Issues) > issuesMaxEntries {
		payload.Issues = payload.Issues[len(payload.Issues)-issuesMaxEntries:]
	}
	payload.Status = issuesStatusIssues
	payload.UpdatedAt = now.Format(time.RFC3339)

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	if errWrite := fileutil.WriteSecureFile(path, data); errWrite != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to write issues file").WithError(errWrite).Log()
	}
}

// ClearIssuesIfOk writes status "ok" to issues.json when all recorded issues are older than issuesClearAfter.
// Call this from the health-monitoring path so that after a period with no new failures, the file shows ok again.
func ClearIssuesIfOk(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	issuesMu.Lock()
	defer issuesMu.Unlock()

	path := issuesFilePath(projectRoot)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return // already absent = ok
		}
		return
	}
	var payload IssuesPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	if payload.Status != issuesStatusIssues || len(payload.Issues) == 0 {
		return
	}
	now := time.Now().UTC()
	cutoff := now.Add(-issuesClearAfter)

	healthReport, _ := compareIssuesToBundleHealthUnsafe(projectRoot, 0, &payload)

	var remaining []Issue
	for _, i := range payload.Issues {
		t, err := time.Parse(time.RFC3339, i.At)
		isOld := err == nil && t.Before(cutoff)

		isGreen := false
		if healthReport != nil {
			for _, row := range healthReport.Rows {
				if row.JobID == i.JobID && row.IssueRecordedAt == i.At {
					if strings.Contains(row.Interpretation, "latest bundle outcome for this fingerprint is green") {
						isGreen = true
					}
					break
				}
			}
		}

		if !isOld && !isGreen {
			remaining = append(remaining, i)
		}
	}

	if len(remaining) == len(payload.Issues) {
		return // nothing cleared
	}

	if len(remaining) == 0 {
		payload.Status = issuesStatusOK
		payload.UpdatedAt = now.Format(time.RFC3339)
		payload.Issues = nil
	} else {
		payload.Status = issuesStatusIssues
		payload.UpdatedAt = now.Format(time.RFC3339)
		payload.Issues = remaining
	}

	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	if errWrite := fileutil.WriteSecureFile(path, out); errWrite != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to clear issues file").WithError(errWrite).Log()
	}
}

// ReadIssues reads the current issues payload from .zqk/scheduler/issues.json.
// Returns (nil, nil) if the file is missing or empty/ok. Safe to call from any goroutine.
func ReadIssues(projectRoot string) (*IssuesPayload, error) {
	if projectRoot == emptyValue {
		return nil, nil
	}
	issuesMu.Lock()
	defer issuesMu.Unlock()

	path := issuesFilePath(projectRoot)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return &IssuesPayload{Status: issuesStatusOK, UpdatedAt: zqktime.NowRFC3339UTC()}, nil
		}
		return nil, err
	}
	var payload IssuesPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, errfmt.Newf("parse issues file").Wrap(err)
	}
	return &payload, nil
}

// ClearIssues writes status "ok" to issues.json, clearing all recorded issues.
// projectRoot is normalized to an absolute path so the file is always written under the intended directory.
// Returns the absolute path that was written so callers can verify (e.g. CLI prints it).
func ClearIssues(projectRoot string) (writtenPath string, err error) {
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root is required")
	}
	absRoot, absErr := filepath.Abs(projectRoot)
	if absErr != nil {
		return "", errfmt.Newf("failed to resolve project root").Wrap(absErr)
	}
	projectRoot = absRoot

	issuesMu.Lock()
	defer issuesMu.Unlock()

	path := issuesFilePath(projectRoot)
	dir := filepath.Dir(path)
	if err := fileutil.EnsureDir(dir); err != nil {
		return "", errfmt.Newf("failed to create scheduler directory").Wrap(err)
	}

	now := time.Now().UTC()
	payload := IssuesPayload{
		Status:    issuesStatusOK,
		UpdatedAt: now.Format(time.RFC3339),
		Issues:    nil,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", errfmt.Newf("failed to marshal issues payload").Wrap(err)
	}
	if err := fileutil.WriteSecureFile(path, data); err != nil {
		return "", errfmt.Newf("failed to write issues file").Wrap(err)
	}

	// Also clear/rotate test bundle health and progress log files when manually clearing issues
	healthPath := TestBundlesHealthFilePath(projectRoot)
	if errHealth := fileutil.Remove(healthPath); errHealth != nil && !fileutil.IsNotExist(errHealth) {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to remove health.jsonl file").WithError(errHealth).Log()
	}
	progressPath := TestBundlesProgressFilePath(projectRoot)
	if errProgress := fileutil.Remove(progressPath); errProgress != nil && !fileutil.IsNotExist(errProgress) {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to remove progress.jsonl file").WithError(errProgress).Log()
	}

	return path, nil
}
