package agentonboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSyncSkills_ProjectsAndPrunes(t *testing.T) {
	root := t.TempDir()

	// 1. Setup kernel skills in .zqk/skills
	kernelSkillsDir := filepath.Join(root, paths.ProjectDataDir, paths.SkillsSubdir)
	skill1Dir := filepath.Join(kernelSkillsDir, "arch-design")
	skill2Dir := filepath.Join(kernelSkillsDir, "code-craftsman")
	if err := fileutil.EnsureDir(skill1Dir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(skill2Dir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(skill1Dir, "SKILL.md"), []byte("# Arch Design")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(skill2Dir, "SKILL.md"), []byte("# Code Craftsman")); err != nil {
		t.Fatal(err)
	}

	// 2. Setup .agent marker and an unmanaged snowflake in .agent/skills
	agentDir := filepath.Join(root, ".agent")
	agentSkillsDir := filepath.Join(agentDir, "skills")
	snowflakeDir := filepath.Join(agentSkillsDir, "rogue-snowflake")
	if err := fileutil.EnsureDir(snowflakeDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(snowflakeDir, "SKILL.md"), []byte("# Rogue")); err != nil {
		t.Fatal(err)
	}

	detected := []DetectedVendor{
		{ID: VendorAgent, DisplayName: "Agent"},
	}

	// First pass: Without force, user directory should NOT be pruned
	safeRes, err := SyncSkills(root, detected, false, false, false, false)
	if err != nil {
		t.Fatalf("SyncSkills safe execution failed: %v", err)
	}
	if len(safeRes.Pruned) != 0 {
		t.Errorf("expected 0 pruned without force, got %v", safeRes.Pruned)
	}
	if _, err := fileutil.Stat(snowflakeDir); err != nil {
		t.Errorf("expected user snowflake dir to be preserved when force is false")
	}

	// Dry run with force should detect link and prune targets without modifying disk
	dryRes, err := SyncSkills(root, detected, false, false, true, true)
	if err != nil {
		t.Fatalf("SyncSkills dry-run failed: %v", err)
	}
	if len(dryRes.Pruned) != 1 || dryRes.Pruned[0] != ".agent/skills/rogue-snowflake" {
		t.Errorf("expected dry-run pruned [.agent/skills/rogue-snowflake], got %v", dryRes.Pruned)
	}
	if _, err := fileutil.Stat(snowflakeDir); err != nil {
		t.Errorf("expected snowflake to still exist during dry-run")
	}

	// Second pass: Real execution with force
	execRes, err := SyncSkills(root, detected, false, false, false, true)
	if err != nil {
		t.Fatalf("SyncSkills execution failed: %v", err)
	}
	if len(execRes.Pruned) != 1 || execRes.Pruned[0] != ".agent/skills/rogue-snowflake" {
		t.Errorf("expected pruned [.agent/skills/rogue-snowflake], got %v", execRes.Pruned)
	}

	// Verify snowflake was purged when force was true
	if _, err := fileutil.Stat(snowflakeDir); !fileutil.IsNotExist(err) {
		t.Errorf("expected snowflake dir to be deleted with force, but stat returned: %v", err)
	}

	// Verify symlinks were created and point to correct relative target
	link1 := filepath.Join(agentSkillsDir, "arch-design")
	fi1, err := os.Lstat(link1)
	if err != nil {
		t.Fatalf("stat link1 failed: %v", err)
	}
	if fi1.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected link1 to be symlink")
	}
	target1, err := os.Readlink(link1)
	if err != nil {
		t.Fatalf("readlink link1 failed: %v", err)
	}
	expectedTarget1 := "../../" + filepath.ToSlash(filepath.Join(paths.ProjectDataDir, paths.SkillsSubdir, "arch-design"))
	if filepath.Clean(target1) != filepath.Clean(expectedTarget1) {
		t.Errorf("expected target %q, got %q", expectedTarget1, target1)
	}

	// Third pass: Idempotency check — second real execution should skip already linked skills
	idemRes, err := SyncSkills(root, detected, false, false, false, false)
	if err != nil {
		t.Fatalf("SyncSkills idempotent execution failed: %v", err)
	}
	if len(idemRes.Pruned) != 0 {
		t.Errorf("expected 0 pruned, got %v", idemRes.Pruned)
	}
	if len(idemRes.Linked) != 0 {
		t.Errorf("expected 0 newly linked, got %v", idemRes.Linked)
	}
	if len(idemRes.Skipped) != 2 {
		t.Errorf("expected 2 skipped, got %v", idemRes.Skipped)
	}
}

func TestSyncSkills_OrphanedSymlinkPrunedWithoutForce(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".agent", "skills")
	if err := fileutil.EnsureDir(agentDir); err != nil {
		t.Fatal(err)
	}

	// Create kernel skills dir
	kernelSkillsDir := filepath.Join(root, paths.ProjectDataDir, paths.SkillsSubdir)
	if err := fileutil.EnsureDir(filepath.Join(kernelSkillsDir, "active-skill")); err != nil {
		t.Fatal(err)
	}
	_ = fileutil.WriteSecureFile(filepath.Join(kernelSkillsDir, "active-skill", "SKILL.md"), []byte("# Active"))

	// Create an orphaned symlink pointing to an old kernel skill that no longer exists
	orphanLink := filepath.Join(agentDir, "old-deleted-skill")
	orphanTarget := "../../" + filepath.ToSlash(filepath.Join(paths.ProjectDataDir, paths.SkillsSubdir, "old-deleted-skill"))
	if err := os.Symlink(orphanTarget, orphanLink); err != nil {
		t.Fatal(err)
	}

	detected := []DetectedVendor{{ID: VendorAgent, DisplayName: "Agent"}}
	res, err := SyncSkills(root, detected, false, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Pruned) != 1 || res.Pruned[0] != ".agent/skills/old-deleted-skill" {
		t.Fatalf("expected orphaned kernel symlink to be pruned even without force, got: %v", res.Pruned)
	}
	if _, err := os.Lstat(orphanLink); !os.IsNotExist(err) {
		t.Fatalf("expected orphan symlink to be removed from disk")
	}
}

func TestSyncSkills_CollisionWithoutForceErrors(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".agent", "skills")
	collidingDir := filepath.Join(agentDir, "arch-design")
	if err := fileutil.EnsureDir(collidingDir); err != nil {
		t.Fatal(err)
	}
	_ = fileutil.WriteSecureFile(filepath.Join(collidingDir, "user.txt"), []byte("user content"))

	// Create kernel skill with same name
	kernelSkillsDir := filepath.Join(root, paths.ProjectDataDir, paths.SkillsSubdir)
	if err := fileutil.EnsureDir(filepath.Join(kernelSkillsDir, "arch-design")); err != nil {
		t.Fatal(err)
	}
	_ = fileutil.WriteSecureFile(filepath.Join(kernelSkillsDir, "arch-design", "SKILL.md"), []byte("# Arch"))

	detected := []DetectedVendor{{ID: VendorAgent, DisplayName: "Agent"}}
	_, err := SyncSkills(root, detected, false, false, false, false)
	if err == nil {
		t.Fatal("expected error on collision with non-symlink directory when force is false")
	}

	// Verify user file was not destroyed
	if _, err := os.Stat(filepath.Join(collidingDir, "user.txt")); err != nil {
		t.Fatalf("user content was destroyed on collision: %v", err)
	}
}

func TestSyncSkills_HeadlessSkips(t *testing.T) {
	root := t.TempDir()
	res, err := SyncSkills(root, nil, false, true, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Dests) != 0 {
		t.Errorf("expected 0 dests in headless, got %v", res.Dests)
	}
}

func TestSyncSkills_NoKernelSkillsDir(t *testing.T) {
	root := t.TempDir()
	res, err := SyncSkills(root, nil, true, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Linked) != 0 || len(res.Pruned) != 0 {
		t.Errorf("expected empty result when no kernel skills exist, got %v", res)
	}
}
