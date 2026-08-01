package scheduler

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// IssueBundleHealthRow is one row comparing an issues.json entry to test-bundles/health.jsonl.
type IssueBundleHealthRow struct {
	JobID           string
	JobType         string
	IssueRecordedAt string
	// ThatRunOutcome is the test_outcome from the health line whose job_id matches, if any.
	ThatRunOutcome string
	ThatRunAt      string
	// Fingerprint is from the matching health line (same bundle as that job run).
	Fingerprint string
	// LatestForBundle is the latest test_outcome for that fingerprint in the scanned health window (last line wins).
	LatestForBundle string
	Interpretation  string
}

// IssuesBundleHealthReport is the result of CompareIssuesToBundleHealth.
type IssuesBundleHealthReport struct {
	// HealthFileMissing is true when health.jsonl does not exist (no bundle runs recorded yet).
	HealthFileMissing  bool
	HealthLinesScanned int
	HealthMaxLines     int
	Rows               []IssueBundleHealthRow
}

// CompareIssuesToBundleHealth cross-checks .zqk/scheduler/issues.json against
// .zqk/logs/scheduler/cvs/test-bundles/health.jsonl.
//
// For each issue whose job_id is a test-bundle id (SCH-run-*), it reports:
//   - the outcome recorded for that specific job_id in the health tail (if present);
//   - the latest outcome for the same bundle_command_fingerprint, so re-runs that used a new job_id
//     still show whether the bundle is green now.
//
// Non-SCH-run issues get a short note that this comparison does not apply.
func CompareIssuesToBundleHealth(projectRoot string, healthMaxLines int) (*IssuesBundleHealthReport, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root required")
	}
	payload, err := ReadIssues(projectRoot)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		payload = &IssuesPayload{Status: issuesStatusOK}
	}
	if healthMaxLines <= 0 {
		healthMaxLines = defaultHealthLinesCapForIssuesCompare
	}

	lines, healthMissing, err := readTestBundleHealthJSONLFullScan(projectRoot, healthMaxLines)
	if err != nil {
		return nil, err
	}

	lastByJobID := make(map[string]map[string]any)
	lastOutcomeByFP := make(map[string]string)
	for _, m := range lines {
		if jid, _ := m[KeyJobID].(string); jid != emptyValue {
			lastByJobID[jid] = m
		}
		fp, _ := m[KeyBundleCommandFingerprint].(string)
		if fp == emptyValue {
			continue
		}
		if o, _ := m[KeyTestOutcome].(string); o != emptyValue {
			lastOutcomeByFP[fp] = o
		}
	}

	out := &IssuesBundleHealthReport{
		HealthFileMissing:  healthMissing,
		HealthLinesScanned: len(lines),
		HealthMaxLines:     healthMaxLines,
	}
	for _, iss := range payload.Issues {
		row := IssueBundleHealthRow{
			JobID:           iss.JobID,
			JobType:         iss.JobType,
			IssueRecordedAt: iss.At,
		}
		if !IsTestBundleJob(iss.JobID) {
			row.Interpretation = "not a SCH-run test-bundle job; use scheduler history/activity or logs for this id"
			out.Rows = append(out.Rows, row)
			continue
		}
		hm := lastByJobID[iss.JobID]
		if hm == nil {
			if healthMissing {
				row.Interpretation = "cvs/test-bundles/health.jsonl missing; no bundle health data yet"
			} else {
				row.Interpretation = "no health.jsonl row for this job_id in the scanned file (predates health log, different job id for same bundle, or log rotation)"
			}
			out.Rows = append(out.Rows, row)
			continue
		}
		row.ThatRunOutcome, _ = hm[KeyTestOutcome].(string)
		row.ThatRunAt, _ = hm[KeyTimestamp].(string)
		row.Fingerprint, _ = hm[KeyBundleCommandFingerprint].(string)

		if row.Fingerprint != emptyValue {
			row.LatestForBundle = lastOutcomeByFP[row.Fingerprint]
		}
		row.Interpretation = interpretIssueVsBundle(row.ThatRunOutcome, row.LatestForBundle)
		out.Rows = append(out.Rows, row)
	}

	return out, nil
}

func interpretIssueVsBundle(thatRun, latest string) string {
	thatRun = strings.TrimSpace(thatRun)
	latest = strings.TrimSpace(latest)
	if latest == emptyValue {
		return "could not resolve latest outcome for bundle fingerprint"
	}
	bad := outcomeIsBad(latest)
	good := outcomeIsGood(latest)
	if good && outcomeIsBad(thatRun) {
		return "that run failed, but latest bundle outcome for this fingerprint is green (issues.json may be stale — consider clear-issues or wait for age-out)"
	}
	if bad {
		return "latest bundle outcome for this fingerprint is still failing"
	}
	if good && (thatRun == emptyValue || outcomeIsGood(thatRun)) {
		return "latest bundle outcome for this fingerprint is green"
	}
	return "mixed or unknown; compare that_run vs latest manually"
}

// defaultHealthLinesCapForIssuesCompare bounds how many parsed rows we read from health.jsonl when
// cross-checking issues (full file scan, oldest→newest, so old SCH-run job_ids resolve).
const defaultHealthLinesCapForIssuesCompare = 100000
