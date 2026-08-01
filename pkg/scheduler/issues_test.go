package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestClearIssues_WritesToAbsolutePathAndReturnsIt(t *testing.T) {
	dir := t.TempDir()
	issuesPath := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, issuesFileName)
	if err := fileutil.EnsureDir(filepath.Dir(issuesPath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Start with "issues" state so we can confirm it was cleared
	initial := IssuesPayload{Status: issuesStatusIssues, UpdatedAt: "2020-01-01T00:00:00Z", Issues: []Issue{{JobID: "J1", Error: "test"}}}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := fileutil.WriteSecureFile(issuesPath, data); err != nil {
		t.Fatalf("write initial: %v", err)
	}

	writtenPath, err := ClearIssues(dir)
	if err != nil {
		t.Fatalf("ClearIssues: %v", err)
	}
	// Returned path must be absolute and point to the file we expect
	if writtenPath != issuesPath {
		absExpected, _ := filepath.Abs(issuesPath)
		if writtenPath != absExpected {
			t.Errorf("ClearIssues returned path %q, expected %q (abs %q)", writtenPath, issuesPath, absExpected)
		}
	}

	// Verify file content
	got, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var payload IssuesPayload
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Status != issuesStatusOK {
		t.Errorf("status = %q, want %q", payload.Status, issuesStatusOK)
	}
	if len(payload.Issues) != 0 {
		t.Errorf("issues length = %d, want 0", len(payload.Issues))
	}
}

func TestClearIssues_NormalizesRelativePath(t *testing.T) {
	dir := t.TempDir()
	// Use a relative path that would resolve to dir when CWD is dir (we'll chdir in test)
	schedulerDir := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir)
	if err := fileutil.EnsureDir(schedulerDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// ClearIssues with absolute path
	writtenPath, err := ClearIssues(dir)
	if err != nil {
		t.Fatalf("ClearIssues: %v", err)
	}
	expectedAbs := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, issuesFileName)
	if writtenPath != expectedAbs {
		t.Errorf("written path %q is not absolute under project root; expected %q", writtenPath, expectedAbs)
	}
}

func TestReadIssues_MissingFile_ReturnsOkPayload(t *testing.T) {
	dir := t.TempDir()
	// Do not create issues.json

	payload, err := ReadIssues(dir)
	if err != nil {
		t.Fatalf("ReadIssues: %v", err)
	}
	if payload == nil {
		t.Fatal("ReadIssues returned nil payload")
	}
	if payload.Status != issuesStatusOK {
		t.Errorf("status = %q, want %q", payload.Status, issuesStatusOK)
	}
	if len(payload.Issues) != 0 {
		t.Errorf("issues length = %d, want 0", len(payload.Issues))
	}
}

func TestReadIssues_ExistingFile_ReturnsPayload(t *testing.T) {
	dir := t.TempDir()
	issuesPath := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, issuesFileName)
	if err := fileutil.EnsureDir(filepath.Dir(issuesPath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := IssuesPayload{
		Status:    issuesStatusIssues,
		UpdatedAt: "2026-02-27T12:00:00Z",
		Issues:    []Issue{{JobID: "SCH-1", JobType: JobTypeRunWrapper, Error: "timeout", At: "2026-02-27T12:00:00Z"}},
	}
	data, _ := json.MarshalIndent(want, "", "  ")
	if err := fileutil.WriteSecureFile(issuesPath, data); err != nil {
		t.Fatalf("write: %v", err)
	}

	payload, err := ReadIssues(dir)
	if err != nil {
		t.Fatalf("ReadIssues: %v", err)
	}
	if payload == nil {
		t.Fatal("ReadIssues returned nil payload")
	}
	if payload.Status != want.Status {
		t.Errorf("status = %q, want %q", payload.Status, want.Status)
	}
	if len(payload.Issues) != 1 {
		t.Fatalf("issues length = %d, want 1", len(payload.Issues))
	}
	if payload.Issues[0].JobID != "SCH-1" || payload.Issues[0].Error != "timeout" {
		t.Errorf("issue = %+v", payload.Issues[0])
	}
}

func TestReadIssues_MalformedFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	issuesPath := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, issuesFileName)
	if err := fileutil.EnsureDir(filepath.Dir(issuesPath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteSecureFile(issuesPath, []byte("not json")); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := ReadIssues(dir)
	if err == nil {
		t.Fatal("ReadIssues expected error for malformed JSON")
	}
}

func TestReadIssues_EmptyProjectRoot_ReturnsNilNil(t *testing.T) {
	payload, err := ReadIssues("")
	if err != nil {
		t.Fatalf("ReadIssues with empty root: %v", err)
	}
	if payload != nil {
		t.Errorf("expected nil payload for empty project root, got %+v", payload)
	}
}
