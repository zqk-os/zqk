package closureevidence_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/closureevidence"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func setupTestRepoWithHealth(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	logDir := filepath.Join(root, paths.ProjectDataDir, paths.SchedulerLogsDir, paths.SchedulerCVSSubdir, paths.SchedulerTestBundlesSubdir)
	if err := fileutil.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeBundleLog(t *testing.T, root, relPath, content string) string {
	t.Helper()
	absPath := filepath.Join(root, relPath)
	if err := fileutil.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return absPath
}

func appendHealthRecord(t *testing.T, root string, rec closureevidence.HealthRecord) {
	t.Helper()
	healthPath := filepath.Join(root, paths.ProjectDataDir, paths.SchedulerLogsDir, paths.SchedulerCVSSubdir, paths.SchedulerTestBundlesSubdir, "health.jsonl")
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	f, err := fileutil.OpenFile(healthPath, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

func TestValidateEvidence_ValidGreenEvidencePasses(t *testing.T) {
	root := setupTestRepoWithHealth(t)
	logRel := filepath.Join(paths.ProjectDataDir, paths.SchedulerLogsDir, paths.SchedulerCVSSubdir, paths.SchedulerTestBundlesSubdir, "bundle-1.log")
	writeBundleLog(t, root, logRel, "=== RUN TestExample\n--- PASS: TestExample (0.01s)\nPASS\n")

	rec := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-green-1234",
		EventType:                "completed",
		JobID:                    "SCH-run-bundle-1",
		LogPath:                  logRel,
		Package:                  "./pkg/example",
		TestOutcome:              "pass",
		TestsFailed:              0,
		Timestamp:                "2026-08-25T23:30:00Z",
	}
	appendHealthRecord(t, root, rec)

	ev := closureevidence.Evidence{
		JobID:       "SCH-run-bundle-1",
		LogPath:     logRel,
		Fingerprint: "fp-green-1234",
	}

	if err := closureevidence.ValidateEvidence(root, ev); err != nil {
		t.Fatalf("expected valid green evidence to pass, got error: %v", err)
	}
}

func TestValidateEvidence_MissingFieldsFail(t *testing.T) {
	root := setupTestRepoWithHealth(t)

	// Missing JobID
	if err := closureevidence.ValidateEvidence(root, closureevidence.Evidence{
		LogPath:     "some.log",
		Fingerprint: "fp-123",
	}); err == nil {
		t.Fatal("expected error for missing JobID")
	}

	// Missing LogPath
	if err := closureevidence.ValidateEvidence(root, closureevidence.Evidence{
		JobID:       "SCH-1",
		Fingerprint: "fp-123",
	}); err == nil {
		t.Fatal("expected error for missing LogPath")
	}

	// Missing Fingerprint
	if err := closureevidence.ValidateEvidence(root, closureevidence.Evidence{
		JobID:   "SCH-1",
		LogPath: "some.log",
	}); err == nil {
		t.Fatal("expected error for missing Fingerprint")
	}
}

func TestValidateEvidence_MissingOrEmptyBundleLogFails(t *testing.T) {
	root := setupTestRepoWithHealth(t)
	rec := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-123",
		JobID:                    "SCH-1",
		TestOutcome:              "pass",
	}
	appendHealthRecord(t, root, rec)

	// Log does not exist
	if err := closureevidence.ValidateEvidence(root, closureevidence.Evidence{
		JobID:       "SCH-1",
		LogPath:     "nonexistent.log",
		Fingerprint: "fp-123",
	}); err == nil {
		t.Fatal("expected error for nonexistent bundle log")
	}

	// Log is empty
	emptyLog := writeBundleLog(t, root, "empty.log", "")
	if err := closureevidence.ValidateEvidence(root, closureevidence.Evidence{
		JobID:       "SCH-1",
		LogPath:     emptyLog,
		Fingerprint: "fp-123",
	}); err == nil {
		t.Fatal("expected error for empty bundle log")
	}
}

func TestValidateEvidence_FailingHealthOutcomeFails(t *testing.T) {
	root := setupTestRepoWithHealth(t)
	logRel := "bundle-fail.log"
	writeBundleLog(t, root, logRel, "--- FAIL: TestExample\nFAIL\n")

	rec := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-red-5678",
		JobID:                    "SCH-fail-1",
		LogPath:                  logRel,
		TestOutcome:              "fail",
		TestsFailed:              1,
	}
	appendHealthRecord(t, root, rec)

	ev := closureevidence.Evidence{
		JobID:       "SCH-fail-1",
		LogPath:     logRel,
		Fingerprint: "fp-red-5678",
	}

	if err := closureevidence.ValidateEvidence(root, ev); err == nil {
		t.Fatal("expected error for failing health outcome")
	}
}

func TestValidateObjectClosureEvidence_NestedAndTopLevel(t *testing.T) {
	root := setupTestRepoWithHealth(t)
	logRel := "bundle-ok.log"
	writeBundleLog(t, root, logRel, "PASS\n")

	rec := closureevidence.HealthRecord{
		BundleCommandFingerprint: "fp-ok-999",
		JobID:                    "SCH-ok-999",
		LogPath:                  logRel,
		TestOutcome:              "pass",
		TestsFailed:              0,
	}
	appendHealthRecord(t, root, rec)

	// Top level
	objTop := map[string]any{
		"job_id":                     "SCH-ok-999",
		"bundle_log_path":            logRel,
		"bundle_command_fingerprint": "fp-ok-999",
	}
	if err := closureevidence.ValidateObjectClosureEvidence(root, objTop); err != nil {
		t.Fatalf("expected top-level evidence to pass, got: %v", err)
	}

	// Nested under evidence
	objNested := map[string]any{
		"evidence": map[string]any{
			"job_id":      "SCH-ok-999",
			"log_path":    logRel,
			"fingerprint": "fp-ok-999",
		},
	}
	if err := closureevidence.ValidateObjectClosureEvidence(root, objNested); err != nil {
		t.Fatalf("expected nested evidence to pass, got: %v", err)
	}

	// Missing evidence
	objEmpty := map[string]any{
		objects.FieldKeyTitle: "No evidence here",
	}
	if err := closureevidence.ValidateObjectClosureEvidence(root, objEmpty); err == nil {
		t.Fatal("expected error for object missing evidence")
	}
}
