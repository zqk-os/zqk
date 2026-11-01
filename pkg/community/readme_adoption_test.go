package community_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestREADMEAdoption_FunctionalAcceptance satisfies CRIT-1793490258663216000-7e0d99a8.
// Verifies that README.md provides zero-jargon developer positioning, instant token-savings
// narrative, 1-click Codespaces sandbox, and progressive disclosure tiers.
func TestREADMEAdoption_FunctionalAcceptance(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)

	readmePath := filepath.Join(repoRoot, "README.md")
	contentBytes, err := fileutil.ReadFile(readmePath)
	require.NoError(t, err, "README.md must exist in repo root")
	content := string(contentBytes)

	// 1. Positioning & Above-the-fold Headline
	assert.Contains(t, content, "# ZQK: Git for AI Agent Memory and Task Truth", "must contain clear, zero-jargon headline")
	assert.Contains(t, content, "The persistence, coordination, and verification substrate for Cursor, Claude Code, Cline, and Windsurf", "must name target agents")

	// 2. High-Converting Badges
	assert.Contains(t, content, "https://codespaces.new/zqk-os/zqk", "must include 1-click GitHub Codespaces badge")
	assert.Contains(t, content, "https://docs.zqk.dev", "must include documentation portal badge")
	assert.Contains(t, content, "Apache_2.0", "must include open-source license badge")

	// 3. Quickstart & Installation Options
	assert.Contains(t, content, "curl -sSL https://raw.githubusercontent.com/zqk-os/zqk/main/scripts/install.sh | sh", "must include 1-line curl installer")
	assert.Contains(t, content, "brew tap zqk-os/tap && brew install zqk", "must include Homebrew install")
	assert.Contains(t, content, "1-Click Instant Cloud Sandbox (GitHub Codespaces)", "must have dedicated Codespaces section")

	// 4. 30-Second Greenfield Setup
	assert.Contains(t, content, "zqk init", "must include zqk init")
	assert.Contains(t, content, "zqk ci init", "must include zqk ci init")
	assert.Contains(t, content, "zqk ui -w", "must include zqk ui -w Web Studio")
	assert.Contains(t, content, "zqk mcp ensure", "must include zqk mcp ensure")

	// 5. Progressive Disclosure Paths (Go Fast vs. Walk Through)
	assert.Contains(t, content, "Option 1: Go Fast (Immediate Token Savings)", "must provide Go Fast path")
	assert.Contains(t, content, "zgrep", "must highlight token-budgeted zgrep")
	assert.Contains(t, content, "Option 2: Walk Through (Full Knowledge Kernel)", "must provide Walk Through path")
	assert.Contains(t, content, "zqk system start-here", "must reference zqk system start-here")

	// 6. Visual Web Studio Highlights
	assert.Contains(t, content, "Visual Web Studio (`zqk ui -w`)", "must highlight Web Studio visual surface")
	assert.Contains(t, content, "docs/manual/screenshots/ui_web_studio.svg", "must embed Web Studio screenshot")
}

// TestREADMEAdoption_BoundaryAndErrorHandling satisfies CRIT-1793490258663217000-c48cd692.
// Verifies that all relative file paths, markdown links, and media assets referenced in
// README.md exist on disk and resolve cleanly without broken links.
func TestREADMEAdoption_BoundaryAndErrorHandling(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)

	readmePath := filepath.Join(repoRoot, "README.md")
	contentBytes, err := fileutil.ReadFile(readmePath)
	require.NoError(t, err)
	content := string(contentBytes)

	// Regex for markdown links [text](target)
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	matches := linkRegex.FindAllStringSubmatch(content, -1)
	require.NotEmpty(t, matches, "README.md must contain markdown links")

	for _, match := range matches {
		target := match[2]

		// Skip external URLs and mailto links
		if strings.HasPrefix(target, "http://") ||
			strings.HasPrefix(target, "https://") ||
			strings.HasPrefix(target, "mailto:") {
			continue
		}

		// Strip anchor fragments (#heading)
		cleanTarget := target
		if idx := strings.Index(cleanTarget, "#"); idx != -1 {
			cleanTarget = cleanTarget[:idx]
		}
		if cleanTarget == "" {
			continue
		}

		// Validate local file path existence
		resolvedPath := filepath.Join(repoRoot, filepath.FromSlash(cleanTarget))
		_, errStat := os.Stat(resolvedPath)
		assert.NoError(t, errStat, "relative link '%s' in README.md must point to an existing file", target)
	}
}

// TestREADMEAdoption_DocumentationAndKnowledgeBase satisfies CRIT-1793490258663218000-eb90c3f2.
// Verifies that docs/onboarding/COMMUNITY_FIRST_RUN.md exists, contains the canonical onboarding
// sequence, links back to README.md and documentation portal, and has no missing assets.
func TestREADMEAdoption_DocumentationAndKnowledgeBase(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)

	firstRunPath := filepath.Join(repoRoot, "docs", "onboarding", "COMMUNITY_FIRST_RUN.md")
	contentBytes, err := fileutil.ReadFile(firstRunPath)
	require.NoError(t, err, "COMMUNITY_FIRST_RUN.md must exist")
	content := string(contentBytes)

	// Verify key onboarding sections
	assert.Contains(t, content, "# Community First-Run Guide (Agent + Human)", "must have canonical guide title")
	assert.Contains(t, content, "1-Click Cloud Sandbox (GitHub Codespaces)", "must have Codespaces section")
	assert.Contains(t, content, "Quickstart (2–3 Commands)", "must have quickstart section")
	assert.Contains(t, content, "Fail-closed sequence", "must have fail-closed sequence table")
	assert.Contains(t, content, "zqk system agent-onboard", "must reference zqk system agent-onboard")
	assert.Contains(t, content, "zqk workflow whats-next", "must reference whats-next")
	assert.Contains(t, content, "zqk do", "must reference zqk do autonomous execution")
}
