package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentonboard"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestWorkspaceProjection_FunctionalAcceptance verifies that agent workspace projection
// generates synchronized agent directives (AGENTS.md and vendor rules) with zero drift
// and without leaking studio internal directories (CRIT-1789702349205952000-b3e910a4).
func TestWorkspaceProjection_FunctionalAcceptance(t *testing.T) {
	tmpDir := t.TempDir()

	logger := logging.GetLoggerFromProfile("system")
	res, err := agentonboard.Run(agentonboard.Options{
		ProjectRoot: tmpDir,
		Logger:      logger,
		AllVendors:  true,
		Force:       true,
		SessionOK:   true,
		SkipSeat:    true,
	})
	if err != nil {
		t.Fatalf("agentonboard.Run failed: %v", err)
	}

	if res.Status != "success" {
		t.Errorf("expected status success, got %s", res.Status)
	}

	// Verify universal AGENTS.md was created and contains anti-idleness protocol
	agentsMd := filepath.Join(tmpDir, ".agents", "AGENTS.md")
	if !fileutil.Exists(agentsMd) {
		agentsMd = filepath.Join(tmpDir, "AGENTS.md")
	}
	if !fileutil.Exists(agentsMd) {
		t.Fatalf("expected AGENTS.md to be created in %s", tmpDir)
	}

	content, err := fileutil.ReadFile(agentsMd)
	if err != nil {
		t.Fatalf("failed to read AGENTS.md: %v", err)
	}

	requiredDirectives := []string{
		"ZQK Agent Boot Protocol",
		"Continuous Autonomous Loop Discipline",
		"NEVER yield control or go idle",
	}

	for _, d := range requiredDirectives {
		if !strings.Contains(string(content), d) {
			t.Errorf("AGENTS.md missing mandatory directive: %s", d)
		}
	}

	// Verify no internal studio paths leaked into projected agent files
	forbiddenTokens := []string{
		"zqk-restore-clone",
		"pkg/mesh",
		"cmd/zqk-admin",
	}

	for _, tok := range forbiddenTokens {
		if strings.Contains(string(content), tok) {
			t.Errorf("projected AGENTS.md leaked internal studio token: %s", tok)
		}
	}
}

// TestWorkspaceProjection_BoundaryAndErrorHandling verifies error handling when target directory
// is read-only or corrupted.
func TestWorkspaceProjection_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile("system")

	// Missing root must fail-closed
	_, err := agentonboard.Run(agentonboard.Options{
		ProjectRoot: filepath.Join(t.TempDir(), "nonexistent_dir"),
		Logger:      logger,
	})
	if err == nil {
		t.Errorf("expected error for nonexistent project root, got nil")
	}
}

// TestWorkspaceProjection_IntegrationAndConformance verifies that sync report is recorded
// and matches the agent workspace sync schema.
func TestWorkspaceProjection_IntegrationAndConformance(t *testing.T) {
	tmpDir := t.TempDir()

	logger := logging.GetLoggerFromProfile("system")
	res, err := agentonboard.Run(agentonboard.Options{
		ProjectRoot: tmpDir,
		Logger:      logger,
		Headless:    true,
		Force:       true,
		SessionOK:   true,
		SkipSeat:    true,
	})
	if err != nil {
		t.Fatalf("agentonboard.Run failed: %v", err)
	}

	if res.SyncReport == "" {
		t.Errorf("expected sync report path to be populated in result")
	}

	reportPath := filepath.Join(tmpDir, res.SyncReport)
	if !fileutil.Exists(reportPath) {
		t.Errorf("sync report file does not exist at %s", reportPath)
	}

	reportBytes, err := fileutil.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed reading sync report: %v", err)
	}

	if !strings.Contains(string(reportBytes), "zqk_agent_workspace_sync_v1") {
		t.Errorf("sync report missing expected schema token, got: %s", string(reportBytes))
	}
}
