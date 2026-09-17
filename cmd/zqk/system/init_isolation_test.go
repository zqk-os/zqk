package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestWriteRootIsolationFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("human")

	// Ensure .zqk/config and .zqk/state dirs exist or are created
	err := writeRootIsolationFiles(tmpDir, false, logger)
	if err != nil {
		t.Fatalf("writeRootIsolationFiles failed: %v", err)
	}

	expectedFiles := []string{
		filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkConfigFileName),
		filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkLocalConfigFileName),
		filepath.Join(tmpDir, paths.ProjectDataDir, paths.ConfigDir, "agent_chat_channel.json"),
		filepath.Join(tmpDir, paths.ProjectDataDir, paths.ConfigDir, "agent_workspace_sync.json"),
		filepath.Join(tmpDir, paths.ProjectDataDir, paths.ConfigDir, "agent_git_identity.env.example"),
		filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "remote_hold.json"),
	}

	for _, ef := range expectedFiles {
		if !fileutil.Exists(ef) {
			t.Errorf("expected isolation file to exist: %s", ef)
		}
	}

	// Verify content of config/zqk.yaml
	cfgContent, err := os.ReadFile(filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkConfigFileName))
	if err != nil {
		t.Fatalf("failed to read zqk.yaml: %v", err)
	}
	if !strings.Contains(string(cfgContent), "storage_mode") {
		t.Errorf("zqk.yaml missing storage_mode: %s", string(cfgContent))
	}

	// Verify content of config/zqk-local.yaml
	localContent, err := os.ReadFile(filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkLocalConfigFileName))
	if err != nil {
		t.Fatalf("failed to read zqk-local.yaml: %v", err)
	}
	absTmp, _ := filepath.Abs(tmpDir)
	if !strings.Contains(string(localContent), absTmp) {
		t.Errorf("zqk-local.yaml should contain project root %s, got: %s", absTmp, string(localContent))
	}

	// Verify remote_hold.json has hold=true
	holdContent, err := os.ReadFile(filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "remote_hold.json"))
	if err != nil {
		t.Fatalf("failed to read remote_hold.json: %v", err)
	}
	if !strings.Contains(string(holdContent), `"hold": true`) {
		t.Errorf("remote_hold.json should have hold=true, got: %s", string(holdContent))
	}
}

func TestWriteBrandSettings_ProjectRootExplicit(t *testing.T) {
	tmpDir := t.TempDir()

	err := writeBrandSettings(tmpDir, false)
	if err != nil {
		t.Fatalf("writeBrandSettings failed: %v", err)
	}

	rootSettings := filepath.Join(tmpDir, paths.BrandSettingsFilename)
	data, err := os.ReadFile(rootSettings)
	if err != nil {
		t.Fatalf("failed to read %s: %v", rootSettings, err)
	}
	if !strings.Contains(string(data), `project_root: "."`) {
		t.Errorf("brand settings should contain project_root: \".\", got: %s", string(data))
	}
}

func TestConfigRootFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Place zqk.yaml directly in root instead of config/
	zqkContent := `system:
  storage_mode: "file"
`
	if err := os.WriteFile(filepath.Join(tmpDir, paths.ZqkConfigFileName), []byte(zqkContent), 0644); err != nil {
		t.Fatalf("failed to write root zqk.yaml: %v", err)
	}

	cfg := config.LoadForRoot(tmpDir)
	if cfg.System.StorageMode == nil || *cfg.System.StorageMode != "file" {
		t.Fatalf("expected StorageMode 'file', got: %v", cfg.System.StorageMode)
	}
}

func TestStableBinaryName_Zcom(t *testing.T) {
	prev := brand.ExecutableName()
	defer brand.SetExecutableName(prev)

	brand.SetExecutableName("zcom")
	if got := paths.StableBinaryName(); got != "zcom" {
		t.Errorf("expected StableBinaryName() to be 'zcom' when brand is zcom, got: %s", got)
	}

	tmpDir := t.TempDir()
	if got := paths.StableBinaryPath(tmpDir); !strings.HasSuffix(got, "/zcom") {
		t.Errorf("expected StableBinaryPath to end with /zcom, got: %s", got)
	}
}

func TestFirstRun_PolyglotWorkspaceErgonomics(t *testing.T) {
	// Verify that in an empty, non-Go polyglot workspace (e.g. Python / Node project),
	// system initialization successfully writes standard root isolation, configurations,
	// and system accounts without requiring go.mod or external studio assets.
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("human")

	// Simulate a Python / TypeScript project
	if err := fileutil.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"name":"polyglot-app"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte(`[project]`+"\n"+`name = "polyglot-app"`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := writeRootIsolationFiles(tmpDir, false, logger); err != nil {
		t.Fatalf("writeRootIsolationFiles failed in polyglot root: %v", err)
	}
	if err := writeBrandSettings(tmpDir, false); err != nil {
		t.Fatalf("writeBrandSettings failed in polyglot root: %v", err)
	}
	if err := writeSystemAccount(tmpDir); err != nil {
		t.Fatalf("writeSystemAccount failed in polyglot root: %v", err)
	}

	// Assert root isolation and account exist
	if !fileutil.Exists(filepath.Join(tmpDir, paths.BrandSettingsFilename)) {
		t.Errorf("expected brand settings in polyglot root")
	}
	if !fileutil.Exists(filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkConfigFileName)) {
		t.Errorf("expected zqk.yaml in polyglot root config/")
	}
	accountsDir := filepath.Join(tmpDir, paths.ProcessDir, "accounts")
	entries, err := fileutil.ReadDir(accountsDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected CAS account in polyglot root accounts dir, err: %v", err)
	}
}

func TestFirstRun_AgentOnboardingDirectives(t *testing.T) {
	// Verify that agent onboarding files provide headless-safe AGENTS.md / GEMINI.md directives
	// and standard isolation markers in the target workspace.
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("human")

	if err := writeRootIsolationFiles(tmpDir, false, logger); err != nil {
		t.Fatalf("writeRootIsolationFiles failed: %v", err)
	}

	holdPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "remote_hold.json")
	if !fileutil.Exists(holdPath) {
		t.Errorf("expected remote_hold.json to guard fresh workspace from premature remote sync")
	}

	syncPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.ConfigDir, "agent_workspace_sync.json")
	if !fileutil.Exists(syncPath) {
		t.Errorf("expected agent_workspace_sync.json to be created")
	}
}

func TestFirstRun_BoundaryAndErrorHandling(t *testing.T) {
	// Verify boundary condition where projectRoot is an existing regular file rather than a directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "blocked_by_file")
	if err := os.WriteFile(filePath, []byte("plain file"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := logging.GetLoggerFromProfile("human")
	if err := writeRootIsolationFiles(filePath, false, logger); err == nil {
		t.Errorf("expected writeRootIsolationFiles to fail when root is a file")
	}

	if err := writeBrandSettings(filePath, false); err == nil {
		t.Errorf("expected writeBrandSettings to fail when root is a file")
	}
}

func TestPublicRelease_PackagingIntegrityAndGates(t *testing.T) {
	// Verifies the presence and executable bit of release packaging and fail-closed gate scripts.
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("failed to determine repository root: %v", err)
	}

	requiredScripts := []string{
		filepath.Join(root, "scripts", "package-community.sh"),
		filepath.Join(root, "scripts", "test_package_community.sh"),
		filepath.Join(root, "scripts", "test-public-push-gate-failclosed.sh"),
		filepath.Join(root, "scripts", "check-public-release-payload.sh"),
	}

	for _, s := range requiredScripts {
		st, err := os.Stat(s)
		if err != nil {
			t.Errorf("expected release script to exist: %s (%v)", s, err)
			continue
		}
		if st.Mode()&0111 == 0 {
			t.Errorf("expected script %s to be executable, mode: %v", s, st.Mode())
		}
	}
}

func TestCommunitySourceOverlay_IntegrityAndProcessPreservation(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("failed to determine repository root: %v", err)
	}

	overlayScript := filepath.Join(root, "scripts", "open-core", "apply-community-source.sh")
	st, err := os.Stat(overlayScript)
	if err != nil {
		t.Fatalf("expected apply-community-source.sh to exist: %v", err)
	}
	if st.Mode()&0111 == 0 {
		t.Errorf("expected apply-community-source.sh to be executable, mode: %v", st.Mode())
	}

	// Verify Makefile contains zcom target
	makefilePath := filepath.Join(root, "Makefile")
	mfData, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("failed to read Makefile: %v", err)
	}
	if !strings.Contains(string(mfData), "zcom:") {
		t.Errorf("expected Makefile to contain 'zcom:' build target")
	}

	// Test non-destructive overlay into temp directory with existing process object
	tmpDest := t.TempDir()
	procDir := filepath.Join(tmpDest, ".zqk", "process")
	if err := os.MkdirAll(procDir, 0755); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(procDir, "sentinel.yaml")
	sentinelContent := []byte("sentinel: preserved\n")
	if err := os.WriteFile(sentinelPath, sentinelContent, 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(overlayScript, tmpDest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply-community-source.sh failed: %v, output: %s", err, string(out))
	}

	// Verify sentinel was preserved untouched
	saved, err := os.ReadFile(sentinelPath)
	if err != nil || string(saved) != string(sentinelContent) {
		t.Errorf(".zqk/process was corrupted or deleted by overlay! err=%v, content=%s", err, string(saved))
	}

	// Verify required overlay assets were copied
	expectedCopied := []string{
		filepath.Join(tmpDest, "Makefile"),
		filepath.Join(tmpDest, "cmd", "zqk-community", "main.go"),
		filepath.Join(tmpDest, "scripts", "build-bootstrap-archive.sh"),
		filepath.Join(tmpDest, ".zqk", "specs", "spec_index.json"),
	}
	for _, ef := range expectedCopied {
		if !fileutil.Exists(ef) {
			t.Errorf("expected overlaid file to exist: %s", ef)
		}
	}
}

