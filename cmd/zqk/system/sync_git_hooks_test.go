package system

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
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
