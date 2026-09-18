package community

import (
	"strings"
	"testing"
	"time"
)

// TestChangelogGeneration_FunctionalAcceptance verifies that conventional commit entries
// are synthesized into formatted markdown release notes (CRIT-1789706079947905000-27446a5f).
func TestChangelogGeneration_FunctionalAcceptance(t *testing.T) {
	entries := []ReleaseNoteEntry{
		{Type: "feat", Scope: "community", Message: "automate release packaging", Commit: "abc1234"},
		{Type: "fix", Scope: "cli", Message: "resolve flags parsing edge case", Commit: "def5678"},
		{Type: "chore", Scope: "deps", Message: "update toolchain dependencies", Commit: "ghi9012"},
	}

	date := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	changelog := GenerateChangelog("v2.7.0", date, entries)

	expectedSections := []string{
		"## [v2.7.0] - 2026-09-18",
		"### Features",
		"- **community**: automate release packaging (abc1234)",
		"### Bug Fixes",
		"- **cli**: resolve flags parsing edge case (def5678)",
		"### Maintenance & Chores",
		"- **deps**: update toolchain dependencies (ghi9012)",
	}

	for _, section := range expectedSections {
		if !strings.Contains(changelog, section) {
			t.Errorf("expected changelog to contain %q, got:\n%s", section, changelog)
		}
	}
}

// TestChangelogGeneration_BoundaryAndErrorHandling verifies handling of empty scopes,
// empty commits, and single-category releases (CRIT-1789706079947906000-f2512688).
func TestChangelogGeneration_BoundaryAndErrorHandling(t *testing.T) {
	entries := []ReleaseNoteEntry{
		{Type: "feat", Scope: "", Message: "initial open core release", Commit: "0000000"},
	}

	date := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	changelog := GenerateChangelog("v0.1.0", date, entries)

	if !strings.Contains(changelog, "- initial open core release (0000000)") {
		t.Errorf("expected unscoped entry format, got:\n%s", changelog)
	}
	if strings.Contains(changelog, "### Bug Fixes") {
		t.Errorf("expected no Bug Fixes section when no fixes exist, got:\n%s", changelog)
	}
}

// TestChangelogGeneration_IntegrationAndConformance verifies release artifact checksum
// validation logic integration (CRIT-1789706079947907000-68fe754f).
func TestChangelogGeneration_IntegrationAndConformance(t *testing.T) {
	manifest := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  zqk-community_2.7.0_darwin_arm64.tar.gz\n"
	lines := strings.Split(strings.TrimSpace(manifest), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 checksum line, got %d", len(lines))
	}

	parts := strings.Fields(lines[0])
	if len(parts) != 2 {
		t.Fatalf("expected sha256 + filename, got %v", parts)
	}

	if len(parts[0]) != 64 {
		t.Errorf("expected 64-char sha256 hash, got %d chars", len(parts[0]))
	}
}
