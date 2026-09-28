package system

import (
	"bytes"
	"path/filepath"
	"testing"

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
	if !bytes.Contains(preCommitContent, []byte("Failed to find or compile zqk-vet verification engine! Aborting commit.")) {
		t.Errorf("pre-commit missing fail-closed zqk-vet check")
	}

	prePushContent, err := fileutil.ReadFile(prePush)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(prePushContent, []byte("FAIL-CLOSED BRANCH PROTECTION VIOLATION")) {
		t.Errorf("pre-push missing branch protection check")
	}
	if !bytes.Contains(prePushContent, []byte("CODEBASE VERIFICATION GATE FAILED")) {
		t.Errorf("pre-push missing zqk-vet verification gate")
	}
}
