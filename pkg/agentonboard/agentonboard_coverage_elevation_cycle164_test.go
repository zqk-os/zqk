package agentonboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEdgeSignals_EnvironmentAndMarkers(t *testing.T) {
	root := t.TempDir()

	t.Setenv("NVIDIA_VISIBLE_DEVICES", "GPU-00000000-0000-0000-0000-000000000000")
	t.Setenv("SSH_CONNECTION", "192.168.1.100 54321 192.168.1.1 22")
	t.Setenv("SSH_CLIENT", "192.168.1.100 54321 22")

	markerFile := paths.AgentRuntimeFile(root, "headless_edge.json")
	if err := fileutil.EnsureDir(filepath.Dir(markerFile)); err != nil {
		t.Fatalf("ensure runtime dir failed: %v", err)
	}
	if err := fileutil.WriteSecureFile(markerFile, []byte(`{"mode":"headless"}`)); err != nil {
		t.Fatalf("write headless marker failed: %v", err)
	}

	signals := CollectEdgeSignals(root)
	foundNvidia := false
	foundSSH := false
	foundMarker := false
	for _, sig := range signals {
		switch sig.ID {
		case "nvidia_visible_devices":
			if sig.Present {
				foundNvidia = true
			}
		case "ssh_session":
			if sig.Present {
				foundSSH = true
			}
		case "headless_edge_marker":
			if sig.Present {
				foundMarker = true
			}
		}
	}

	if !foundNvidia {
		t.Errorf("expected nvidia_visible_devices signal")
	}
	if !foundSSH {
		t.Errorf("expected ssh_session signal")
	}
	if !foundMarker {
		t.Errorf("expected headless_edge_marker signal")
	}

	if !AnyStrongEdgeSignal(signals) {
		t.Errorf("expected strong edge signals to evaluate to true")
	}

	sshOnlySignals := []EdgeSignal{
		{ID: "ssh_session", Present: true, Detail: "SSH session"},
		{ID: "other_signal", Present: false, Detail: "none"},
	}
	if AnyStrongEdgeSignal(sshOnlySignals) {
		t.Errorf("expected ssh_session only to not be strong edge signal")
	}

	emptySignals := []EdgeSignal{}
	if AnyStrongEdgeSignal(emptySignals) {
		t.Errorf("expected empty signals to not be strong edge signal")
	}
}

func TestPacks_WriteVendorPacks_EdgeCases(t *testing.T) {
	root := t.TempDir()

	customVendor := Vendor{
		ID:          VendorID("custom_ai_copilot"),
		DisplayName: "Custom AI Copilot",
	}
	vendors := []Vendor{customVendor}

	// 1. Initial write
	written, err := WriteVendorPacks(root, vendors, false, false)
	if err != nil {
		t.Fatalf("WriteVendorPacks failed: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("expected 1 pack written, got %d", len(written))
	}

	// 2. Skipping existing without force
	skipped, err := WriteVendorPacks(root, vendors, false, false)
	if err != nil {
		t.Fatalf("WriteVendorPacks skipped failed: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("expected 0 packs written when existing and force=false, got %d", len(skipped))
	}

	// 3. Overwriting existing with force
	forced, err := WriteVendorPacks(root, vendors, false, true)
	if err != nil {
		t.Fatalf("WriteVendorPacks force failed: %v", err)
	}
	if len(forced) != 1 {
		t.Errorf("expected 1 pack written with force=true, got %d", len(forced))
	}

	// 4. Directory creation failure (file blocking directory)
	failRoot := t.TempDir()
	blockingFilePath := filepath.Join(failRoot, filepath.FromSlash(AgentPacksRelDir), string(customVendor.ID))
	if err := fileutil.EnsureDir(filepath.Dir(blockingFilePath)); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	if err := fileutil.WriteSecureFile(blockingFilePath, []byte("blocking-file")); err != nil {
		t.Fatalf("write blocking file failed: %v", err)
	}
	_, err = WriteVendorPacks(failRoot, vendors, false, true)
	if err == nil {
		t.Errorf("expected error when directory creation is blocked by a file")
	}

	// 5. File write failure (directory blocking BOOT.md file)
	failRoot2 := t.TempDir()
	blockingDir := filepath.Join(failRoot2, filepath.FromSlash(AgentPacksRelDir), string(customVendor.ID), "BOOT.md")
	if err := fileutil.EnsureDir(blockingDir); err != nil {
		t.Fatalf("ensure blocking dir failed: %v", err)
	}
	_, err = WriteVendorPacks(failRoot2, vendors, false, true)
	if err == nil {
		t.Errorf("expected error when file write is blocked by a directory")
	}
}

func TestPrime_WriteVendorConfigs_AgentHooksAndFailures(t *testing.T) {
	root := t.TempDir()

	// Create .agent directory so both .agents/hooks.json and .agent/hooks.json are targeted
	if err := fileutil.EnsureDir(filepath.Join(root, ".agent")); err != nil {
		t.Fatalf("ensure .agent dir failed: %v", err)
	}

	vendors := []Vendor{{
		ID:             VendorAgentsMD,
		DisplayName:    "AGENTS.md",
		ConfigRelPaths: []string{".agents/AGENTS.md"},
	}}

	// 1. Initial write
	wr, err := WriteVendorConfigs(root, vendors, false, false)
	if err != nil {
		t.Fatalf("WriteVendorConfigs failed: %v", err)
	}
	if len(wr.Written) < 3 {
		t.Fatalf("expected at least 3 files written (.agents/AGENTS.md, .agents/hooks.json, .agent/hooks.json), got: %v", wr.Written)
	}

	// 2. Skipping existing files without force
	wr2, err := WriteVendorConfigs(root, vendors, false, false)
	if err != nil {
		t.Fatalf("WriteVendorConfigs second pass failed: %v", err)
	}
	if len(wr2.Written) != 0 || len(wr2.Skipped) < 3 {
		t.Errorf("expected files skipped without force, got written=%v skipped=%v", wr2.Written, wr2.Skipped)
	}

	// 3. Dry-run pass
	wrDry, err := WriteVendorConfigs(root, vendors, true, true)
	if err != nil {
		t.Fatalf("WriteVendorConfigs dry run failed: %v", err)
	}
	if len(wrDry.Written) < 3 {
		t.Errorf("expected dry run to report files as written, got: %v", wrDry.Written)
	}

	// 4. Directory creation failure for vendor config
	failRoot := t.TempDir()
	blockingAgentFile := filepath.Join(failRoot, ".agents")
	if err := fileutil.WriteSecureFile(blockingAgentFile, []byte("file-blocking-dir")); err != nil {
		t.Fatalf("write blocking file failed: %v", err)
	}
	_, err = WriteVendorConfigs(failRoot, vendors, false, true)
	if err == nil {
		t.Errorf("expected error when vendor config dir cannot be created")
	}

	// 5. File write failure for vendor config
	failRoot2 := t.TempDir()
	blockingConfigFile := filepath.Join(failRoot2, ".agents", "AGENTS.md")
	if err := fileutil.EnsureDir(blockingConfigFile); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	_, err = WriteVendorConfigs(failRoot2, vendors, false, true)
	if err == nil {
		t.Errorf("expected error when vendor config file write fails")
	}
}

func TestPrime_SyncReport_ReadWriteScenarios(t *testing.T) {
	root := t.TempDir()

	// 1. Read non-existent report
	report, err := ReadSyncReport(root)
	if err != nil {
		t.Fatalf("ReadSyncReport for missing file failed: %v", err)
	}
	if report != nil {
		t.Errorf("expected nil report for missing file, got: %v", report)
	}

	// 2. Write with default schema and default updated_at
	relPath, err := WriteSyncReport(root, SyncReport{
		Vector: "A",
	}, false)
	if err != nil {
		t.Fatalf("WriteSyncReport failed: %v", err)
	}
	if relPath != SyncReportRelPath {
		t.Errorf("expected %s, got %s", SyncReportRelPath, relPath)
	}

	// 3. Read back valid report
	readBack, err := ReadSyncReport(root)
	if err != nil {
		t.Fatalf("ReadSyncReport failed: %v", err)
	}
	if readBack == nil || readBack.Schema != SyncReportSchema || readBack.Vector != "A" {
		t.Fatalf("readBack report invalid: %+v", readBack)
	}

	// 4. Dry-run write
	dryRel, err := WriteSyncReport(root, SyncReport{Vector: "B"}, true)
	if err != nil {
		t.Fatalf("WriteSyncReport dry run failed: %v", err)
	}
	if dryRel != SyncReportRelPath {
		t.Errorf("expected dryRel %s, got %s", SyncReportRelPath, dryRel)
	}

	// 5. Corrupt file read error
	reportAbs := filepath.Join(root, filepath.FromSlash(SyncReportRelPath))
	if err := fileutil.WriteSecureFile(reportAbs, []byte("invalid-json{")); err != nil {
		t.Fatalf("write invalid json failed: %v", err)
	}
	_, err = ReadSyncReport(root)
	if err == nil {
		t.Errorf("expected parse error for corrupt sync report JSON")
	}

	// 6. Write error: directory creation blocked by file
	failRoot := t.TempDir()
	blockingDirFile := filepath.Join(failRoot, ".zqk")
	if err := fileutil.WriteSecureFile(blockingDirFile, []byte("blocking")); err != nil {
		t.Fatalf("write blocking file failed: %v", err)
	}
	_, err = WriteSyncReport(failRoot, SyncReport{Vector: "B"}, false)
	if err == nil {
		t.Errorf("expected error when sync report directory creation fails")
	}

	// 7. Write error: file writing blocked by directory
	failRoot2 := t.TempDir()
	blockingReportDir := filepath.Join(failRoot2, filepath.FromSlash(SyncReportRelPath))
	if err := fileutil.EnsureDir(blockingReportDir); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	_, err = WriteSyncReport(failRoot2, SyncReport{Vector: "B"}, false)
	if err == nil {
		t.Errorf("expected error when sync report file write fails")
	}
}

func TestRun_SeatingAndLogger(t *testing.T) {
	// 1. Missing root path
	_, err := Run(Options{ProjectRoot: "/non/existent/path/for/zqk/testing"})
	if err == nil {
		t.Errorf("expected error for non-existent root path")
	}

	// 2. Successful seating function and logger output
	root := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")

	res, err := Run(Options{
		ProjectRoot: root,
		SessionOK:   true,
		Logger:      logger,
		Seat: func(string, logging.Logger) (int, error) {
			return 3, nil
		},
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.Status != ResultSuccess {
		t.Fatalf("expected ResultSuccess, got: %s", res.Status)
	}
	if seatCreated, ok := res.Stages[StageSeat].Detail["created"].(int); !ok || seatCreated != 3 {
		t.Errorf("expected 3 seats created, got: %v", res.Stages[StageSeat].Detail)
	}
}

func TestRun_StageFailures(t *testing.T) {
	// 1. WriteVendorConfigs failure during Run
	failRootConfigs := t.TempDir()
	if err := fileutil.WriteSecureFile(filepath.Join(failRootConfigs, ".agents"), []byte("blocking-file")); err != nil {
		t.Fatalf("write blocking file failed: %v", err)
	}
	resConfigs, err := Run(Options{
		ProjectRoot: failRootConfigs,
		SessionOK:   true,
		SkipSeat:    true,
	})
	if err != nil {
		t.Fatalf("Run unexpectedly errored: %v", err)
	}
	if resConfigs.Status != ResultBlocked || resConfigs.Stages[StagePrimeWorkspace].Status != StageFailed {
		t.Errorf("expected PrimeWorkspace failure when vendor configs cannot be written")
	}

	// 2. WriteVendorPacks failure during Run
	failRootPacks := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(failRootPacks, ".agents")); err != nil {
		t.Fatalf("ensure .agents failed: %v", err)
	}
	packsBlocking := filepath.Join(failRootPacks, filepath.FromSlash(AgentPacksRelDir))
	if err := fileutil.EnsureDir(filepath.Dir(packsBlocking)); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	if err := fileutil.WriteSecureFile(packsBlocking, []byte("blocking-file")); err != nil {
		t.Fatalf("write blocking file failed: %v", err)
	}
	resPacks, err := Run(Options{
		ProjectRoot: failRootPacks,
		SessionOK:   true,
		SkipSeat:    true,
	})
	if err != nil {
		t.Fatalf("Run unexpectedly errored: %v", err)
	}
	if resPacks.Status != ResultBlocked || resPacks.Stages[StagePrimeWorkspace].Status != StageFailed {
		t.Errorf("expected PrimeWorkspace failure when vendor packs cannot be written")
	}

	// 3. WriteSyncReport failure during Run
	failRootReport := t.TempDir()
	reportBlocking := filepath.Join(failRootReport, ".zqk", "agent-runtime")
	if err := fileutil.EnsureDir(filepath.Dir(reportBlocking)); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	if err := fileutil.WriteSecureFile(reportBlocking, []byte("blocking-file")); err != nil {
		t.Fatalf("write blocking file failed: %v", err)
	}
	resReport, err := Run(Options{
		ProjectRoot: failRootReport,
		SessionOK:   true,
		SkipSeat:    true,
	})
	if err != nil {
		t.Fatalf("Run unexpectedly errored: %v", err)
	}
	if resReport.Status != ResultBlocked || resReport.Stages[StagePrimeKernel].Status != StageFailed {
		t.Errorf("expected PrimeKernel failure when sync report cannot be written")
	}

	// 4. Vendor filter smoke test
	failRootSmoke := t.TempDir()
	resSmoke, err := Run(Options{
		ProjectRoot:  failRootSmoke,
		SessionOK:    true,
		SkipSeat:     true,
		VendorFilter: map[VendorID]struct{}{VendorID("non_existent"): {}},
	})
	if err != nil {
		t.Fatalf("Run unexpectedly errored: %v", err)
	}
	if resSmoke == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestSkills_DestinationsAndInvalidDir(t *testing.T) {
	root := t.TempDir()

	// 1. DefaultSkillDestinations with detected vendors without directories on disk
	detected := []DetectedVendor{
		{ID: VendorClaudeCode, DisplayName: "Claude Code"},
		{ID: VendorAgent, DisplayName: "Agent"},
		{ID: VendorIDE, DisplayName: "IDE"},
	}
	dests := DefaultSkillDestinations(root, detected, false, false)
	if len(dests) < 3 {
		t.Errorf("expected at least 3 destinations based on detected vendors, got: %v", dests)
	}

	// 2. DefaultSkillDestinations in headless mode with allVendors false
	headlessDests := DefaultSkillDestinations(root, detected, false, true)
	if len(headlessDests) != 0 {
		t.Errorf("expected 0 destinations for headless without allVendors, got: %v", headlessDests)
	}

	// 3. SyncSkills when kernel skills directory is blocked by a file
	kernelSkillsDir := filepath.Join(root, paths.ProjectDataDir, paths.SkillsSubdir)
	if err := fileutil.EnsureDir(filepath.Dir(kernelSkillsDir)); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	if err := fileutil.WriteSecureFile(kernelSkillsDir, []byte("blocking-file")); err != nil {
		t.Fatalf("write blocking file failed: %v", err)
	}
	_, err := SyncSkills(root, detected, true, false, false, false)
	if err == nil {
		t.Errorf("expected error when kernel skills dir is a file")
	}
}

func TestSkills_IgnoredEntriesAndOverwriting(t *testing.T) {
	root2 := t.TempDir()
	validKernelDir := filepath.Join(root2, paths.ProjectDataDir, paths.SkillsSubdir)
	if err := fileutil.EnsureDir(validKernelDir); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	// Non-directory file in kernel skills dir
	if err := fileutil.WriteSecureFile(filepath.Join(validKernelDir, "README.md"), []byte("readme")); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	// Dot-prefixed directory in kernel skills dir
	if err := fileutil.EnsureDir(filepath.Join(validKernelDir, ".hidden_skill")); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	// Directory without SKILL.md
	if err := fileutil.EnsureDir(filepath.Join(validKernelDir, "no_skill_md")); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	// Valid skill directory
	validSkill := filepath.Join(validKernelDir, "valid-skill")
	if err := fileutil.EnsureDir(validSkill); err != nil {
		t.Fatalf("ensure dir failed: %v", err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(validSkill, "SKILL.md"), []byte("# Valid Skill")); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}

	// Existing destination directory containing ignored dot-files and non-kernel symlink
	agentSkillsDir := filepath.Join(root2, ".agent", "skills")
	if err := fileutil.EnsureDir(agentSkillsDir); err != nil {
		t.Fatalf("ensure agent skills dir failed: %v", err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(agentSkillsDir, ".gitignore"), []byte("*.tmp")); err != nil {
		t.Fatalf("write .gitignore failed: %v", err)
	}
	otherTarget := filepath.Join(root2, "other_dir")
	if err := fileutil.EnsureDir(otherTarget); err != nil {
		t.Fatalf("ensure other dir failed: %v", err)
	}
	if err := os.Symlink(otherTarget, filepath.Join(agentSkillsDir, "foreign_link")); err != nil {
		t.Fatalf("create foreign symlink failed: %v", err)
	}

	// Sync with valid skill and destination
	syncRes, err := SyncSkills(root2, []DetectedVendor{{ID: VendorAgent}}, false, false, false, false)
	if err != nil {
		t.Fatalf("SyncSkills failed: %v", err)
	}
	if len(syncRes.Linked) != 1 || syncRes.Linked[0] != ".agent/skills/valid-skill" {
		t.Errorf("expected valid-skill to be linked, got: %+v", syncRes)
	}

	// Foreign link must not be pruned without force
	if _, err := os.Lstat(filepath.Join(agentSkillsDir, "foreign_link")); err != nil {
		t.Errorf("foreign symlink should not be pruned without force: %v", err)
	}

	// Broken or wrong-target symlink updating
	brokenLink := filepath.Join(agentSkillsDir, "valid-skill")
	if err := fileutil.RemoveAll(brokenLink); err != nil {
		t.Fatalf("remove link failed: %v", err)
	}
	if err := os.Symlink("/invalid/target/path", brokenLink); err != nil {
		t.Fatalf("create broken symlink failed: %v", err)
	}
	syncRes2, err := SyncSkills(root2, []DetectedVendor{{ID: VendorAgent}}, false, false, false, false)
	if err != nil {
		t.Fatalf("SyncSkills updating broken symlink failed: %v", err)
	}
	if len(syncRes2.Linked) != 1 {
		t.Errorf("expected broken symlink to be re-linked, got: %+v", syncRes2)
	}

	// Non-symlink existing file collision with force=true
	collidingFile := filepath.Join(agentSkillsDir, "valid-skill")
	if err := fileutil.RemoveAll(collidingFile); err != nil {
		t.Fatalf("remove link failed: %v", err)
	}
	if err := fileutil.WriteSecureFile(collidingFile, []byte("colliding-file")); err != nil {
		t.Fatalf("write colliding file failed: %v", err)
	}
	syncRes3, err := SyncSkills(root2, []DetectedVendor{{ID: VendorAgent}}, false, false, false, true)
	if err != nil {
		t.Fatalf("SyncSkills with force on colliding file failed: %v", err)
	}
	if len(syncRes3.Linked) != 1 {
		t.Errorf("expected colliding file to be overwritten with symlink when force=true, got: %+v", syncRes3)
	}
}

func TestVendors_OllamaHostAndFilterEdgeCases(t *testing.T) {
	root := t.TempDir()

	// 1. Ollama detected via OLLAMA_HOST
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:11434")
	detected := DetectVendors(root)
	foundOllama := false
	for _, d := range detected {
		if d.ID == VendorOllama {
			foundOllama = true
			if len(d.Markers) == 0 || d.Markers[0] != "local:11434" {
				t.Errorf("expected marker local:11434, got: %v", d.Markers)
			}
		}
	}
	if !foundOllama {
		t.Errorf("expected Ollama detection via OLLAMA_HOST")
	}

	// 2. Ollama detected via LLM_PROVIDER=ollama
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("LLM_PROVIDER", "ollama")
	detected2 := DetectVendors(root)
	foundOllama2 := false
	for _, d := range detected2 {
		if d.ID == VendorOllama {
			foundOllama2 = true
		}
	}
	if !foundOllama2 {
		t.Errorf("expected Ollama detection via LLM_PROVIDER=ollama")
	}

	// 3. VendorsToPrime with duplicate and unknown vendors
	dupDetected := []DetectedVendor{
		{ID: VendorIDE, DisplayName: "IDE"},
		{ID: VendorIDE, DisplayName: "IDE"},
		{ID: VendorID("non_existent_vendor"), DisplayName: "Unknown"},
	}
	primeList := VendorsToPrime(dupDetected, false)
	ideCount := 0
	for _, v := range primeList {
		if v.ID == VendorIDE {
			ideCount++
		}
	}
	if ideCount != 1 {
		t.Errorf("expected exactly 1 VendorIDE in prime list, got %d", ideCount)
	}

	// 4. NormalizeVendorFilter with comma and whitespace variations
	filter := NormalizeVendorFilter(" , ide , , claude_code , ")
	if filter == nil || len(filter) != 2 {
		t.Fatalf("expected 2 filter entries, got: %v", filter)
	}
	if _, ok := filter[VendorIDE]; !ok {
		t.Errorf("expected VendorIDE in filter")
	}
	if _, ok := filter[VendorClaudeCode]; !ok {
		t.Errorf("expected VendorClaudeCode in filter")
	}
	if emptyFilter := NormalizeVendorFilter(",,,   ,,,"); emptyFilter != nil {
		t.Errorf("expected nil filter for only commas and spaces, got: %v", emptyFilter)
	}
}
