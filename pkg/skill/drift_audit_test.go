package skill

import (
	"path/filepath"
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDriftAuditor(t *testing.T) {
	// Create temporary workspace
	tempDir := t.TempDir()

	primaryDir := filepath.Join(tempDir, ".zqk", "skills")
	secondaryDir := filepath.Join(tempDir, "skills")

	if err := fileutil.MkdirAll(primaryDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(secondaryDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Helper to create a skill
	createSkill := func(dir, name string, mtime time.Time) {
		skillDir := filepath.Join(dir, name)
		if err := fileutil.MkdirAll(skillDir, 0755); err != nil {
			t.Fatal(err)
		}
		skillMd := filepath.Join(skillDir, "SKILL.md")
		if err := fileutil.WriteFile(skillMd, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := fileutil.Chtimes(skillMd, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now()
	later := now.Add(time.Hour)

	// Scenario: perfect alignment
	createSkill(primaryDir, "skill1", now)
	createSkill(secondaryDir, "skill1", now)

	// Scenario: missing in secondary
	createSkill(primaryDir, "skill2", now)

	// Scenario: missing in primary
	createSkill(secondaryDir, "skill3", now)

	// Scenario: mtime mismatch
	createSkill(primaryDir, "skill4", now)
	createSkill(secondaryDir, "skill4", later)

	auditor := NewDriftAuditor(tempDir)
	issues, err := auditor.Audit()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d: %v", len(issues), issues)
	}

	issueTypes := make(map[string]string)
	for _, issue := range issues {
		issueTypes[issue.SkillName] = issue.IssueType
	}

	if issueTypes["skill2"] != "MissingInSecondary" {
		t.Errorf("expected skill2 to be MissingInSecondary, got %v", issueTypes["skill2"])
	}
	if issueTypes["skill3"] != "MissingInPrimary" {
		t.Errorf("expected skill3 to be MissingInPrimary, got %v", issueTypes["skill3"])
	}
	if issueTypes["skill4"] != "MtimeMismatch" {
		t.Errorf("expected skill4 to be MtimeMismatch, got %v", issueTypes["skill4"])
	}
}
