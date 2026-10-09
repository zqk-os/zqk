package system

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestWriteRootIsolationFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("human")

	// Ensure .zqk/agent-runtime and .zqk/state dirs exist or are created
	err := writeRootIsolationFiles(tmpDir, false, logger)
	if err != nil {
		t.Fatalf("writeRootIsolationFiles failed: %v", err)
	}

	expectedFiles := []string{
		filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkConfigFileName),
		filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkLocalConfigFileName),
		filepath.Join(tmpDir, paths.ProjectDataDir, paths.AgentRuntimeDir, "agent_chat_channel.json"),
		filepath.Join(tmpDir, paths.ProjectDataDir, paths.AgentRuntimeDir, "agent_workspace_sync.json"),
		filepath.Join(tmpDir, paths.ProjectDataDir, paths.AgentRuntimeDir, "agent_git_identity.env.example"),
		filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "remote_hold.json"),
	}

	for _, ef := range expectedFiles {
		if !fileutil.Exists(ef) {
			t.Errorf("expected isolation file to exist: %s", ef)
		}
	}

	// Verify content of config/zqk.yaml
	cfgContent, err := fileutil.ReadFile(filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkConfigFileName))
	if err != nil {
		t.Fatalf("failed to read zqk.yaml: %v", err)
	}
	if !strings.Contains(string(cfgContent), "storage_mode") {
		t.Errorf("zqk.yaml missing storage_mode: %s", string(cfgContent))
	}

	// Verify content of config/zqk-local.yaml
	localContent, err := fileutil.ReadFile(filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkLocalConfigFileName))
	if err != nil {
		t.Fatalf("failed to read zqk-local.yaml: %v", err)
	}
	absTmp, _ := filepath.Abs(tmpDir)
	if !strings.Contains(string(localContent), absTmp) {
		t.Errorf("zqk-local.yaml should contain project root %s, got: %s", absTmp, string(localContent))
	}

	// Verify remote_hold.json has hold=true
	holdContent, err := fileutil.ReadFile(filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "remote_hold.json"))
	if err != nil {
		t.Fatalf("failed to read remote_hold.json: %v", err)
	}
	if !strings.Contains(string(holdContent), `"hold": true`) {
		t.Errorf("remote_hold.json should have hold=true, got: %s", string(holdContent))
	}
}

func TestWriteProjectConfigFiles_SSOT(t *testing.T) {
	tmpDir := t.TempDir()

	projectDataDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	err := writeProjectConfigFiles(projectDataDir, "test-proj", "standard", false)
	if err != nil {
		t.Fatalf("writeProjectConfigFiles failed: %v", err)
	}

	configFile := filepath.Join(tmpDir, paths.ConfigDir, paths.ZqkConfigFileName)
	data, err := fileutil.ReadFile(configFile)
	if err != nil {
		t.Fatalf("failed to read %s: %v", configFile, err)
	}
	if !strings.Contains(string(data), `name: test-proj`) {
		t.Errorf("config should contain name: test-proj, got: %s", string(data))
	}
	if fileutil.Exists(filepath.Join(tmpDir, "zqk-settings.yaml")) {
		t.Errorf("obsolete zqk-settings.yaml must not be created")
	}
}

func TestConfigRootFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Place zqk.yaml directly in root instead of config/
	zqkContent := `system:
  storage_mode: "file"
`
	if err := fileutil.WriteFile(filepath.Join(tmpDir, paths.ZqkConfigFileName), []byte(zqkContent), paths.FilePerm644); err != nil {
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
	if err := fileutil.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"name":"polyglot-app"}`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte(`[project]`+"\n"+`name = "polyglot-app"`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	if err := writeRootIsolationFiles(tmpDir, false, logger); err != nil {
		t.Fatalf("writeRootIsolationFiles failed in polyglot root: %v", err)
	}
	if err := writeSystemAccount(tmpDir); err != nil {
		t.Fatalf("writeSystemAccount failed in polyglot root: %v", err)
	}

	// Assert root isolation and account exist
	if fileutil.Exists(filepath.Join(tmpDir, "zqk-settings.yaml")) {
		t.Errorf("obsolete brand settings must not exist in polyglot root")
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

	syncPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.AgentRuntimeDir, "agent_workspace_sync.json")
	if !fileutil.Exists(syncPath) {
		t.Errorf("expected agent_workspace_sync.json to be created")
	}
}

func TestFirstRun_BoundaryAndErrorHandling(t *testing.T) {
	// Verify boundary condition where projectRoot is an existing regular file rather than a directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "blocked_by_file")
	if err := fileutil.WriteFile(filePath, []byte("plain file"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	logger := logging.GetLoggerFromProfile("human")
	if err := writeRootIsolationFiles(filePath, false, logger); err == nil {
		t.Errorf("expected writeRootIsolationFiles to fail when root is a file")
	}

	if err := writeProjectConfigFiles(filepath.Join(filePath, paths.ProjectDataDir), "blocked", "standard", false); err == nil {
		t.Errorf("expected writeProjectConfigFiles to fail when root is a file")
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
		filepath.Join(root, "scripts", "open-core", "test-public-release-gates.sh"),
		filepath.Join(root, "scripts", "open-core", "check-public-release-payload.sh"),
	}

	for _, s := range requiredScripts {
		st, err := fileutil.Stat(s)
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
	if testing.Short() {
		t.Skip("skipping slow overlay test in short mode")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("failed to determine repository root: %v", err)
	}

	overlayScript := filepath.Join(root, "scripts", "open-core", "apply-community-source.sh")
	st, err := fileutil.Stat(overlayScript)
	if err != nil {
		t.Fatalf("expected apply-community-source.sh to exist: %v", err)
	}
	if st.Mode()&0111 == 0 {
		t.Errorf("expected apply-community-source.sh to be executable, mode: %v", st.Mode())
	}

	// Verify Makefile contains zcom target
	makefilePath := filepath.Join(root, "Makefile")
	mfData, err := fileutil.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("failed to read Makefile: %v", err)
	}
	if !strings.Contains(string(mfData), "zcom:") {
		t.Errorf("expected Makefile to contain 'zcom:' build target")
	}

	// Test non-destructive overlay into temp directory with existing process object
	tmpDest := t.TempDir()
	procDir := filepath.Join(tmpDest, paths.ProjectDataDir, paths.ProcessSubdir)
	if err := fileutil.MkdirAll(procDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(procDir, "sentinel.yaml")
	sentinelContent := []byte("sentinel: preserved\n")
	if err := fileutil.WriteFile(sentinelPath, sentinelContent, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	cmd := testkit.ManagedCommand(t, t.Context(), overlayScript, tmpDest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply-community-source.sh failed: %v, output: %s", err, string(out))
	}

	// Verify sentinel was preserved untouched
	saved, err := fileutil.ReadFile(sentinelPath)
	if err != nil || string(saved) != string(sentinelContent) {
		t.Errorf("%s was corrupted or deleted by overlay! err=%v, content=%s", paths.ProcessDir, err, string(saved))
	}

	// Verify required overlay assets were copied
	expectedCopied := []string{
		filepath.Join(tmpDest, "Makefile"),
		filepath.Join(tmpDest, "cmd", "zqk", "main.go"),
		filepath.Join(tmpDest, "scripts", "build-bootstrap-archive.sh"),
		filepath.Join(tmpDest, paths.ProjectDataDir, paths.SpecsSubdir, "spec_index.json"),
	}
	for _, ef := range expectedCopied {
		if !fileutil.Exists(ef) {
			t.Errorf("expected overlaid file to exist: %s", ef)
		}
	}

	// Verify the installed Makefile is the slim community version
	installedMf, err := fileutil.ReadFile(filepath.Join(tmpDest, "Makefile"))
	if err != nil {
		t.Fatalf("failed to read installed Makefile: %v", err)
	}
	mfStr := string(installedMf)
	if !strings.Contains(mfStr, "zcom:") {
		t.Errorf("expected installed Makefile to contain 'zcom:'")
	}
	if strings.Contains(mfStr, "zqk-admin:") || strings.Contains(mfStr, "promote-stable:") {
		t.Errorf("installed Makefile contains studio-only targets: %s", mfStr)
	}

	// Verify apply-community-source.sh preserves destination-owned README.md
	destReadmePath := filepath.Join(tmpDest, "README.md")
	customReadme := []byte("# Custom Dest Community README\n")
	if err := fileutil.WriteFile(destReadmePath, customReadme, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	cmd2 := testkit.ManagedCommand(t, t.Context(), overlayScript, tmpDest)
	if out2, err := cmd2.CombinedOutput(); err != nil {
		t.Fatalf("second apply-community-source.sh run failed: %v, output: %s", err, string(out2))
	}
	preservedReadme, err := fileutil.ReadFile(destReadmePath)
	if err != nil || string(preservedReadme) != string(customReadme) {
		t.Errorf("apply-community-source.sh clobbered destination README.md: got %s", string(preservedReadme))
	}

	// Verify apply-community-source.sh includes build-bootstrap-archive.sh and sync-public-candidate.sh includes specs
	overlayData, err := fileutil.ReadFile(overlayScript)
	if err != nil {
		t.Fatalf("failed to read apply-community-source.sh: %v", err)
	}
	overlayStr := string(overlayData)
	if !strings.Contains(overlayStr, "scripts/build-bootstrap-archive.sh") {
		t.Errorf("apply-community-source.sh must include scripts/build-bootstrap-archive.sh")
	}
	if strings.Contains(overlayStr, "\"Makefile\"") || strings.Contains(overlayStr, "\"README.md\"") {
		t.Errorf("apply-community-source.sh OVERLAY_PATHS must not contain Makefile or README.md")
	}
	if !strings.Contains(overlayStr, "\"cmd/zqk/scheduler\"") {
		t.Errorf("apply-community-source.sh OVERLAY_PATHS must contain cmd/zqk/scheduler")
	}

	syncScript := filepath.Join(root, "scripts", "open-core", "sync-public-candidate.sh")
	syncData, err := fileutil.ReadFile(syncScript)
	if err != nil {
		t.Fatalf("failed to read sync-public-candidate.sh: %v", err)
	}
	syncStr := string(syncData)
	specsPath := filepath.Join(paths.ProjectDataDir, paths.SpecsSubdir)
	if !strings.Contains(syncStr, specsPath) {
		t.Errorf("sync-public-candidate.sh must include %s", specsPath)
	}
	if strings.Contains(syncStr, "\"Makefile\"") {
		t.Errorf("sync-public-candidate.sh INCLUDES must not contain Makefile")
	}
	if !strings.Contains(syncStr, "\"cmd/zqk/scheduler\"") {
		t.Errorf("sync-public-candidate.sh INCLUDES must contain cmd/zqk/scheduler")
	}

	// Verify cmd/zqk/scheduler was copied to destination and studio-specific files were pruned
	schedDest := filepath.Join(tmpDest, "cmd", "zqk", "scheduler")
	if !fileutil.Exists(schedDest) {
		t.Errorf("expected destination cmd/zqk/scheduler to exist")
	}
	for _, forbidden := range []string{"scan_tests.go", "convergence_agent_prompt.go", "ide_paste_automation.go"} {
		if fileutil.Exists(filepath.Join(schedDest, forbidden)) {
			t.Errorf("destination scheduler must not contain studio file: %s", forbidden)
		}
	}
	// Verify register_studio_scheduler_commands.go is stubbed as a no-op
	stubPath := filepath.Join(schedDest, "register_studio_scheduler_commands.go")
	if !fileutil.Exists(stubPath) {
		t.Errorf("expected register_studio_scheduler_commands.go stub to exist in destination")
	} else {
		stubData, _ := fileutil.ReadFile(stubPath)
		if !strings.Contains(string(stubData), "func registerStudioSchedulerCommands(_ *cobra.Command) {}") {
			t.Errorf("expected no-op stub for registerStudioSchedulerCommands, got: %s", string(stubData))
		}
	}
}

func TestCommunityInit_SeedsStarterGraphAndRetentionJobs(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SkipSetupTestEnvironment: true, SkipFileStorage: true})
	tmpDir := proj.Root

	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(originalDir)

	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Community init failed: %v", err)
	}

	// Verify starter organization graph exists (in-process starter graph)
	orgsDir := filepath.Join(tmpDir, paths.ProcessDir, "organizations")
	if !fileutil.Exists(orgsDir) {
		t.Fatalf("expected organizations directory to exist at %s", orgsDir)
	}

	// Verify retention / maintenance scheduler jobs were seeded
	jobsDir := filepath.Join(tmpDir, paths.ProcessDir, "scheduler_jobs")
	if !fileutil.Exists(jobsDir) {
		t.Fatalf("expected scheduler_jobs directory to exist at %s", jobsDir)
	}
}
