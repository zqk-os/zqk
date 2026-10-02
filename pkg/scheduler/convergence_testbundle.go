package scheduler

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestBundleHealthJSONLMaxToken is the maximum JSONL line length for test-bundles/health.jsonl
// (entries can include long suggested_rerun_commands arrays).
const TestBundleHealthJSONLMaxToken = 1024 * 1024

// NewTestBundleHealthJSONLScanner returns a bufio.Scanner suitable for reading health.jsonl lines
// that may exceed the default scanner token limit.
func NewTestBundleHealthJSONLScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	buf := make([]byte, TestBundleHealthJSONLMaxToken)
	sc.Buffer(buf, TestBundleHealthJSONLMaxToken)
	return sc
}

// DefaultConvergenceHeartbeatStaleAfter is the default age after which health.jsonl is considered stale for automation.
const DefaultConvergenceHeartbeatStaleAfter = 15 * time.Minute

// TestBundleConvergenceSnapshot is structured state derived from test-bundles/health.jsonl for
// convergence_session fields (after_state_snapshot, delta_assessment, last_measurement_at) and
// for automation triggers (heartbeat, suggested reruns).
type TestBundleConvergenceSnapshot struct {
	HealthWatermarkRFC3339 string `json:"health_watermark_rfc3339"`
	LinesInWindow          int    `json:"lines_in_window"`
	DeltaAssessment        string `json:"delta_assessment"` // trending_toward | trending_away | neutral | unknown
	// HadFailureInWindow is true when some fingerprint has a bad outcome in the window with no later
	// good outcome for that same fingerprint (chronological). A superseded fail→pass clears it.
	HadFailureInWindow bool `json:"had_failure_in_window"`
	// FailingFingerprintsNow lists bundle_command_fingerprint values whose latest outcome is bad.
	FailingFingerprintsNow []string `json:"failing_fingerprints_now"`
	// FlakingFingerprintsNow lists bundle_command_fingerprint values whose latest outcome is a quarantined flake.
	FlakingFingerprintsNow []string `json:"flaking_fingerprints_now,omitempty"`
	// BuildFailingFingerprintsNow lists bundle_command_fingerprint values whose latest outcome is a build failure.
	BuildFailingFingerprintsNow []string `json:"build_failing_fingerprints_now,omitempty"`
	PassCount                   int      `json:"pass_count"`
	FlakeCount                  int      `json:"flake_count"`
	FailCount                   int      `json:"fail_count"`
	BuildFailCount              int      `json:"build_fail_count"`
	// FingerprintLatestOutcome maps fingerprint -> latest test_outcome in the window (last line wins per key).
	FingerprintLatestOutcome map[string]string `json:"fingerprint_latest_outcome"`
	// SuggestedRerunByFingerprint is populated for fingerprints that are still failing (latest line bad).
	SuggestedRerunByFingerprint map[string][]string   `json:"suggested_rerun_by_fingerprint,omitempty"`
	Heartbeat                   *ConvergenceHeartbeat `json:"heartbeat,omitempty"`
	NextActionHint              string                `json:"next_action_hint"`
	// TriggerQueuePending is the number of pending trigger-queue entries (scheduled work not yet started).
	// -1 if projectRoot was empty or the queue could not be read. Do not treat empty failing_fingerprints_now
	// as "all bundles done" while TriggerQueuePending > 0 or while jobs may still be executing (see zqk scheduler activity).
	TriggerQueuePending int `json:"trigger_queue_pending"`
	// ReadyForSessionCompletion is true only when health + queue gates say it is safe to mark a convergence_session complete.
	ReadyForSessionCompletion bool `json:"ready_for_session_completion"`
	// SessionCompletionBlockedReasons lists human-readable gates that blocked ReadyForSessionCompletion.
	SessionCompletionBlockedReasons []string `json:"session_completion_blocked_reasons,omitempty"`
	// SessionCompletionNote is set when ReadyForSessionCompletion is true (extra checks beyond health.jsonl).
	SessionCompletionNote string `json:"session_completion_note,omitempty"`
	// PrimaryMeasurementOutcome is the normative four-outcome taxonomy (MEASUREMENT_OUTCOME_TAXONOMY.md),
	// derived from the same inputs as rollup_status_core test-bundle slice with rollup gates skipped.
	PrimaryMeasurementOutcome       string `json:"primary_measurement_outcome,omitempty"`
	PrimaryMeasurementOutcomeDetail string `json:"primary_measurement_outcome_detail,omitempty"`
	MeasurementOutcomeSchemaVersion string `json:"measurement_outcome_schema_version,omitempty"`
}

// ConvergenceHeartbeat supports automated staleness checks against health.jsonl recency.
type ConvergenceHeartbeat struct {
	MeasurementAgeSeconds float64 `json:"measurement_age_seconds,omitempty"`
	Stale                 bool    `json:"stale"`
	StaleAfterSeconds     float64 `json:"stale_after_seconds,omitempty"`
}

// healthJSONLScanYieldInterval checks ctx between chunks of lines so long health.jsonl reads
// respect job max_runtime / cancellation. Tests may set to 1 for cheaper cancellation coverage.
var healthJSONLScanYieldInterval = 64

// testHookAfterHealthJSONLScanLine is set by package tests only: called after each scanner line
// number (including blank lines) so cancellation can be triggered deterministically mid-read.
var testHookAfterHealthJSONLScanLine func(lineNumber int)

// ReadTestBundleHealthTailLines reads up to maxLines non-empty JSON objects from the end of
// .zqk/logs/scheduler/cvs/test-bundles/health.jsonl. Lines that fail to parse are skipped.
// Pass a cancellable context (e.g. scheduler job execution context) so large files do not ignore timeouts.
func ReadTestBundleHealthTailLines(ctx context.Context, projectRoot string, maxLines int) ([]map[string]any, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root required")
	}
	path := TestBundlesHealthFilePath(projectRoot)
	return readTestBundleSharedJSONLTail(ctx, path, maxLines)
}

// ReadTestBundleEventsTailLines reads up to maxLines non-empty JSON objects from the end of
// test-bundles/events.jsonl (criteria_verification_evidence and other shared events).
func ReadTestBundleEventsTailLines(ctx context.Context, projectRoot string, maxLines int) ([]map[string]any, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root required")
	}
	path := TestBundlesEventsFilePath(projectRoot)
	return readTestBundleSharedJSONLTail(ctx, path, maxLines)
}

// ReadTestBundleProgressTailLines reads up to maxLines non-empty JSON objects from the end of progress.jsonl.
func ReadTestBundleProgressTailLines(ctx context.Context, projectRoot string, maxLines int) ([]map[string]any, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root required")
	}
	path := TestBundlesProgressFilePath(projectRoot)
	return readTestBundleSharedJSONLTail(ctx, path, maxLines)
}

func readTestBundleSharedJSONLTail(ctx context.Context, path string, maxLines int) ([]map[string]any, error) {
	if maxLines <= 0 {
		maxLines = 500
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	f, err := fileutil.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var raw []string
	sc := NewTestBundleHealthJSONLScanner(f)
	n := 0
	for sc.Scan() {
		n++
		if fn := testHookAfterHealthJSONLScanLine; fn != nil {
			fn(n)
		}
		if ctx != nil && n%healthJSONLScanYieldInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		s := strings.TrimSpace(sc.Text())
		if s == emptyValue {
			continue
		}
		raw = append(raw, s)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(raw) > maxLines {
		raw = raw[len(raw)-maxLines:]
	}

	out := make([]map[string]any, 0, len(raw))
	for _, s := range raw {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// readTestBundleHealthJSONLFullScan reads health.jsonl from the start, returning parsed rows.
// If more than maxParsedRows successfully parsed rows are seen, returns an error (caller raises --health-limit).
// If the file is missing, returns (nil, true, nil).
func readTestBundleHealthJSONLFullScan(projectRoot string, maxParsedRows int) ([]map[string]any, bool, error) {
	if projectRoot == emptyValue {
		return nil, false, errfmt.Errorf("project root required")
	}
	path := TestBundlesHealthFilePath(projectRoot)
	f, err := fileutil.Open(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, true, nil
		}
		return nil, false, err
	}
	defer f.Close()

	var out []map[string]any
	sc := NewTestBundleHealthJSONLScanner(f)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s == emptyValue {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) != nil {
			continue
		}
		out = append(out, m)
		if len(out) > maxParsedRows {
			return nil, false, errfmt.Errorf("health.jsonl exceeds %d parsed rows (raise --health-limit)", maxParsedRows)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, false, err
	}
	return out, false, nil
}

func parseSuggestedRerunCommands(m map[string]any) []string {
	if m == nil {
		return nil
	}
	arr, ok := m[KeySuggestedRerunCommands].([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	var out []string
	for _, x := range arr {
		if s, ok := x.(string); ok && strings.TrimSpace(s) != emptyValue {
			out = append(out, s)
		}
	}
	return out
}

func outcomeIsBad(o string) bool {
	switch strings.TrimSpace(o) {
	case "test_fail", "fail", "timeout", "flake", "quarantined_flake", "quarantine", "build_fail", "build_failure", "compile_fail":
		return true
	default:
		return false
	}
}

func outcomeIsFlake(o string) bool {
	switch strings.TrimSpace(o) {
	case "flake", "quarantined_flake", "quarantine":
		return true
	default:
		return false
	}
}

func outcomeIsBuildFailure(o string) bool {
	switch strings.TrimSpace(o) {
	case "build_fail", "build_failure", "compile_fail":
		return true
	default:
		return false
	}
}

func outcomeIsGood(o string) bool {
	switch strings.TrimSpace(o) {
	case "pass", "ok":
		return true
	default:
		return false
	}
}

// readTriggerQueuePending returns len(PeekTriggerRequests) or -1 if projectRoot is empty or read fails.
func readTriggerQueuePending(projectRoot string) int {
	if projectRoot == emptyValue {
		return -1
	}
	q := NewJobTriggerQueue(projectRoot)
	pending, err := q.PeekTriggerRequests()
	if err != nil {
		return -1
	}
	return len(pending)
}

// applySessionCompletionGates sets ReadyForSessionCompletion, SessionCompletionBlockedReasons, and SessionCompletionNote.
func applySessionCompletionGates(out *TestBundleConvergenceSnapshot) {
	var reasons []string
	if len(out.FailingFingerprintsNow) > 0 {
		reasons = append(reasons, "one or more bundle fingerprints have latest outcome fail/test_fail/timeout")
	}
	if len(out.FlakingFingerprintsNow) > 0 {
		reasons = append(reasons, "one or more bundle fingerprints have latest outcome flake/quarantine (quarantined flake cannot report green)")
	}
	if out.HadFailureInWindow && len(out.FailingFingerprintsNow) == 0 && len(out.FlakingFingerprintsNow) == 0 {
		reasons = append(reasons, "failures occurred earlier in this health window (last line per fingerprint is green but window is not clean)")
	}
	if out.DeltaAssessment == "unknown" {
		reasons = append(reasons, "no health data in window")
	}
	if out.Heartbeat != nil && out.Heartbeat.Stale {
		reasons = append(reasons, "health watermark is stale (no recent bundle completions)")
	}
	switch {
	case out.TriggerQueuePending > 0:
		reasons = append(reasons, "scheduler trigger queue has pending work (bundle jobs not yet started)")
	case out.TriggerQueuePending < 0:
		reasons = append(reasons, "trigger queue depth unavailable (empty project root or queue read error)")
	}
	out.SessionCompletionBlockedReasons = reasons
	out.ReadyForSessionCompletion = len(reasons) == 0 && out.DeltaAssessment == "neutral" && len(out.FailingFingerprintsNow) == 0
	if out.ReadyForSessionCompletion {
		out.SessionCompletionNote = paths.RewriteCanonicalCLIInvocations("Health snapshot is green; also confirm `zqk scheduler activity` shows busyness Executing=0 and PendingInQueue=0 before closing a convergence_session (jobs may still run when trigger queue is empty).")
	}
}

// BuildTestBundleConvergenceSnapshot computes delta_assessment and related fields from parsed health lines
// (oldest to newest within the window). Last line per fingerprint wins for outcome.
// projectRoot is used to read the scheduler trigger queue (pending work). Pass "" in unit tests to skip queue depth.
func BuildTestBundleConvergenceSnapshot(projectRoot string, lines []map[string]any) *TestBundleConvergenceSnapshot {
	out := &TestBundleConvergenceSnapshot{
		LinesInWindow:               len(lines),
		FingerprintLatestOutcome:    make(map[string]string),
		SuggestedRerunByFingerprint: make(map[string][]string),
	}
	out.TriggerQueuePending = readTriggerQueuePending(projectRoot)
	if len(lines) == 0 {
		out.DeltaAssessment = "unknown"
		out.NextActionHint = "No health lines in window; widen --limit or run scheduler test bundles."
		applySessionCompletionGates(out)
		fillPrimaryMeasurementOutcomeFromRollup(out)
		return out
	}

	var lastTS string
	// openBadFingerprint: saw a bad outcome for this fingerprint without a later good outcome
	// in the same window (chronological order). A historic test_fail line therefore does not
	// poison small health.jsonl files forever once the same bundle fingerprint records pass again.
	openBadFingerprint := make(map[string]struct{})
	for _, m := range lines {
		if ts, ok := m[KeyTimestamp].(string); ok && ts != emptyValue {
			lastTS = ts
		}
		fp, _ := m[KeyBundleCommandFingerprint].(string)
		if fp == emptyValue {
			continue
		}
		o, _ := m[KeyTestOutcome].(string)
		out.FingerprintLatestOutcome[fp] = o
		if outcomeIsBad(o) {
			openBadFingerprint[fp] = struct{}{}
			out.SuggestedRerunByFingerprint[fp] = parseSuggestedRerunCommands(m)
		} else if outcomeIsGood(o) {
			delete(openBadFingerprint, fp)
			delete(out.SuggestedRerunByFingerprint, fp)
		}
	}

	// Override/update with live progress state and verification signals
	if projectRoot != emptyValue {
		// 1. Process verification_signals from lines (health.jsonl)
		for _, m := range lines {
			et, _ := m[objects.FieldKeyEventType].(string)
			if et == "verification_signal" {
				tp, _ := m["test_pattern"].(string)
				if fp := findFingerprintForPattern(projectRoot, tp); fp != "" {
					out.FingerprintLatestOutcome[fp] = "running"
					delete(out.SuggestedRerunByFingerprint, fp)
					delete(openBadFingerprint, fp)
				}
			}
		}

		// 2. Read up to 200 progress lines
		if pLines, err := ReadTestBundleProgressTailLines(context.Background(), projectRoot, 200); err == nil { // Background: request-or-shutdown derived
			runningFPs := map[string]bool{}
			latestProgressOutcome := map[string]string{}
			for _, m := range pLines {
				fp, _ := m["bundle_command_fingerprint"].(string)
				et, _ := m[objects.FieldKeyEventType].(string)
				if fp == "" {
					continue
				}
				if et == "started" {
					runningFPs[fp] = true
					delete(latestProgressOutcome, fp)
				} else if et == "passed" || et == "pass" || et == "completed" || et == "failed" || et == "fail" || et == "test_fail" {
					delete(runningFPs, fp)
					if et == "passed" || et == "pass" || et == "completed" {
						latestProgressOutcome[fp] = "pass"
					} else {
						latestProgressOutcome[fp] = "fail"
					}
				}
			}
			for fp := range runningFPs {
				out.FingerprintLatestOutcome[fp] = "running"
				delete(out.SuggestedRerunByFingerprint, fp)
				delete(openBadFingerprint, fp)
			}
			for fp, o := range latestProgressOutcome {
				out.FingerprintLatestOutcome[fp] = o
				if o == "pass" {
					delete(out.SuggestedRerunByFingerprint, fp)
					delete(openBadFingerprint, fp)
				} else {
					openBadFingerprint[fp] = struct{}{}
				}
			}
		}
	}

	out.HealthWatermarkRFC3339 = lastTS
	out.HadFailureInWindow = len(openBadFingerprint) > 0

	var failing []string
	var flaking []string
	var buildFailing []string
	var passCount, flakeCount, failCount, buildFailCount int
	for fp, o := range out.FingerprintLatestOutcome {
		if outcomeIsFlake(o) {
			flaking = append(flaking, fp)
			flakeCount++
			failing = append(failing, fp)
		} else if outcomeIsBuildFailure(o) {
			buildFailing = append(buildFailing, fp)
			buildFailCount++
			failing = append(failing, fp)
		} else if outcomeIsBad(o) {
			failing = append(failing, fp)
			failCount++
		} else if outcomeIsGood(o) {
			passCount++
		}
	}
	sort.Strings(failing)
	sort.Strings(flaking)
	sort.Strings(buildFailing)
	out.FailingFingerprintsNow = failing
	out.FlakingFingerprintsNow = flaking
	out.BuildFailingFingerprintsNow = buildFailing
	out.PassCount = passCount
	out.FlakeCount = flakeCount
	out.FailCount = failCount
	out.BuildFailCount = buildFailCount

	switch {
	case len(flaking) > 0:
		out.DeltaAssessment = "trending_away"
		out.NextActionHint = "One or more bundle fingerprints flaking/quarantined (quarantined flakes cannot report green); investigate and fix test instability."
	case len(failing) > 0:
		out.DeltaAssessment = "trending_away"
		out.NextActionHint = "One or more bundle fingerprints still failing; use suggested_rerun_by_fingerprint or apply a fix, then re-run bundles."
	case out.HadFailureInWindow:
		out.DeltaAssessment = "trending_toward"
		out.NextActionHint = "Latest outcomes are green but an unrecovered bad outcome remains in-window for at least one fingerprint (e.g. ambiguous outcomes after a fail); confirm with another bundle run or inspect health.jsonl."
	default:
		out.DeltaAssessment = "neutral"
		out.NextActionHint = "No failing outcomes in this window."
	}

	if out.TriggerQueuePending > 0 {
		out.NextActionHint = "Trigger queue has pending work; wait for bundle jobs to drain before treating empty failing_fingerprints_now as final. " + out.NextActionHint
	}

	if lastTS != emptyValue {
		if t, err := time.Parse(time.RFC3339, lastTS); err == nil {
			age := time.Since(t).Seconds()
			staleAfter := DefaultConvergenceHeartbeatStaleAfter.Seconds()
			out.Heartbeat = &ConvergenceHeartbeat{
				MeasurementAgeSeconds: age,
				Stale:                 age > staleAfter,
				StaleAfterSeconds:     staleAfter,
			}
		}
	}
	applySessionCompletionGates(out)
	fillPrimaryMeasurementOutcomeFromRollup(out)
	return out
}

// fillPrimaryMeasurementOutcomeFromRollup sets PrimaryMeasurementOutcome* using the same bundle-only
// slice as rollup_status_core when literal gates are not run (gatesSkipped=true).
func fillPrimaryMeasurementOutcomeFromRollup(out *TestBundleConvergenceSnapshot) {
	tb := convergerollup.TestBundleInput{
		ReadyForSessionCompletion: out.ReadyForSessionCompletion,
		FailingFingerprintsNow:    out.FailingFingerprintsNow,
		DeltaAssessment:           out.DeltaAssessment,
	}
	st, blockers, readyForParentCompletion, recommendedNext := convergerollup.ComputeRollupStatus(tb, 0, 0, nil, "", nil)
	_ = readyForParentCompletion // Acknowledged
	_ = recommendedNext          // Acknowledged
	pm, detail := convergerollup.ComputePrimaryMeasurementOutcome(st, blockers, "", 0, 0, true)
	out.PrimaryMeasurementOutcome = string(pm)
	out.PrimaryMeasurementOutcomeDetail = detail
	out.MeasurementOutcomeSchemaVersion = "1"
}

func findFingerprintForPattern(projectRoot, pattern string) string {
	if pattern == "" {
		return ""
	}
	// Try direct fingerprint hash in case it's already a fingerprint
	if len(pattern) >= 24 && !strings.Contains(pattern, " ") {
		return pattern
	}
	fp := FingerprintBundleCommand(pattern)
	if projectRoot == "" {
		return fp
	}

	dir := filepath.Join(projectRoot, paths.ProcessDir, "scheduler_jobs")
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.yaml"))
	if err != nil {
		return fp
	}

	for _, file := range files {
		b, err := fileutil.ReadFile(file)
		if err != nil {
			continue
		}
		content := string(b)
		if strings.Contains(content, pattern) {
			idx := strings.Index(content, "bundle_command_fingerprint:")
			if idx >= 0 {
				remain := content[idx+len("bundle_command_fingerprint:"):]
				remain = strings.TrimSpace(remain)
				if len(remain) > 0 && remain[0] == '"' {
					remain = remain[1:]
					endIdx := strings.Index(remain, "\"")
					if endIdx >= 0 {
						return remain[:endIdx]
					}
				} else {
					parts := strings.Fields(remain)
					if len(parts) > 0 {
						return parts[0]
					}
				}
			}
		}
	}
	return fp
}
