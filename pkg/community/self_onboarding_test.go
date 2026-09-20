package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentonboard"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCommunitySelfOnboarding_FunctionalAcceptance verifies that the self-onboarding engine
// detects vendor environments and executes a clean dry-run onboarding pass.
func TestCommunitySelfOnboarding_FunctionalAcceptance(t *testing.T) {
	tmpDir := t.TempDir()

	// Simulate a community workspace with .ide marker
	if err := fileutil.MkdirAll(filepath.Join(tmpDir, ".ide"), 0o755); err != nil {
		t.Fatalf("failed to create simulated ide marker: %v", err)
	}

	vendors := agentonboard.DetectVendors(tmpDir)
	if len(vendors) == 0 {
		t.Fatalf("expected vendor detection for .ide, got 0")
	}

	vector := agentonboard.InferVector(vendors)
	if vector != "A" {
		t.Fatalf("expected Vector A for IDE detected workspace, got %s", vector)
	}

	logger := logging.GetLoggerFromProfile("system")
	res, err := agentonboard.Run(agentonboard.Options{
		ProjectRoot: tmpDir,
		Logger:      logger,
		DryRun:      true,
		DetectOnly:  true,
	})
	if err != nil {
		t.Fatalf("agentonboard.Run failed in dry-run mode: %v", err)
	}

	if res.Vector != "A" {
		t.Errorf("expected result vector A, got %s", res.Vector)
	}
	if res.Status != "success" {
		t.Errorf("expected success status, got %s", res.Status)
	}
}

// TestCommunitySelfOnboarding_BoundaryAndErrorHandling verifies resilience against missing roots,
// headless fallback (Vector B), and invalid options.
func TestCommunitySelfOnboarding_BoundaryAndErrorHandling(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")

	// 1. Missing project root must error fail-closed
	_, errMissing := agentonboard.Run(agentonboard.Options{
		ProjectRoot: "",
		Logger:      logger,
	})
	if errMissing == nil {
		t.Error("expected error for empty project root, got nil")
	}

	// 2. Empty directory must default cleanly to Vector B (headless)
	emptyDir := t.TempDir()
	emptyVendors := agentonboard.DetectVendors(emptyDir)
	if len(emptyVendors) != 0 {
		t.Errorf("expected 0 vendors for clean dir, got %d", len(emptyVendors))
	}
	if v := agentonboard.InferVector(emptyVendors); v != "B" {
		t.Errorf("expected Vector B for empty dir, got %s", v)
	}

	resHeadless, errHeadless := agentonboard.Run(agentonboard.Options{
		ProjectRoot: emptyDir,
		Logger:      logger,
		DryRun:      true,
		Headless:    true,
	})
	if errHeadless != nil {
		t.Fatalf("expected headless dry run to succeed, got: %v", errHeadless)
	}
	if resHeadless.Vector != "B" {
		t.Errorf("expected Vector B, got %s", resHeadless.Vector)
	}
}

// TestCommunitySelfOnboarding_IntegrationAndConformance verifies that the standalone community
// candidate tree has all required onboarding documentation and entry points for strangers.
func TestCommunitySelfOnboarding_IntegrationAndConformance(t *testing.T) {
	candidateDir := publicCandidateFixture(t)

	// 1. Verify COMMUNITY_FIRST_RUN.md exists in candidate docs
	guidePath := filepath.Join(candidateDir, "docs", "onboarding", "COMMUNITY_FIRST_RUN.md")
	if !fileutil.Exists(guidePath) {
		t.Fatalf("missing required COMMUNITY_FIRST_RUN.md in candidate at %s", guidePath)
	}

	// 2. Verify candidate README references COMMUNITY_FIRST_RUN.md
	readmePath := filepath.Join(candidateDir, "README.md")
	readmeContent, err := fileutil.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read candidate README.md: %v", err)
	}
	if !strings.Contains(string(readmeContent), "COMMUNITY_FIRST_RUN.md") {
		t.Errorf("candidate README.md must link to COMMUNITY_FIRST_RUN.md")
	}

	// 3. Verify vendor detection runs cleanly on candidate directory without error
	vendors := agentonboard.DetectVendors(candidateDir)
	_ = agentonboard.InferVector(vendors)

	// 4. Verify candidate CLI help works if candidate binary is present
	binPath := filepath.Join(candidateDir, "bin", "zqk-community")
	if fileutil.Exists(binPath) {
		cmd := execwrap.Command(binPath, "--help")
		out, errCmd := cmd.CombinedOutput()
		if errCmd != nil {
			t.Errorf("candidate binary failed --help: %v, output: %s", errCmd, string(out))
		}
	}
}
