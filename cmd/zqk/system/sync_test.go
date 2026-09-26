package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestNewSyncCmd(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCmd()
	if cmd == nil {
		t.Fatal("NewSyncCmd() returned nil")
	}

	if cmd.Use != "sync" {
		t.Errorf("Expected command use to be 'sync', got '%s'", cmd.Use)
	}

	if cmd.Short == emptyValue {
		t.Error("Command should have a short description")
	}
}

func TestSyncCommandFlags(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCmd()

	// Test pull flag
	if cmd.Flags().Lookup("pull") == nil {
		t.Error("Command should have --pull flag")
	}

	// Test push flag
	if cmd.Flags().Lookup("push") == nil {
		t.Error("Command should have --push flag")
	}

	// Test verify flag
	if cmd.Flags().Lookup("verify") == nil {
		t.Error("Command should have --verify flag")
	}
}

func TestSyncRequiresGitRepository(t *testing.T) {
	// Create temporary directory for testing (not a git repo)
	tmpDir := t.TempDir()
	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Change to temp directory
	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		_ = fileutil.Chdir(originalDir)
	}()

	// Create a mock command context would be needed here
	// For now, we just test that the command structure is correct
	cmd := NewSyncCmd()
	if cmd == nil {
		t.Fatal("NewSyncCmd() returned nil")
	}
}

func TestSyncWithGitRepository(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping test")
	}

	// Create temporary directory for testing
	tmpDir := t.TempDir()
	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Change to temp directory
	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		_ = fileutil.Chdir(originalDir)
	}()

	// Initialize git repository
	initCmd := testkit.ManagedCommand(t, t.Context(), "git", "init")
	zqkenv.WireExecForIsolatedProject(initCmd, tmpDir)
	if err := initCmd.Run(); err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}

	// Create a test file and commit it
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := fileutil.WriteFile(testFile, []byte("test"), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create test file: %v", err)
	}

	addCmd := testkit.ManagedCommand(t, t.Context(), "git", "add", "test.txt")
	zqkenv.WireExecForIsolatedProject(addCmd, tmpDir)
	if err := addCmd.Run(); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}

	commitCmd := testkit.ManagedCommand(t, t.Context(), "git", "commit", "-m", "Initial commit")
	zqkenv.WireExecForIsolatedProject(commitCmd, tmpDir)
	commitCmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test that sync command can detect git repository
	// Note: Full test would require mocking the command context
	// This is a structural test to ensure the command is set up correctly
	cmd := NewSyncCmd()
	if cmd == nil {
		t.Fatal("NewSyncCmd() returned nil")
	}
}
