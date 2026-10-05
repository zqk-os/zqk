package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestCLIManual_FunctionalAcceptance validates that the CLI User Manual exists,
// covers all core command families, global flags, and environment variables.
// (CRIT-1789627326377073000-baa0cede / BLI-1789627326377073000-79ae994e)
func TestCLIManual_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	manualPath := filepath.Join(root, "docs", "manual", "CLI_REFERENCE.md")

	if !fileutil.Exists(manualPath) {
		t.Fatalf("CLI reference manual not found at %s", manualPath)
	}

	contentBytes, err := fileutil.ReadFile(manualPath)
	if err != nil {
		t.Fatalf("failed reading CLI reference manual: %v", err)
	}
	content := string(contentBytes)

	requiredSections := []string{
		"Global Flags & Conventions",
		"Getting Started",
		"Everyday Commands",
		"Integrations & Daemon Operations",
		"Advanced Knowledge Kernel Commands",
		"Concrete Operator Workflows",
		"Exit Codes & Error Verbs",
		"Environment Variables",
	}

	for _, sec := range requiredSections {
		if !strings.Contains(content, sec) {
			t.Errorf("manual missing required section: %q", sec)
		}
	}

	// Verify all 33 top-level commands are documented in CLI_REFERENCE.md
	requiredCommands := []string{
		// Getting started
		"zqk quickstart",
		// Everyday
		"zqk workflow",
		"zqk object",
		"zqk system",
		"zqk pplan",
		"zqk grep",
		"zqk test",
		"zqk auth",
		"zqk do",
		"zqk validate",
		"zqk version",
		// Integrations
		"zqk scheduler",
		"zqk mcp",
		"zqk feed",
		"zqk inbox",
		"zqk intake",
		"zqk keystore",
		"zqk learn",
		"zqk new",
		"zqk pre-commit",
		"zqk reports",
		"zqk tray",
		"zqk automation",
		"zqk callback",
		"zqk ci",
		"zqk completion",
		// Advanced
		"zqk agent",
		"zqk swarm",
		"zqk ambient",
		"zqk convergence",
		"zqk docman",
		"zqk domain",
		"zqk graph",
		"zqk join",
		"zqk matrix",
		"zqk mesh",
		"zqk observer",
		"zqk ontology",
		"zqk ops",
		"zqk organizational",
		"zqk rollback",
		"zqk semantic",
		"zqk spec",
	}

	for _, cmd := range requiredCommands {
		if !strings.Contains(content, cmd) {
			t.Errorf("manual missing command reference: %q", cmd)
		}
	}

	// Verify key global flags
	requiredFlags := []string{
		"--context",
		"--format",
		"--timeout",
		"--allow-degraded",
		"--help",
		"--version",
	}
	for _, flag := range requiredFlags {
		if !strings.Contains(content, flag) {
			t.Errorf("manual missing global flag reference: %q", flag)
		}
	}

	// Verify environment variables
	requiredEnvs := []string{
		zqkenv.ProjectRoot().Name(),
		zqkenv.ContextProfile().Name(),
		zqkenv.LogLevel().Name(),
		zqkenv.Timeout().Name(),
		zqkenv.AllowDegraded().Name(),
	}
	for _, envVar := range requiredEnvs {
		if !strings.Contains(content, envVar) {
			t.Errorf("manual missing environment variable: %q", envVar)
		}
	}
}

// TestCLIManual_BoundaryAndErrorHandling validates error verbs, exit codes,
// context profiles, formats, and boundary conditions documented in the manual.
// (CRIT-1789627326377074000-befed66b / BLI-1789627326377073000-79ae994e)
func TestCLIManual_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	manualPath := filepath.Join(root, "docs", "manual", "CLI_REFERENCE.md")

	contentBytes, err := fileutil.ReadFile(manualPath)
	if err != nil {
		t.Fatalf("failed reading CLI reference manual: %v", err)
	}
	content := string(contentBytes)

	// Check documented exit codes
	requiredCodes := []string{"`0`", "`1`", "`2`", "`137`"}
	for _, code := range requiredCodes {
		if !strings.Contains(content, code) {
			t.Errorf("manual missing documented exit code: %s", code)
		}
	}

	// Check documented context profiles and formatting options
	requiredContexts := []string{"ai-agent", "human", "debug"}
	for _, ctx := range requiredContexts {
		if !strings.Contains(content, ctx) {
			t.Errorf("manual missing documented context profile: %s", ctx)
		}
	}

	requiredFormats := []string{"table", "json", "yaml", "stream", "json-rpc"}
	for _, fmtStr := range requiredFormats {
		if !strings.Contains(content, fmtStr) {
			t.Errorf("manual missing documented format: %s", fmtStr)
		}
	}
}

// TestCLIManual_IntegrationAndConformance validates documentation portal integration,
// symlink conformance, and clean navigation links across the Divio quadrants.
// (CRIT-1789627326377075000-8eb816a8 / BLI-1789627326377073000-79ae994e)
func TestCLIManual_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	manualPath := filepath.Join(root, "docs", "manual", "CLI_REFERENCE.md")
	refSymlink := filepath.Join(root, "docs", "reference", "CLI_REFERENCE.md")
	portalReadme := filepath.Join(root, "docs", "reference", "README.md")
	indexDoc := filepath.Join(root, "docs", "INDEX.md")

	if !fileutil.Exists(manualPath) {
		t.Errorf("target manual does not exist: %s", manualPath)
	}

	if !fileutil.Exists(refSymlink) {
		t.Errorf("reference symlink does not exist: %s", refSymlink)
	}

	portalBytes, err := fileutil.ReadFile(portalReadme)
	if err != nil {
		t.Fatalf("failed reading docs/reference/README.md: %v", err)
	}
	if !strings.Contains(string(portalBytes), "CLI_REFERENCE.md") {
		t.Errorf("docs/reference/README.md does not reference CLI_REFERENCE.md")
	}

	indexBytes, err := fileutil.ReadFile(indexDoc)
	if err != nil {
		t.Fatalf("failed reading docs/INDEX.md: %v", err)
	}
	if !strings.Contains(string(indexBytes), "CLI_REFERENCE.md") {
		t.Errorf("docs/INDEX.md does not reference CLI_REFERENCE.md")
	}
}
