package closureevidence_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/closureevidence"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func writeRawHealthFile(t *testing.T, root, content string) {
	t.Helper()
	healthPath := filepath.Join(root, paths.ProjectDataDir, paths.SchedulerLogsDir, paths.SchedulerCVSSubdir, paths.SchedulerTestBundlesSubdir, "health.jsonl")
	if err := fileutil.MkdirAll(filepath.Dir(healthPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(healthPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
}

func TestExtractEvidence_NilAndEmptyScenarios(t *testing.T) {
	// Nil object
	ev, has := closureevidence.ExtractEvidence(nil)
	if has {
		t.Errorf("expected has=false for nil map, got true")
	}
	if ev != (closureevidence.Evidence{}) {
		t.Errorf("expected zero Evidence, got %+v", ev)
	}

	// Empty map
	ev, has = closureevidence.ExtractEvidence(map[string]any{})
	if has {
		t.Errorf("expected has=false for empty map, got true")
	}
	if ev != (closureevidence.Evidence{}) {
		t.Errorf("expected zero Evidence, got %+v", ev)
	}

	// Whitespace only map
	ev, has = closureevidence.ExtractEvidence(map[string]any{
		"job_id":                     "   ",
		"bundle_log_path":            " \t ",
		"bundle_command_fingerprint": "\n ",
	})
	if has {
		t.Errorf("expected has=false for whitespace-only map, got true")
	}

	// Non-string types should not panic and return has=false
	ev, has = closureevidence.ExtractEvidence(map[string]any{
		"evidence": map[string]any{
			"job_id":      12345,
			"log_path":    true,
			"fingerprint": nil,
		},
		"job_id":                     123,
		"bundle_log_path":            false,
		"bundle_command_fingerprint": 99.9,
	})
	if has {
		t.Errorf("expected has=false for non-string fields, got true")
	}
}

func TestExtractEvidence_FallbackVariants(t *testing.T) {
	// 1. Nested evidence with bundle_command_fingerprint instead of fingerprint
	objNestedAlt := map[string]any{
		"evidence": map[string]any{
			"job_id":                     "SCH-nested-alt",
			"log_path":                   "logs/alt.log",
			"bundle_command_fingerprint": "fp-nested-alt",
		},
	}
	ev, has := closureevidence.ExtractEvidence(objNestedAlt)
	if !has {
		t.Fatal("expected has=true for nested evidence with bundle_command_fingerprint")
	}
	if ev.JobID != "SCH-nested-alt" || ev.LogPath != "logs/alt.log" || ev.Fingerprint != "fp-nested-alt" {
		t.Fatalf("unexpected extracted evidence: %+v", ev)
	}

	// 2. Top-level fallback to log_path (when bundle_log_path is empty)
	// and top-level fallback to fingerprint (when bundle_command_fingerprint is empty)
	objTopFallbacks := map[string]any{
		"job_id":      "SCH-top-fallback",
		"log_path":    "logs/fallback.log",
		"fingerprint": "fp-top-fallback",
	}
	ev, has = closureevidence.ExtractEvidence(objTopFallbacks)
	if !has {
		t.Fatal("expected has=true for top-level fallbacks")
	}
	if ev.JobID != "SCH-top-fallback" || ev.LogPath != "logs/fallback.log" || ev.Fingerprint != "fp-top-fallback" {
		t.Fatalf("unexpected extracted evidence: %+v", ev)
	}
}

func TestValidateEvidence_BypassEnvVar(t *testing.T) {
	// When TEST_BYPASS_GITEVIDENCE is set to 1, ValidateEvidence must return nil immediately
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "1")

	err := closureevidence.ValidateEvidence("", closureevidence.Evidence{})
	if err != nil {
		t.Fatalf("expected nil error when bypass env var is set, got: %v", err)
	}
}

func TestValidateEvidence_EmptyRepoRoot(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")

	ev := closureevidence.Evidence{
		JobID:       "SCH-100",
		LogPath:     "log.log",
		Fingerprint: "fp-100",
	}

	if err := closureevidence.ValidateEvidence("", ev); err == nil || !strings.Contains(err.Error(), "repo root is required") {
		t.Fatalf("expected repo root error for empty root, got: %v", err)
	}

	if err := closureevidence.ValidateEvidence("   ", ev); err == nil || !strings.Contains(err.Error(), "repo root is required") {
		t.Fatalf("expected repo root error for whitespace root, got: %v", err)
	}
}

func TestValidateEvidence_WhitespaceFieldsFail(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)

	// Whitespace JobID
	err := closureevidence.ValidateEvidence(root, closureevidence.Evidence{
		JobID:       "   ",
		LogPath:     "valid.log",
		Fingerprint: "fp-1",
	})
	if err == nil || !strings.Contains(err.Error(), "requires a non-empty scheduler job_id") {
		t.Fatalf("expected non-empty job_id error, got: %v", err)
	}

	// Whitespace LogPath
	err = closureevidence.ValidateEvidence(root, closureevidence.Evidence{
		JobID:       "SCH-1",
		LogPath:     " \t ",
		Fingerprint: "fp-1",
	})
	if err == nil || !strings.Contains(err.Error(), "requires a non-empty bundle log_path") {
		t.Fatalf("expected non-empty log_path error, got: %v", err)
	}

	// Whitespace Fingerprint
	err = closureevidence.ValidateEvidence(root, closureevidence.Evidence{
		JobID:       "SCH-1",
		LogPath:     "valid.log",
		Fingerprint: "\n ",
	})
	if err == nil || !strings.Contains(err.Error(), "requires a non-empty bundle_command_fingerprint") {
		t.Fatalf("expected non-empty bundle_command_fingerprint error, got: %v", err)
	}
}

func TestValidateEvidence_AbsoluteLogPath(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)

	// Absolute log path that exists
	absLog := writeBundleLog(t, root, "abs-bundle.log", "PASS\n")

	rec := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-abs-test",
		JobID:                    "SCH-abs-job",
		TestOutcome:              "pass",
		TestsFailed:              0,
	}
	appendHealthRecord(t, root, rec)

	ev := closureevidence.Evidence{
		JobID:       "SCH-abs-job",
		LogPath:     absLog, // absolute path
		Fingerprint: "fp-abs-test",
	}

	if err := closureevidence.ValidateEvidence(root, ev); err != nil {
		t.Fatalf("expected absolute log path to validate successfully, got error: %v", err)
	}
}

func TestValidateEvidence_HealthJsonlMissing(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := t.TempDir() // Temp dir without health.jsonl or health directory
	logRel := "test.log"
	writeBundleLog(t, root, logRel, "PASS\n")

	ev := closureevidence.Evidence{
		JobID:       "SCH-missing-health",
		LogPath:     logRel,
		Fingerprint: "fp-missing-health",
	}

	err := closureevidence.ValidateEvidence(root, ev)
	if err == nil || !strings.Contains(err.Error(), "cannot read health.jsonl") {
		t.Fatalf("expected cannot read health.jsonl error, got: %v", err)
	}
}

func TestValidateEvidence_HealthJsonlEmptyAndCorruptedLines(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)
	logRel := "test.log"
	writeBundleLog(t, root, logRel, "PASS\n")

	// health.jsonl with empty lines, comments/malformed JSON, and a valid green record
	rawContent := "\n\n   \n" +
		"{not-valid-json}\n" +
		"{\"unexpected_format\": true}\n" +
		"{\"job_id\":\"SCH-with-corrupt-neighbors\",\"bundle_command_fingerprint\":\"fp-corrupt-neighbors\",\"test_outcome\":\"pass\",\"tests_failed\":0}\n" +
		"\n"

	writeRawHealthFile(t, root, rawContent)

	ev := closureevidence.Evidence{
		JobID:       "SCH-with-corrupt-neighbors",
		LogPath:     logRel,
		Fingerprint: "fp-corrupt-neighbors",
	}

	if err := closureevidence.ValidateEvidence(root, ev); err != nil {
		t.Fatalf("expected validation to pass despite blank and corrupt lines, got: %v", err)
	}
}

func TestValidateEvidence_MatchingByJobIDOrFingerprint(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)
	logRel := "match.log"
	writeBundleLog(t, root, logRel, "PASS\n")

	// 1. Record matches by BundleCommandFingerprint even when JobID in record differs
	recByFP := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-unique-1",
		JobID:                    "SCH-other-job",
		TestOutcome:              "pass",
		TestsFailed:              0,
	}
	appendHealthRecord(t, root, recByFP)

	ev1 := closureevidence.Evidence{
		JobID:       "SCH-query-job",
		LogPath:     logRel,
		Fingerprint: "fp-unique-1",
	}
	if err := closureevidence.ValidateEvidence(root, ev1); err != nil {
		t.Fatalf("expected match by fingerprint to pass, got: %v", err)
	}

	// 2. Record matches by JobID even when BundleCommandFingerprint differs
	recByJob := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-other",
		JobID:                    "SCH-job-exact",
		TestOutcome:              "pass",
		TestsFailed:              0,
	}
	appendHealthRecord(t, root, recByJob)

	ev2 := closureevidence.Evidence{
		JobID:       "SCH-job-exact",
		LogPath:     logRel,
		Fingerprint: "fp-different-query",
	}
	if err := closureevidence.ValidateEvidence(root, ev2); err != nil {
		t.Fatalf("expected match by job id to pass, got: %v", err)
	}
}

func TestValidateEvidence_RecordNotFound(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)
	logRel := "notfound.log"
	writeBundleLog(t, root, logRel, "PASS\n")

	rec := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-exists",
		JobID:                    "SCH-exists",
		TestOutcome:              "pass",
		TestsFailed:              0,
	}
	appendHealthRecord(t, root, rec)

	ev := closureevidence.Evidence{
		JobID:       "SCH-unrelated",
		LogPath:     logRel,
		Fingerprint: "fp-unrelated",
	}
	err := closureevidence.ValidateEvidence(root, ev)
	if err == nil || !strings.Contains(err.Error(), "not found in health.jsonl") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

func TestValidateEvidence_OutcomePassWithFailuresFails(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)
	logRel := "pass-with-failures.log"
	writeBundleLog(t, root, logRel, "FAIL\n")

	// Outcome is "pass", but TestsFailed is > 0
	rec := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-pass-but-failed",
		JobID:                    "SCH-pass-failed",
		TestOutcome:              "pass",
		TestsFailed:              2,
	}
	appendHealthRecord(t, root, rec)

	ev := closureevidence.Evidence{
		JobID:       "SCH-pass-failed",
		LogPath:     logRel,
		Fingerprint: "fp-pass-but-failed",
	}
	err := closureevidence.ValidateEvidence(root, ev)
	if err == nil || !strings.Contains(err.Error(), "must be green pass") {
		t.Fatalf("expected error for pass with test failures > 0, got: %v", err)
	}
}

func TestValidateEvidence_MultipleRecordsLatestControls(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)
	logRel := "multi.log"
	writeBundleLog(t, root, logRel, "PASS\n")

	// 1. Earlier failed, later passed -> should PASS
	appendHealthRecord(t, root, closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-flaky-fixed",
		JobID:                    "SCH-flaky",
		TestOutcome:              "fail",
		TestsFailed:              1,
	})
	appendHealthRecord(t, root, closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-flaky-fixed",
		JobID:                    "SCH-flaky",
		TestOutcome:              "pass",
		TestsFailed:              0,
	})

	evPassed := closureevidence.Evidence{
		JobID:       "SCH-flaky",
		LogPath:     logRel,
		Fingerprint: "fp-flaky-fixed",
	}
	if err := closureevidence.ValidateEvidence(root, evPassed); err != nil {
		t.Fatalf("expected latest green record to pass, got: %v", err)
	}

	// 2. Earlier passed, later failed -> should FAIL
	appendHealthRecord(t, root, closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-regressed",
		JobID:                    "SCH-regressed",
		TestOutcome:              "pass",
		TestsFailed:              0,
	})
	appendHealthRecord(t, root, closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-regressed",
		JobID:                    "SCH-regressed",
		TestOutcome:              "fail",
		TestsFailed:              3,
	})

	evFailed := closureevidence.Evidence{
		JobID:       "SCH-regressed",
		LogPath:     logRel,
		Fingerprint: "fp-regressed",
	}
	if err := closureevidence.ValidateEvidence(root, evFailed); err == nil || !strings.Contains(err.Error(), "must be green pass") {
		t.Fatalf("expected latest failing record to fail, got: %v", err)
	}
}

func TestValidateEvidence_ScannerErrorOnOversizedLine(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)
	logRel := "huge.log"
	writeBundleLog(t, root, logRel, "PASS\n")

	// bufio.Scanner default max token size is 64KB (65536 bytes).
	// Writing a line of 70000 bytes without newline triggers bufio.ErrTooLong.
	oversizedLine := strings.Repeat("x", 70000) + "\n"
	writeRawHealthFile(t, root, oversizedLine)

	ev := closureevidence.Evidence{
		JobID:       "SCH-oversized",
		LogPath:     logRel,
		Fingerprint: "fp-oversized",
	}

	err := closureevidence.ValidateEvidence(root, ev)
	if err == nil || !strings.Contains(err.Error(), "error reading health.jsonl") {
		t.Fatalf("expected scanner error reading health.jsonl, got: %v", err)
	}
}

func TestValidateObjectClosureEvidence_FailurePropagation(t *testing.T) {
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	root := setupTestRepoWithHealth(t)

	// Object with evidence pointing to nonexistent log
	obj := map[string]any{
		"evidence": map[string]any{
			"job_id":      "SCH-not-found-log",
			"log_path":    "nonexistent-bundle.log",
			"fingerprint": "fp-valid",
		},
	}
	err := closureevidence.ValidateObjectClosureEvidence(root, obj)
	if err == nil || !strings.Contains(err.Error(), "not found on disk") {
		t.Fatalf("expected error propagating from ValidateEvidence, got: %v", err)
	}
}
