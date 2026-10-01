package system

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestQuickstartCommand(t *testing.T) {
	cmd := NewQuickstartCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf))
	cmd.SetArgs([]string{"--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("quickstart command failed: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("ZQK Quickstart Guide")) {
		t.Errorf("expected json output to contain quickstart title, got: %s", buf.String())
	}

	if !bytes.Contains(buf.Bytes(), []byte("--with-onboarding-roadmap")) {
		t.Errorf("expected json output to include --with-onboarding-roadmap in system init step, got: %s", buf.String())
	}
}

// BLI-1788987316012200000-5097aded: Deprecate 'quick' CLI command family in favor of 'new' / 'quickstart'
func TestQuickstartCommand_BLI_1788987316012200000_5097aded_DeprecateQuick(t *testing.T) {
	cmd := NewQuickstartCmd()
	if cmd.Use != "quickstart" {
		t.Errorf("expected quickstart command Use='quickstart', got %q", cmd.Use)
	}
}

// CRIT-DOCS-QUICKSTART-001: Zero-Friction 5-Minute Onboarding Tutorial Completeness
func TestQuickstartDocumentationCompleteness(t *testing.T) {
	// Find project root
	cwd := "."
	for i := 0; i < 5; i++ {
		if _, err := fileutil.Stat(filepath.Join(cwd, "docs", "onboarding", "QUICKSTART.md")); err == nil {
			break
		}
		cwd = filepath.Join("..", cwd)
	}
	docPath := filepath.Join(cwd, "docs", "onboarding", "QUICKSTART.md")
	content, err := fileutil.ReadFile(docPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", docPath, err)
	}

	text := string(content)
	if !strings.Contains(text, "zqk quickstart") {
		t.Errorf("QUICKSTART.md missing 'zqk quickstart' command")
	}
	if !strings.Contains(text, "zqk mcp install") {
		t.Errorf("QUICKSTART.md missing 'zqk mcp install' automated setup command")
	}
	if !strings.Contains(text, "cursor-adapter") {
		t.Errorf("QUICKSTART.md missing 'cursor-adapter' manual config reference")
	}
	if strings.Contains(text, `"command": "zqk-mcp"`) {
		t.Errorf("QUICKSTART.md contains obsolete standalone 'zqk-mcp' command reference")
	}
	if !strings.Contains(text, "agent-onboard") {
		t.Errorf("QUICKSTART.md missing 'agent-onboard' cross-reference")
	}

	communityPath := filepath.Join(cwd, "docs", "onboarding", "COMMUNITY_FIRST_RUN.md")
	communityContent, err := fileutil.ReadFile(communityPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", communityPath, err)
	}
	commText := string(communityContent)
	if !strings.Contains(commText, "zqk quickstart") {
		t.Errorf("COMMUNITY_FIRST_RUN.md missing 'zqk quickstart' reference")
	}
	if !strings.Contains(commText, "zqk mcp install") {
		t.Errorf("COMMUNITY_FIRST_RUN.md missing 'zqk mcp install' reference")
	}
	if !strings.Contains(commText, "QUICKSTART.md") {
		t.Errorf("COMMUNITY_FIRST_RUN.md missing link to QUICKSTART.md")
	}
}

func TestContributorGuideAndTemplateCompleteness(t *testing.T) {
	// Find project root
	cwd := "."
	for i := 0; i < 5; i++ {
		if _, err := fileutil.Stat(filepath.Join(cwd, "CONTRIBUTING.md")); err == nil {
			break
		}
		cwd = filepath.Join("..", cwd)
	}

	contribPath := filepath.Join(cwd, "CONTRIBUTING.md")
	content, err := fileutil.ReadFile(contribPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", contribPath, err)
	}
	text := string(content)

	if !strings.Contains(text, "zqk workflow whats-next") {
		t.Errorf("CONTRIBUTING.md missing 'zqk workflow whats-next'")
	}
	if !strings.Contains(text, "zqk system check") {
		t.Errorf("CONTRIBUTING.md missing 'zqk system check'")
	}
	if !strings.Contains(text, "PULL_REQUEST_TEMPLATE.md") {
		t.Errorf("CONTRIBUTING.md missing reference to PULL_REQUEST_TEMPLATE.md")
	}

	prTemplatePath := filepath.Join(cwd, ".github", "PULL_REQUEST_TEMPLATE.md")
	prContent, err := fileutil.ReadFile(prTemplatePath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", prTemplatePath, err)
	}
	prText := string(prContent)

	if !strings.Contains(prText, "Priority Plan:") {
		t.Errorf("PR template missing Priority Plan field")
	}
	if !strings.Contains(prText, "Backlog Items:") {
		t.Errorf("PR template missing Backlog Items field")
	}
	if !strings.Contains(prText, "zqk system check") {
		t.Errorf("PR template missing zqk system check integrity gate")
	}
}

func TestContributorGuide_IssueTemplates(t *testing.T) {
	cwd := "."
	for i := 0; i < 5; i++ {
		if _, err := fileutil.Stat(filepath.Join(cwd, ".github", "ISSUE_TEMPLATE")); err == nil {
			break
		}
		cwd = filepath.Join("..", cwd)
	}

	templates := []struct {
		filename string
		required []string
	}{
		{
			filename: "bug.yml",
			required: []string{"Bug report", "Reproduction", "Environment"},
		},
		{
			filename: "feature.yml",
			required: []string{"Enhancement", "Problem", "Proposal"},
		},
	}

	for _, tmpl := range templates {
		path := filepath.Join(cwd, ".github", "ISSUE_TEMPLATE", tmpl.filename)
		content, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read template %s: %v", path, err)
		}
		text := string(content)
		for _, req := range tmpl.required {
			if !strings.Contains(text, req) {
				t.Errorf("template %s missing required section %q", tmpl.filename, req)
			}
		}
	}
}

func TestContributorGuide_BoundaryErrors(t *testing.T) {
	// Boundary test: verify that reading a non-existent template path returns an error
	nonExistent := filepath.Join(t.TempDir(), "non_existent_template.md")
	_, err := fileutil.ReadFile(nonExistent)
	if err == nil {
		t.Errorf("expected error reading non-existent template path, got nil")
	}
}
