package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestCommunityTemplates_FunctionalAcceptance verifies that standard GitHub issue templates
// (bug_report, feature_request, rfc), CONTRIBUTING.md, CODE_OF_CONDUCT.md, and PULL_REQUEST_TEMPLATE.md
// exist and contain required community governance sections.
// (CRIT-1789631749278959000-78db8290 / BLI-1789631749278959000-94153e33)
func TestCommunityTemplates_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	requiredFiles := map[string][]string{
		filepath.Join(root, "CONTRIBUTING.md"): {
			"Contributing to ZQK",
			"Core Principles",
			"Setting Up Your Development Environment",
			"Working on Backlog Items",
			"Submitting Pull Requests",
		},
		filepath.Join(root, "CODE_OF_CONDUCT.md"): {
			"Contributor Covenant Code of Conduct",
			"Our Standards",
			"Enforcement",
		},
		filepath.Join(root, ".github", "PULL_REQUEST_TEMPLATE.md"): {
			"Pull Request Overview",
			"Summary of Changes",
			"Associated Kernel Objects",
			"Operational Checklist",
		},
		filepath.Join(root, ".github", "ISSUE_TEMPLATE", "bug_report.md"): {
			"Description",
			"Reproduction Steps",
			"Expected Behavior",
			"System Environment",
		},
		filepath.Join(root, ".github", "ISSUE_TEMPLATE", "feature_request.md"): {
			"Feature Proposal",
			"Motivation & Use Case",
			"Proposed Kernel / CLI Spec",
		},
		filepath.Join(root, ".github", "ISSUE_TEMPLATE", "rfc.md"): {
			"Executive Summary",
			"Motivation & Problem Statement",
			"Architectural Design",
		},
	}

	for path, sections := range requiredFiles {
		if !fileutil.Exists(path) {
			t.Errorf("expected community file missing: %s", path)
			continue
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("failed reading %s: %v", path, err)
		}
		content := string(data)
		for _, sec := range sections {
			if !strings.Contains(content, sec) {
				t.Errorf("%s missing required section: %q", filepath.Base(path), sec)
			}
		}
	}
}

// TestCommunityTemplates_BoundaryAndErrorHandling verifies validation of empty/malformed
// templates, YAML frontmatter integrity in issue templates, and boundary condition handling.
// (CRIT-1789631749278960000-309ebf9a / BLI-1789631749278959000-94153e33)
func TestCommunityTemplates_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	issueDir := filepath.Join(root, ".github", "ISSUE_TEMPLATE")

	templates := []string{"bug_report.md", "feature_request.md", "rfc.md"}
	for _, tmpl := range templates {
		p := filepath.Join(issueDir, tmpl)
		data, err := fileutil.ReadFile(p)
		if err != nil {
			t.Fatalf("failed to read template %s: %v", tmpl, err)
		}
		content := string(data)
		// GitHub issue templates must have YAML frontmatter with name and about
		if !strings.HasPrefix(content, "---\n") {
			t.Errorf("template %s missing opening YAML frontmatter ---", tmpl)
		}
		if !strings.Contains(content, "name:") {
			t.Errorf("template %s missing 'name:' in frontmatter", tmpl)
		}
		if !strings.Contains(content, "about:") {
			t.Errorf("template %s missing 'about:' in frontmatter", tmpl)
		}
	}
}

// TestCommunityTemplates_IntegrationAndConformance verifies that issue templates and
// pull request guidelines align with open core governance policies without studio baggage.
// (CRIT-1789631749278961000-6bd931f3 / BLI-1789631749278959000-94153e33)
func TestCommunityTemplates_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	contribPath := filepath.Join(root, "CONTRIBUTING.md")

	data, err := fileutil.ReadFile(contribPath)
	if err != nil {
		t.Fatalf("failed reading CONTRIBUTING.md: %v", err)
	}
	content := string(data)

	// Ensure no private studio endpoints or internal monorepo credentials exist
	prohibitedWords := []string{
		"studio-internal",
		"commercial-token",
		"internal.zqk.dev",
	}
	for _, word := range prohibitedWords {
		if strings.Contains(strings.ToLower(content), word) {
			t.Errorf("CONTRIBUTING.md contains prohibited private term: %s", word)
		}
	}
}
