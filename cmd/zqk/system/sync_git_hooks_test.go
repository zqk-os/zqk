package system

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewSyncGitHooksCmd(t *testing.T) {
	t.Parallel()
	cmd := NewSyncGitHooksCmd()
	if cmd == nil {
		t.Fatal("NewSyncGitHooksCmd() returned nil")
	}

	if cmd.Use != "sync-git-hooks" {
		t.Errorf("Expected command use to be 'sync-git-hooks', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Command should have a short description")
	}
}

func TestSyncGitHooksCommandFlags(t *testing.T) {
	t.Parallel()
	cmd := NewSyncGitHooksCmd()

	// Test flags
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("Command should have --dry-run flag")
	}
	if cmd.Flags().Lookup("source") == nil {
		t.Error("Command should have --source flag")
	}
	if cmd.Flags().Lookup("target") == nil {
		t.Error("Command should have --target flag")
	}
}

func TestInstallHookAndEnableRoundTripPreserveExecutableMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	template := filepath.Join(root, "template")
	hooksDir := filepath.Join(root, "hooks")
	if err := fileutil.EnsureDir(hooksDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(template, []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatal(err)
	}

	installed := filepath.Join(hooksDir, "pre-commit")
	if err := installHook(template, installed); err != nil {
		t.Fatalf("installHook: %v", err)
	}
	assertExecutableHook(t, installed)

	cmd := NewSyncGitHooksCmd()
	if err := setGitHookEnabled(cmd, hooksDir, "pre-commit", false); err != nil {
		t.Fatalf("disable hook: %v", err)
	}
	if fileutil.Exists(installed) {
		t.Fatal("active hook still exists after disable")
	}
	if !fileutil.Exists(installed + disabledHookSuffix) {
		t.Fatal("disabled hook marker not found")
	}

	if err := setGitHookEnabled(cmd, hooksDir, "pre-commit", true); err != nil {
		t.Fatalf("enable hook: %v", err)
	}
	assertExecutableHook(t, installed)

	equal, err := compareFiles(template, installed)
	if err != nil {
		t.Fatalf("compareFiles: %v", err)
	}
	if !equal {
		t.Fatal("installed hook content differs from template")
	}
}

func assertExecutableHook(t *testing.T, path string) {
	t.Helper()

	info, err := fileutil.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&ownerExecuteBit == 0 {
		t.Fatalf("hook mode %o is not executable", info.Mode().Perm())
	}
}

func TestEnsureGitHooks_InstallsPreCommitAndPrePush(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := fileutil.EnsureDir(gitDir); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitHooks(root, nil); err != nil {
		t.Fatalf("EnsureGitHooks failed: %v", err)
	}

	preCommit := filepath.Join(gitDir, "hooks", "pre-commit")
	prePush := filepath.Join(gitDir, "hooks", "pre-push")

	assertExecutableHook(t, preCommit)
	assertExecutableHook(t, prePush)

	preCommitContent, err := fileutil.ReadFile(preCommit)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(preCommitContent, []byte("FAIL-CLOSED BRANCH PROTECTION VIOLATION")) {
		t.Errorf("pre-commit missing branch protection check")
	}
	if !bytes.Contains(preCommitContent, []byte("UPSTREAM SYNC VIOLATION")) {
		t.Errorf("pre-commit missing upstream base sync check")
	}
	if !bytes.Contains(preCommitContent, []byte("Failed to find or compile zqk-vet verification engine! Aborting commit.")) {
		t.Errorf("pre-commit missing fail-closed zqk-vet check")
	}
	if !bytes.Contains(preCommitContent, []byte("Gofmt Formatting Ratchet Gate")) {
		t.Errorf("pre-commit missing gofmt formatting ratchet gate")
	}
	if !bytes.Contains(preCommitContent, []byte("Formatting violation in staged file")) {
		t.Errorf("pre-commit missing formatting violation message")
	}

	prePushContent, err := fileutil.ReadFile(prePush)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(prePushContent, []byte("FAIL-CLOSED BRANCH PROTECTION VIOLATION")) {
		t.Errorf("pre-push missing branch protection check")
	}
	if !bytes.Contains(prePushContent, []byte("UPSTREAM SYNC VIOLATION")) {
		t.Errorf("pre-push missing upstream base sync check")
	}
	if !bytes.Contains(prePushContent, []byte("CODEBASE VERIFICATION GATE FAILED")) {
		t.Errorf("pre-push missing zqk-vet verification gate")
	}
}

func TestEnsureGitHooks_PreCommitSkipsZqkVetInConsumerRepo(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := fileutil.EnsureDir(gitDir); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitHooks(root, nil); err != nil {
		t.Fatalf("EnsureGitHooks failed: %v", err)
	}

	preCommit := filepath.Join(gitDir, "hooks", "pre-commit")
	content, err := fileutil.ReadFile(preCommit)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the hook guards zqk-vet with cmd/zqk-vet/main.go check
	if !bytes.Contains(content, []byte(`if [ -f "$REPO_ROOT/cmd/zqk-vet/main.go" ]; then`)) {
		t.Errorf("pre-commit missing guard: if [ -f \"$REPO_ROOT/cmd/zqk-vet/main.go\" ]; then")
	}
}

func TestSyncGitHooks_TableDrivenIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (sourceDir, targetDir string)
		args        func(sourceDir, targetDir string) []string
		wantErr     bool
		errContains string
		verify      func(t *testing.T, sourceDir, targetDir string, output string)
	}{
		{
			name: "install templates from source to target with executable permissions",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				src := filepath.Join(root, "tools", "git-hooks")
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(src); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteStandardFile(filepath.Join(src, "pre-commit"), []byte("#!/bin/sh\necho pre-commit\n")); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteStandardFile(filepath.Join(src, "pre-push"), []byte("#!/bin/sh\necho pre-push\n")); err != nil {
					t.Fatal(err)
				}
				// Non-standard hook file that should be skipped by validHooks check
				if err := fileutil.WriteStandardFile(filepath.Join(src, "unsupported-hook.txt"), []byte("echo ignore")); err != nil {
					t.Fatal(err)
				}
				return src, tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--source", sourceDir, "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				preCommit := filepath.Join(targetDir, "pre-commit")
				prePush := filepath.Join(targetDir, "pre-push")
				unsupported := filepath.Join(targetDir, "unsupported-hook.txt")

				if !fileutil.Exists(preCommit) {
					t.Errorf("expected %s to exist", preCommit)
				}
				assertExecutableHook(t, preCommit)

				if !fileutil.Exists(prePush) {
					t.Errorf("expected %s to exist", prePush)
				}
				assertExecutableHook(t, prePush)

				if fileutil.Exists(unsupported) {
					t.Errorf("expected unsupported hook %s not to be installed", unsupported)
				}

				if !strings.Contains(output, "Installed pre-commit hook from template") {
					t.Errorf("expected output to mention installed pre-commit, got: %s", output)
				}
				if !strings.Contains(output, "Installed pre-push hook from template") {
					t.Errorf("expected output to mention installed pre-push, got: %s", output)
				}
			},
		},
		{
			name: "dry run reports missing hooks without modifying disk",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				src := filepath.Join(root, "tools", "git-hooks")
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(src); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteStandardFile(filepath.Join(src, "pre-commit"), []byte("#!/bin/sh\necho dry\n")); err != nil {
					t.Fatal(err)
				}
				return src, tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--dry-run", "--source", sourceDir, "--target", targetDir}
			},
			wantErr:     true,
			errContains: "git hooks are out of sync",
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				preCommit := filepath.Join(targetDir, "pre-commit")
				if fileutil.Exists(preCommit) {
					t.Errorf("dry-run should not create hook on disk: %s", preCommit)
				}
				if !strings.Contains(output, "Hook out of sync: pre-commit (not installed)") {
					t.Errorf("expected output to report pre-commit not installed, got: %s", output)
				}
				if !strings.Contains(output, "Run 'zqk system sync-git-hooks") {
					t.Errorf("expected output to include run hint, got: %s", output)
				}
			},
		},
		{
			name: "dry run reports drifted hooks without modifying disk",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				src := filepath.Join(root, "tools", "git-hooks")
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(src); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteStandardFile(filepath.Join(src, "pre-commit"), []byte("#!/bin/sh\necho new\n")); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteExecutableFile(filepath.Join(tgt, "pre-commit"), []byte("#!/bin/sh\necho old\n")); err != nil {
					t.Fatal(err)
				}
				return src, tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--dry-run", "--source", sourceDir, "--target", targetDir}
			},
			wantErr:     true,
			errContains: "git hooks are out of sync",
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				preCommit := filepath.Join(targetDir, "pre-commit")
				content, err := fileutil.ReadFile(preCommit)
				if err != nil {
					t.Fatal(err)
				}
				if string(content) != "#!/bin/sh\necho old\n" {
					t.Errorf("dry-run must not modify hook content, got: %s", string(content))
				}
				if !strings.Contains(output, "Hook out of sync: pre-commit (template differs from installed)") {
					t.Errorf("expected template differs warning, got: %s", output)
				}
			},
		},
		{
			name: "dry run reports non-executable hook without modifying disk",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				src := filepath.Join(root, "tools", "git-hooks")
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(src); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				hookContent := []byte("#!/bin/sh\necho hello\n")
				if err := fileutil.WriteStandardFile(filepath.Join(src, "pre-commit"), hookContent); err != nil {
					t.Fatal(err)
				}
				preCommit := filepath.Join(tgt, "pre-commit")
				if err := fileutil.WriteFile(preCommit, hookContent, 0o644); err != nil {
					t.Fatal(err)
				}
				return src, tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--dry-run", "--source", sourceDir, "--target", targetDir}
			},
			wantErr:     true,
			errContains: "git hooks are out of sync",
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				if !strings.Contains(output, "Hook out of sync: pre-commit (installed but not executable; git ignores it)") {
					t.Errorf("expected non-executable warning, got: %s", output)
				}
			},
		},
		{
			name: "dry run succeeds when hooks are up to date",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				src := filepath.Join(root, "tools", "git-hooks")
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(src); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				hookContent := []byte("#!/bin/sh\necho current\n")
				if err := fileutil.WriteStandardFile(filepath.Join(src, "pre-commit"), hookContent); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteExecutableFile(filepath.Join(tgt, "pre-commit"), hookContent); err != nil {
					t.Fatal(err)
				}
				return src, tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--dry-run", "--source", sourceDir, "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				if !strings.Contains(output, "All git hooks are up to date.") {
					t.Errorf("expected all git hooks are up to date, got: %s", output)
				}
			},
		},
		{
			name: "list flag reports enabled disabled and non-executable hooks",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				// 1. Enabled executable hook
				if err := fileutil.WriteExecutableFile(filepath.Join(tgt, "pre-commit"), []byte("#!/bin/sh\n")); err != nil {
					t.Fatal(err)
				}
				// 2. Disabled hook (.disabled suffix)
				if err := fileutil.WriteStandardFile(filepath.Join(tgt, "pre-push.disabled"), []byte("#!/bin/sh\n")); err != nil {
					t.Fatal(err)
				}
				// 3. Non-executable hook (0644)
				if err := fileutil.WriteFile(filepath.Join(tgt, "commit-msg"), []byte("#!/bin/sh\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				// 4. Sample hook (.sample suffix should be skipped)
				if err := fileutil.WriteStandardFile(filepath.Join(tgt, "applypatch-msg.sample"), []byte("#!/bin/sh\n")); err != nil {
					t.Fatal(err)
				}
				return "", tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--list", "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				if !strings.Contains(output, "pre-commit             enabled") {
					t.Errorf("expected pre-commit enabled, got: %s", output)
				}
				if !strings.Contains(output, "pre-push               disabled") {
					t.Errorf("expected pre-push disabled, got: %s", output)
				}
				if !strings.Contains(output, "commit-msg             disabled (not executable; run sync-git-hooks to repair)") {
					t.Errorf("expected commit-msg disabled (not executable), got: %s", output)
				}
				if strings.Contains(output, "applypatch-msg") {
					t.Errorf("expected sample hooks to be excluded from list, got: %s", output)
				}
			},
		},
		{
			name: "list flag on empty directory",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				return "", tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--list", "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				if !strings.Contains(output, "No git hooks installed in") {
					t.Errorf("expected 'No git hooks installed in', got: %s", output)
				}
			},
		},
		{
			name: "disable flag renames active hook to disabled suffix",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteExecutableFile(filepath.Join(tgt, "pre-commit"), []byte("#!/bin/sh\n")); err != nil {
					t.Fatal(err)
				}
				return "", tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--disable", "pre-commit", "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				active := filepath.Join(targetDir, "pre-commit")
				disabled := filepath.Join(targetDir, "pre-commit.disabled")

				if fileutil.Exists(active) {
					t.Errorf("active hook %s should not exist after disable", active)
				}
				if !fileutil.Exists(disabled) {
					t.Errorf("disabled hook %s should exist after disable", disabled)
				}
				if !strings.Contains(output, "Hook pre-commit disabled") {
					t.Errorf("expected confirmation output, got: %s", output)
				}
			},
		},
		{
			name: "disable flag when hook already disabled",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteStandardFile(filepath.Join(tgt, "pre-commit.disabled"), []byte("#!/bin/sh\n")); err != nil {
					t.Fatal(err)
				}
				return "", tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--disable", "pre-commit", "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				if !strings.Contains(output, "Hook pre-commit is already disabled") {
					t.Errorf("expected already disabled message, got: %s", output)
				}
			},
		},
		{
			name: "disable flag when hook not found returns error",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				return "", tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--disable", "nonexistent-hook", "--target", targetDir}
			},
			wantErr:     true,
			errContains: "hook nonexistent-hook not found",
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
			},
		},
		{
			name: "enable flag restores disabled hook and ensures executable bit",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteFile(filepath.Join(tgt, "pre-commit.disabled"), []byte("#!/bin/sh\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return "", tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--enable", "pre-commit", "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				active := filepath.Join(targetDir, "pre-commit")
				disabled := filepath.Join(targetDir, "pre-commit.disabled")

				if fileutil.Exists(disabled) {
					t.Errorf("disabled hook %s should not exist after enable", disabled)
				}
				if !fileutil.Exists(active) {
					t.Errorf("active hook %s should exist after enable", active)
				}
				assertExecutableHook(t, active)
				if !strings.Contains(output, "Hook pre-commit enabled") {
					t.Errorf("expected confirmation output, got: %s", output)
				}
			},
		},
		{
			name: "enable flag when hook already enabled",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteExecutableFile(filepath.Join(tgt, "pre-commit"), []byte("#!/bin/sh\n")); err != nil {
					t.Fatal(err)
				}
				return "", tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--enable", "pre-commit", "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				if !strings.Contains(output, "Hook pre-commit is already enabled") {
					t.Errorf("expected already enabled message, got: %s", output)
				}
			},
		},
		{
			name: "enable flag when hook not found returns error",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				return "", tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--enable", "nonexistent-hook", "--target", targetDir}
			},
			wantErr:     true,
			errContains: "hook nonexistent-hook not found",
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
			},
		},
		{
			name: "sync updates drifted hook repairs permissions and skips disabled",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				src := filepath.Join(root, "tools", "git-hooks")
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(src); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				// 1. pre-commit: drifted content
				if err := fileutil.WriteStandardFile(filepath.Join(src, "pre-commit"), []byte("#!/bin/sh\necho updated\n")); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteExecutableFile(filepath.Join(tgt, "pre-commit"), []byte("#!/bin/sh\necho outdated\n")); err != nil {
					t.Fatal(err)
				}
				// 2. pre-push: non-executable in target
				pushContent := []byte("#!/bin/sh\necho push\n")
				if err := fileutil.WriteStandardFile(filepath.Join(src, "pre-push"), pushContent); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteFile(filepath.Join(tgt, "pre-push"), pushContent, 0o644); err != nil {
					t.Fatal(err)
				}
				// 3. commit-msg: disabled hook in target
				if err := fileutil.WriteStandardFile(filepath.Join(src, "commit-msg"), []byte("#!/bin/sh\necho msg\n")); err != nil {
					t.Fatal(err)
				}
				if err := fileutil.WriteStandardFile(filepath.Join(tgt, "commit-msg.disabled"), []byte("#!/bin/sh\necho parked\n")); err != nil {
					t.Fatal(err)
				}
				return src, tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--source", sourceDir, "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				// pre-commit updated
				content, err := fileutil.ReadFile(filepath.Join(targetDir, "pre-commit"))
				if err != nil {
					t.Fatal(err)
				}
				if string(content) != "#!/bin/sh\necho updated\n" {
					t.Errorf("expected updated content for pre-commit, got: %s", string(content))
				}
				assertExecutableHook(t, filepath.Join(targetDir, "pre-commit"))

				// pre-push permissions repaired
				assertExecutableHook(t, filepath.Join(targetDir, "pre-push"))

				// commit-msg skipped
				if fileutil.Exists(filepath.Join(targetDir, "commit-msg")) {
					t.Errorf("commit-msg should not be installed because commit-msg.disabled exists")
				}
				if !strings.Contains(output, "Skipping commit-msg hook (disabled)") {
					t.Errorf("expected output to mention skipping disabled hook, got: %s", output)
				}
				if !strings.Contains(output, "Updated pre-commit hook from template") {
					t.Errorf("expected output to mention updated pre-commit, got: %s", output)
				}
				if !strings.Contains(output, "Repaired pre-push hook permissions") {
					t.Errorf("expected output to mention repaired pre-push, got: %s", output)
				}
			},
		},
		{
			name: "gracefully handles missing source templates directory",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				tgt := filepath.Join(root, ".git", "hooks")
				if err := fileutil.EnsureDir(tgt); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(root, "non-existent-source"), tgt
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--source", sourceDir, "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				if !strings.Contains(output, "Source templates directory not found") {
					t.Errorf("expected missing source warning, got: %s", output)
				}
			},
		},
		{
			name: "gracefully handles missing target hooks directory",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				src := filepath.Join(root, "tools", "git-hooks")
				if err := fileutil.EnsureDir(src); err != nil {
					t.Fatal(err)
				}
				return src, filepath.Join(root, "non-existent-target")
			},
			args: func(sourceDir, targetDir string) []string {
				return []string{"--source", sourceDir, "--target", targetDir}
			},
			wantErr: false,
			verify: func(t *testing.T, sourceDir, targetDir string, output string) {
				if !strings.Contains(output, "Target hooks directory not found") {
					t.Errorf("expected missing target warning, got: %s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sourceDir, targetDir := tt.setup(t)
			cmd := NewSyncGitHooksCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args(sourceDir, targetDir))

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Fatalf("cmd.Execute() error = %v, wantErr %v (output: %s)", err, tt.wantErr, buf.String())
			}
			if tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			}
			if tt.verify != nil {
				tt.verify(t, sourceDir, targetDir, buf.String())
			}
		})
	}
}

func TestEnsureGitHooks_NonGitRepoAndDisabledHook(t *testing.T) {
	t.Parallel()

	// 1. Non-git repo should return nil cleanly
	nonGitRoot := t.TempDir()
	if err := EnsureGitHooks(nonGitRoot, nil); err != nil {
		t.Fatalf("EnsureGitHooks on non-git repo should return nil, got: %v", err)
	}

	// 2. Disabled hook in repo should not be overwritten
	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := fileutil.EnsureDir(hooksDir); err != nil {
		t.Fatal(err)
	}
	disabledPreCommit := filepath.Join(hooksDir, "pre-commit.disabled")
	if err := fileutil.WriteStandardFile(disabledPreCommit, []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitHooks(repoRoot, nil); err != nil {
		t.Fatalf("EnsureGitHooks failed: %v", err)
	}

	// pre-commit (active) should not exist because pre-commit.disabled was present
	activePreCommit := filepath.Join(hooksDir, "pre-commit")
	if fileutil.Exists(activePreCommit) {
		t.Errorf("pre-commit active hook should not be created when pre-commit.disabled exists")
	}
	// pre-push should have been created
	activePrePush := filepath.Join(hooksDir, "pre-push")
	if !fileutil.Exists(activePrePush) {
		t.Errorf("pre-push active hook should be created")
	}
}

func TestTestDashboardWarmer(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()

	// Calling WarmTestDashboard without any warmer registered returns nil
	if err := WarmTestDashboard(ctx, projectRoot, nil); err != nil {
		t.Fatalf("WarmTestDashboard with nil warmer returned error: %v", err)
	}

	// Register a mock warmer and verify it gets called
	called := false
	var capturedRoot string
	mockWarmer := func(ctx context.Context, root string, sp storage.ObjectStorageProvider) error {
		called = true
		capturedRoot = root
		return nil
	}

	RegisterTestDashboardWarmer(mockWarmer)
	defer RegisterTestDashboardWarmer(nil)

	if err := WarmTestDashboard(ctx, projectRoot, nil); err != nil {
		t.Fatalf("WarmTestDashboard returned error: %v", err)
	}
	if !called {
		t.Error("expected registered test dashboard warmer to be called")
	}
	if capturedRoot != projectRoot {
		t.Errorf("expected root %s, got %s", projectRoot, capturedRoot)
	}
}

func TestEnabledVerb(t *testing.T) {
	t.Parallel()
	if enabledVerb(true) != "enable" {
		t.Errorf("expected enable, got %s", enabledVerb(true))
	}
	if enabledVerb(false) != "disable" {
		t.Errorf("expected disable, got %s", enabledVerb(false))
	}
}

func TestCompareFiles_EdgeCases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	f1 := filepath.Join(root, "file1")
	f2 := filepath.Join(root, "file2")

	// CRLF vs LF normalization
	if err := fileutil.WriteStandardFile(f1, []byte("hello\r\nworld\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(f2, []byte("hello\nworld\n")); err != nil {
		t.Fatal(err)
	}
	equal, err := compareFiles(f1, f2)
	if err != nil || !equal {
		t.Fatalf("expected CRLF and LF to compare equal, got equal=%v, err=%v", equal, err)
	}

	// Missing first file
	_, err = compareFiles(filepath.Join(root, "missing1"), f2)
	if err == nil {
		t.Error("expected error for missing first file")
	}

	// Missing second file
	_, err = compareFiles(f1, filepath.Join(root, "missing2"))
	if err == nil {
		t.Error("expected error for missing second file")
	}
}

func TestEnsureGitHooks_WithLoggerAndDrift(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := fileutil.EnsureDir(gitDir); err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	logger := logging.NewLogger(buf, logging.InfoLevel, logging.NewJSONFormatter(context.Background()))

	// First pass installs hooks
	if err := EnsureGitHooks(root, logger); err != nil {
		t.Fatalf("EnsureGitHooks failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Installed pre-commit hook") {
		t.Errorf("expected log message for pre-commit installation, got: %s", buf.String())
	}

	// Now drift pre-commit content
	preCommitPath := filepath.Join(gitDir, "hooks", "pre-commit")
	if err := fileutil.WriteExecutableFile(preCommitPath, []byte("#!/bin/sh\n# drifted content\nexit 0\n")); err != nil {
		t.Fatal(err)
	}

	// Second pass should detect drift and reinstall
	buf.Reset()
	if err := EnsureGitHooks(root, logger); err != nil {
		t.Fatalf("EnsureGitHooks failed on drifted hook: %v", err)
	}
	if !strings.Contains(buf.String(), "Installed pre-commit hook") {
		t.Errorf("expected log message for drifted pre-commit update, got: %s", buf.String())
	}

	// Park pre-commit as disabled and verify logger records skip
	disabledPath := preCommitPath + disabledHookSuffix
	if err := fileutil.RenameFile(preCommitPath, disabledPath); err != nil {
		t.Fatal(err)
	}

	buf.Reset()
	if err := EnsureGitHooks(root, logger); err != nil {
		t.Fatalf("EnsureGitHooks failed on disabled hook: %v", err)
	}
	if !strings.Contains(buf.String(), "is manually disabled") {
		t.Errorf("expected log message for manually disabled hook, got: %s", buf.String())
	}
}
func TestPreCommitHook_GofmtFormattingGate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := fileutil.EnsureDir(gitDir); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitHooks(root, nil); err != nil {
		t.Fatalf("EnsureGitHooks failed: %v", err)
	}

	preCommit := filepath.Join(gitDir, "hooks", "pre-commit")
	content, err := fileutil.ReadFile(preCommit)
	if err != nil {
		t.Fatal(err)
	}

	requiredTokens := []string{
		"7. Gofmt Formatting Ratchet Gate",
		`gofmt -l "$gf"`,
		"❌ [ZQK PRE-COMMIT] Formatting violation in staged file: $gf",
		"File is not formatted according to gofmt standards.",
		"Remediation: run 'gofmt -w $gf' and stage the changes.",
		"vendor/*|*/testdata/*|testdata/*",
	}

	for _, token := range requiredTokens {
		if !bytes.Contains(content, []byte(token)) {
			t.Errorf("pre-commit hook missing required token %q", token)
		}
	}

	// Verify template in tools/git-hooks also contains Gate 7
	toolsHook := filepath.Join(root, "tools", "git-hooks", "pre-commit")
	toolsContent, err := fileutil.ReadFile(toolsHook)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range requiredTokens {
		if !bytes.Contains(toolsContent, []byte(token)) {
			t.Errorf("tools/git-hooks/pre-commit missing required token %q", token)
		}
	}
}

func TestSyncGitHooks_ValidatesAndInstallsHookTemplates(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := fileutil.EnsureDir(gitDir); err != nil {
		t.Fatal(err)
	}

	err := EnsureGitHooks(root, nil)
	if err != nil {
		t.Fatalf("EnsureGitHooks failed: %v", err)
	}

	toolsHook := filepath.Join(root, "tools", "git-hooks", "pre-commit")
	installedHook := filepath.Join(gitDir, "hooks", "pre-commit")

	if !fileutil.Exists(toolsHook) {
		t.Fatalf("expected template hook at %s", toolsHook)
	}
	if !fileutil.Exists(installedHook) {
		t.Fatalf("expected installed hook at %s", installedHook)
	}

	// Corrupt installed hook and verify re-sync repairs it
	if err := fileutil.WriteFile(installedHook, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err = EnsureGitHooks(root, nil)
	if err != nil {
		t.Fatalf("EnsureGitHooks repair failed: %v", err)
	}

	equal, err := compareFiles(toolsHook, installedHook)
	if err != nil {
		t.Fatalf("compareFiles: %v", err)
	}
	if !equal {
		t.Fatalf("re-sync did not restore installed hook to match template")
	}
}

func TestSyncGitHooks_PreCommitSupportsZQKPrecommitTimeout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := fileutil.EnsureDir(gitDir); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitHooks(root, nil); err != nil {
		t.Fatalf("EnsureGitHooks failed: %v", err)
	}

	preCommit := filepath.Join(gitDir, "hooks", "pre-commit")
	content, err := fileutil.ReadFile(preCommit)
	if err != nil {
		t.Fatal(err)
	}

	requiredTokens := []string{
		`PRECOMMIT_TIMEOUT="${ZQK_SYSTEM_CHECK_TIMEOUT:-${ZQK_PRECOMMIT_TIMEOUT:-$DEFAULT_TIMEOUT}}"`,
		`system check --tier 1 --timeout "$PRECOMMIT_TIMEOUT"`,
		`DEFAULT_TIMEOUT="120s"`,
		`if [ "$CAS_OBJ_COUNT" -gt 1500 ]; then`,
		`DEFAULT_TIMEOUT="180s"`,
	}

	for _, token := range requiredTokens {
		if !bytes.Contains(content, []byte(token)) {
			t.Errorf("pre-commit hook missing expected timeout token %q", token)
		}
	}
}
