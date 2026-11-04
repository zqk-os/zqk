// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package skill

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func createTestSkillFile(t *testing.T, baseDir, skillName string, mtime time.Time, body string) string {
	t.Helper()
	dir := filepath.Join(baseDir, skillName)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create skill directory %s: %v", dir, err)
	}

	skillPath := filepath.Join(dir, "SKILL.md")
	if err := fileutil.WriteFile(skillPath, []byte(body), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write skill file %s: %v", skillPath, err)
	}

	if err := fileutil.Chtimes(skillPath, mtime, mtime); err != nil {
		t.Fatalf("failed to chtimes skill file %s: %v", skillPath, err)
	}

	return skillPath
}

func TestDriftAuditor_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Both directories missing: returns empty issue slice without error
	auditorNonExistent := NewDriftAuditor(tempDir)
	issues, err := auditorNonExistent.Audit()
	if err != nil {
		t.Fatalf("expected nil error when dirs do not exist, got: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues when dirs do not exist, got: %d", len(issues))
	}

	// 2. PrimaryDir is a file instead of directory: readSkills fails with non-NotExist error
	primaryFile := filepath.Join(tempDir, paths.ProjectDataDir, paths.SkillsSubdir)
	if err := fileutil.MkdirAll(filepath.Dir(primaryFile), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create primary parent: %v", err)
	}
	if err := fileutil.WriteFile(primaryFile, []byte("file instead of dir"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write primary file: %v", err)
	}

	auditorBadPrimary := NewDriftAuditor(tempDir)
	_, err = auditorBadPrimary.Audit()
	if err == nil || !strings.Contains(err.Error(), "failed to read primary skills") {
		t.Fatalf("expected error wrapping 'failed to read primary skills', got: %v", err)
	}

	// Remove primaryFile and recreate as directory
	if err := fileutil.Remove(primaryFile); err != nil {
		t.Fatalf("failed to remove primary file: %v", err)
	}
	if err := fileutil.MkdirAll(primaryFile, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create primary dir: %v", err)
	}

	// 3. SecondaryDir is a file instead of directory
	secondaryFile := filepath.Join(tempDir, "skills")
	if err := fileutil.WriteFile(secondaryFile, []byte("secondary file"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write secondary file: %v", err)
	}

	auditorBadSecondary := NewDriftAuditor(tempDir)
	_, err = auditorBadSecondary.Audit()
	if err == nil || !strings.Contains(err.Error(), "failed to read secondary skills") {
		t.Fatalf("expected error wrapping 'failed to read secondary skills', got: %v", err)
	}
}

func TestDriftAuditor_DirectoryFiltering(t *testing.T) {
	tempDir := t.TempDir()
	primaryDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.SkillsSubdir)
	secondaryDir := filepath.Join(tempDir, "skills")

	if err := fileutil.MkdirAll(primaryDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create primary dir: %v", err)
	}
	if err := fileutil.MkdirAll(secondaryDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create secondary dir: %v", err)
	}

	// Add regular non-directory file inside primaryDir (should be skipped by !entry.IsDir())
	looseFile := filepath.Join(primaryDir, "loose_file.txt")
	if err := fileutil.WriteFile(looseFile, []byte("ignore me"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write loose file: %v", err)
	}

	// Add a subfolder inside primaryDir that does NOT contain SKILL.md (should be ignored by stat err != nil)
	emptySubdir := filepath.Join(primaryDir, "not_a_skill_dir")
	if err := fileutil.MkdirAll(emptySubdir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create empty subdir: %v", err)
	}

	// Add a valid skill
	now := time.Now().Truncate(time.Second)
	createTestSkillFile(t, primaryDir, "valid-skill", now, "content")
	createTestSkillFile(t, secondaryDir, "valid-skill", now, "content")

	auditor := NewDriftAuditor(tempDir)
	issues, err := auditor.Audit()
	if err != nil {
		t.Fatalf("unexpected audit error: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected zero drift issues, got: %d (%v)", len(issues), issues)
	}
}

func TestDriftAuditor_AllDriftClassifications(t *testing.T) {
	tempDir := t.TempDir()
	primaryDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.SkillsSubdir)
	secondaryDir := filepath.Join(tempDir, "skills")

	baseTime := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	differentTime := baseTime.Add(2 * time.Hour)

	// 1. In sync
	createTestSkillFile(t, primaryDir, "skill-sync", baseTime, "sync content")
	createTestSkillFile(t, secondaryDir, "skill-sync", baseTime, "sync content")

	// 2. Missing in secondary
	createTestSkillFile(t, primaryDir, "skill-primary-only", baseTime, "primary only")

	// 3. Missing in primary
	createTestSkillFile(t, secondaryDir, "skill-secondary-only", baseTime, "secondary only")

	// 4. Mtime mismatch
	createTestSkillFile(t, primaryDir, "skill-mismatch", baseTime, "mismatch")
	createTestSkillFile(t, secondaryDir, "skill-mismatch", differentTime, "mismatch")

	auditor := NewDriftAuditor(tempDir)
	issues, err := auditor.Audit()
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}

	issueMap := make(map[string]DriftIssue)
	for _, iss := range issues {
		issueMap[iss.SkillName] = iss
	}

	if len(issueMap) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issueMap))
	}

	pOnly, ok := issueMap["skill-primary-only"]
	if !ok || pOnly.IssueType != "MissingInSecondary" {
		t.Errorf("expected MissingInSecondary, got: %+v", pOnly)
	}
	if !strings.Contains(pOnly.Description, "missing in") {
		t.Errorf("expected descriptive text, got %q", pOnly.Description)
	}

	sOnly, ok := issueMap["skill-secondary-only"]
	if !ok || sOnly.IssueType != "MissingInPrimary" {
		t.Errorf("expected MissingInPrimary, got: %+v", sOnly)
	}

	mismatch, ok := issueMap["skill-mismatch"]
	if !ok || mismatch.IssueType != "MtimeMismatch" {
		t.Errorf("expected MtimeMismatch, got: %+v", mismatch)
	}
}

func TestVerifySeal_MissingFrontmatterVariants(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{name: "empty content", content: ""},
		{name: "single line without frontmatter", content: "simple text body"},
		{name: "single delimiter only", content: "---only-one-delimiter"},
		{name: "two delimiters without body", content: "---\nkey: val\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := VerifySeal(tc.content)
			if res.Valid {
				t.Fatalf("expected invalid result for %s", tc.name)
			}
			if !strings.Contains(res.Error, "missing frontmatter") {
				t.Errorf("expected 'missing frontmatter' error, got: %s", res.Error)
			}
		})
	}
}

func TestVerifySeal_MissingRequiredFields(t *testing.T) {
	body := "\nBody content here\n"
	now := time.Now().UTC().Format(time.RFC3339)

	testCases := []struct {
		name        string
		frontmatter string
		errSubstr   string
	}{
		{
			name:        "missing seal_hash",
			frontmatter: fmt.Sprintf("seal_version: v1\nseal_issuer: zqk\nseal_date: %s", now),
			errSubstr:   "seal_hash not found",
		},
		{
			name:        "missing seal_version",
			frontmatter: fmt.Sprintf("seal_hash: 123456\nseal_issuer: zqk\nseal_date: %s", now),
			errSubstr:   "seal_version not found",
		},
		{
			name:        "missing seal_issuer",
			frontmatter: fmt.Sprintf("seal_hash: 123456\nseal_version: v1\nseal_date: %s", now),
			errSubstr:   "seal_issuer not found",
		},
		{
			name:        "missing seal_date",
			frontmatter: "seal_hash: 123456\nseal_version: v1\nseal_issuer: zqk",
			errSubstr:   "seal_date not found",
		},
		{
			name:        "invalid rfc3339 date",
			frontmatter: "seal_hash: 123456\nseal_version: v1\nseal_issuer: zqk\nseal_date: invalid-date",
			errSubstr:   "seal_date invalid RFC3339",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := fmt.Sprintf("---\n%s\n---%s", tc.frontmatter, body)
			res := VerifySeal(content)
			if res.Valid {
				t.Fatalf("expected invalid result for %s", tc.name)
			}
			if !strings.Contains(res.Error, tc.errSubstr) {
				t.Errorf("expected error containing %q, got: %s", tc.errSubstr, res.Error)
			}
		})
	}
}

func TestVerifySeal_QuotingAndWhitespaceHandling(t *testing.T) {
	body := "\n# Execution Guide\nPayload details."
	now := time.Date(2026, 10, 25, 14, 30, 0, 0, time.UTC)
	sealLines := GenerateSealData(body, "v2.1", "sentinel-core", now)

	// Inject extra whitespace and quote characters
	lines := strings.Split(sealLines, "\n")
	quotedLines := make([]string, len(lines))
	for i, l := range lines {
		parts := strings.SplitN(l, ": ", 2)
		quotedLines[i] = fmt.Sprintf("  %s:   \"%s\"  ", parts[0], parts[1])
	}

	content := "---\nname: 'complex-skill'\n" + strings.Join(quotedLines, "\n") + "\n---" + body
	res := VerifySeal(content)
	if !res.Valid {
		t.Fatalf("expected valid seal with quotes and whitespace, got error: %s", res.Error)
	}

	if res.Version != "v2.1" {
		t.Errorf("expected v2.1, got %q", res.Version)
	}
	if res.Issuer != "sentinel-core" {
		t.Errorf("expected sentinel-core, got %q", res.Issuer)
	}
	if !res.Date.Equal(now) {
		t.Errorf("expected date %v, got %v", now, res.Date)
	}
}

func TestVerifySeal_SingleQuoting(t *testing.T) {
	body := "\nBody with single quotes test."
	now := time.Date(2026, 10, 25, 15, 0, 0, 0, time.UTC)
	sealLines := GenerateSealData(body, "v3.0", "issuer-single", now)

	lines := strings.Split(sealLines, "\n")
	singleQuoted := make([]string, len(lines))
	for i, l := range lines {
		parts := strings.SplitN(l, ": ", 2)
		singleQuoted[i] = fmt.Sprintf("%s: '%s'", parts[0], parts[1])
	}

	content := "---\n" + strings.Join(singleQuoted, "\n") + "\n---" + body
	res := VerifySeal(content)
	if !res.Valid {
		t.Fatalf("expected valid seal with single quotes, got error: %s", res.Error)
	}
	if res.Version != "v3.0" || res.Issuer != "issuer-single" {
		t.Errorf("unexpected parsed result: %+v", res)
	}
}

func TestVerifySeal_EmptyBodyAndDelimitersInBody(t *testing.T) {
	emptyBody := ""
	now := time.Now().UTC().Truncate(time.Second)
	sealEmpty := GenerateSealData(emptyBody, "v1", "empty-test", now)
	contentEmpty := "---\n" + sealEmpty + "\n---" + emptyBody

	resEmpty := VerifySeal(contentEmpty)
	if !resEmpty.Valid {
		t.Fatalf("expected valid seal for empty body, got: %s", resEmpty.Error)
	}

	// Body with triple-dash delimiters inside markdown
	bodyWithDashes := "\n# Markdown Title\n---\nHorizontal rule above and below\n---\nEnd."
	sealWithDashes := GenerateSealData(bodyWithDashes, "v1", "dashes-test", now)
	contentWithDashes := "---\n" + sealWithDashes + "\n---" + bodyWithDashes

	resDashes := VerifySeal(contentWithDashes)
	if !resDashes.Valid {
		t.Fatalf("expected valid seal when body contains dashes, got: %s", resDashes.Error)
	}
}

func TestEvolutionaryTracking_VersionProgression(t *testing.T) {
	history := []struct {
		version string
		issuer  string
		body    string
		date    time.Time
	}{
		{
			version: "1.0.0",
			issuer:  "bootstrap-genesis",
			body:    "\n# Genesis Skill\nInitial logic.",
			date:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			version: "1.1.0",
			issuer:  "enhancement-actor",
			body:    "\n# Genesis Skill\nInitial logic.\nOptimized routines added.",
			date:    time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			version: "2.0.0",
			issuer:  "major-evolution-refactor",
			body:    "\n# Genesis Skill v2\nComplete structural overhaul.",
			date:    time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, h := range history {
		seal := GenerateSealData(h.body, h.version, h.issuer, h.date)
		content := "---\n" + seal + "\n---" + h.body

		res := VerifySeal(content)
		if !res.Valid {
			t.Fatalf("evolution verification failed at version %s: %s", h.version, res.Error)
		}
		if res.Version != h.version || res.Issuer != h.issuer || !res.Date.Equal(h.date) {
			t.Fatalf("evolution metadata mismatch at version %s: %+v", h.version, res)
		}
	}
}

func TestConcurrent_AuditAndSealOperations(t *testing.T) {
	tempDir := t.TempDir()
	primaryDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.SkillsSubdir)
	secondaryDir := filepath.Join(tempDir, "skills")

	now := time.Now().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("skill-%d", i)
		createTestSkillFile(t, primaryDir, name, now, fmt.Sprintf("content %d", i))
		createTestSkillFile(t, secondaryDir, name, now, fmt.Sprintf("content %d", i))
	}

	auditor := NewDriftAuditor(tempDir)
	const numGoroutines = 8

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*2)

	for i := 0; i < numGoroutines; i++ {
		workerID := i
		wg.Add(2)

		goroutinelabels.NewGoroutine("skill_test_audit", "concurrent skill audit").StartSimple(func() {
			defer wg.Done()
			issues, err := auditor.Audit()
			if err != nil {
				errCh <- fmt.Errorf("audit worker %d failed: %w", workerID, err)
				return
			}
			if len(issues) != 0 {
				errCh <- fmt.Errorf("audit worker %d found unexpected issues: %v", workerID, issues)
				return
			}
		})

		goroutinelabels.NewGoroutine("skill_test_seal", "concurrent skill seal").StartSimple(func() {
			defer wg.Done()
			body := fmt.Sprintf("\nWorker %d body content\n", workerID)
			seal := GenerateSealData(body, "v1", "concurrent-test", now)
			content := "---\n" + seal + "\n---" + body

			res := VerifySeal(content)
			if !res.Valid {
				errCh <- fmt.Errorf("seal worker %d failed: %s", workerID, res.Error)
				return
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent test failure: %v", err)
	}
}
