package main_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestZQKShim_E2E(t *testing.T) {
	// Setup: compile the shim binary
	tempDir := t.TempDir()
	shimBinary := filepath.Join(tempDir, "zqk-shim")

	buildCmd := execwrap.Command("go", "build", "-o", shimBinary, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build zqk-shim: %v\nOutput: %s", err, out)
	}

	// Create a fake real binary for git and gh to ensure we intercept
	fakeBinDir := filepath.Join(tempDir, "fake-bin")
	if err := fileutil.EnsureDir(fakeBinDir); err != nil {
		t.Fatalf("Failed to create fake-bin dir: %v", err)
	}

	fakeGit := filepath.Join(fakeBinDir, "git")
	gitScript := "#!/bin/sh\necho 'intercepted git:' \"$@\"\n"
	if err := fileutil.WriteFile(fakeGit, []byte(gitScript), 0755); err != nil {
		t.Fatalf("Failed to write fake git: %v", err)
	}

	fakeGh := filepath.Join(fakeBinDir, "gh")
	ghScript := "#!/bin/sh\necho 'intercepted gh:' \"$@\"\n"
	if err := fileutil.WriteFile(fakeGh, []byte(ghScript), 0755); err != nil {
		t.Fatalf("Failed to write fake gh: %v", err)
	}

	// Setup PATH to point to our fake binaries first
	originalPath := zqkenv.OSPath().Get()
	newPath := fakeBinDir + string(fileutil.PathListSeparator) + originalPath

	tests := []struct {
		name       string
		shimName   string
		args       []string
		wantSubstr string
	}{
		{
			name:       "git commit",
			shimName:   "git",
			args:       []string{"commit", "-m", "test e2e"},
			wantSubstr: "intercepted git: commit -m test e2e",
		},
		{
			name:       "gh pr create",
			shimName:   "gh",
			args:       []string{"pr", "create", "--title", "test e2e"},
			wantSubstr: "intercepted gh: pr create --title test e2e",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a symlink in a 'shim-bin' directory
			shimBinDir := filepath.Join(tempDir, "shim-bin")
			_ = fileutil.EnsureDir(shimBinDir)
			shimLink := filepath.Join(shimBinDir, tt.shimName)
			if err := fileutil.Symlink(shimBinary, shimLink); err != nil && !fileutil.IsExist(err) {
				t.Fatalf("Failed to create symlink: %v", err)
			}
			// We need to bypass POL-CODE-009 validation or ensure it doesn't run.
			// Set up a fake project root so logging writes to .zqk/logs instead of failing
			_ = fileutil.EnsureDir(filepath.Join(tempDir, paths.ProjectDataDir, "logs"))
			cmd := execwrap.Command(shimLink, tt.args...)
			cmd.Dir = tempDir
			cmd.Env = []string{
				zqkenv.OSPath().Name() + "=" + shimBinDir + string(fileutil.PathListSeparator) + newPath,
				zqkenv.OSHome().Name() + "=" + tempDir,
				"ZQK_SHIM_BYPASS_POLCODE009=1",
				"GIT_ZQK_SHIM_BYPASS_POLCODE009=1",
				"GH_ZQK_SHIM_BYPASS_POLCODE009=1",
				"ZQK_BREAK_GLASS_REASON=testing shim e2e execution under isolated test harness",
				"ZQK_PROJECT_ROOT=" + tempDir,
				"ZQK_LOG_LEVEL=debug",
				"ZQK_PROFILE=system",
			}

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			outStr := stdout.String()
			errStr := stderr.String()

			if err != nil {
				t.Fatalf("Shim execution failed: %v\nStdout: %s\nStderr: %s", err, outStr, errStr)
			}

			// Ensure we captured the output of the intercepted command
			if !strings.Contains(outStr, tt.wantSubstr) {
				t.Errorf("Expected stdout to contain %q, got: %s", tt.wantSubstr, outStr)
			}

			// Verify telemetry events were fired by checking the system log file.
			// Verify telemetry events were fired.
			// TelemetryInterceptor outputs JSON telemetry to stderr.
			if !strings.Contains(errStr, "cli_exec_start") {
				t.Errorf("Expected telemetry 'cli_exec_start' in stderr, got: %s", errStr)
			}
			if !strings.Contains(errStr, "cli_exec_success") {
				t.Errorf("Expected telemetry 'cli_exec_success' in stderr, got: %s", errStr)
			}
		})
	}

	t.Run("fails when bypass specified without break glass reason", func(t *testing.T) {
		shimBinDir := filepath.Join(tempDir, "shim-bin-fail")
		_ = fileutil.EnsureDir(shimBinDir)
		shimLink := filepath.Join(shimBinDir, "git")
		if err := fileutil.Symlink(shimBinary, shimLink); err != nil && !fileutil.IsExist(err) {
			t.Fatalf("Failed to create symlink: %v", err)
		}
		_ = fileutil.EnsureDir(filepath.Join(tempDir, paths.ProjectDataDir, "logs"))

		cmd := execwrap.Command(shimLink, "commit", "-m", "should fail")
		cmd.Dir = tempDir
		cmd.Env = []string{
			zqkenv.OSPath().Name() + "=" + shimBinDir + string(fileutil.PathListSeparator) + newPath,
			zqkenv.OSHome().Name() + "=" + tempDir,
			"ZQK_SHIM_BYPASS_POLCODE009=1",
			"ZQK_PROJECT_ROOT=" + tempDir,
			"ZQK_LOG_LEVEL=debug",
			"ZQK_PROFILE=system",
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err == nil {
			t.Fatal("Expected command to fail due to unvalidated bypass without break-glass reason, but it succeeded")
		}
	})
}
