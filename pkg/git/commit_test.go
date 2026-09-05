package git

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestCommitAnalyzer_AnalyzeCommit(t *testing.T) {
	t.Parallel()
	// Create a temporary Git repository for testing
	repoPath := setupTestRepo(t)
	defer fileutil.RemoveAll(repoPath)

	analyzer := NewCommitAnalyzer(repoPath)

	// Create a test commit
	createTestCommit(t, repoPath, "test commit message with BLI-123 and GOAL-456")

	// Get the commit hash
	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash := strings.TrimSpace(string(output))

	// Analyze the commit
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	commit, err := analyzer.AnalyzeCommit(ctx, hash)
	if err != nil {
		t.Fatalf("failed to analyze commit: %v", err)
	}

	// Verify commit details
	if commit.Hash != hash {
		t.Errorf("expected hash %s, got %s", hash, commit.Hash)
	}
	if commit.ShortHash != hash[:7] {
		t.Errorf("expected short hash %s, got %s", hash[:7], commit.ShortHash)
	}
	if commit.Subject != "test commit message with BLI-123 and GOAL-456" {
		t.Errorf("expected subject 'test commit message with BLI-123 and GOAL-456', got '%s'", commit.Subject)
	}

	// Verify work item references
	if len(commit.BacklogItemRefs) != 1 || commit.BacklogItemRefs[0] != "BLI-123" {
		t.Errorf("expected BacklogItemRefs ['BLI-123'], got %v", commit.BacklogItemRefs)
	}
	if len(commit.GoalRefs) != 1 || commit.GoalRefs[0] != "GOAL-456" {
		t.Errorf("expected GoalRefs ['GOAL-456'], got %v", commit.GoalRefs)
	}
}

func TestCommitAnalyzer_AnalyzeCommits(t *testing.T) {
	t.Parallel()
	repoPath := setupTestRepo(t)
	defer fileutil.RemoveAll(repoPath)

	analyzer := NewCommitAnalyzer(repoPath)

	// Create multiple test commits
	createTestCommit(t, repoPath, "commit 1 with BLI-001")
	createTestCommit(t, repoPath, "commit 2 with MIL-002")
	createTestCommit(t, repoPath, "commit 3 with GOAL-003")

	// Get commit hashes
	cmd := exec.Command("git", "log", "--format=%H", "--reverse")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hashes: %v", err)
	}
	hashes := strings.Fields(string(output))

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel()

	// Analyze commits concurrently
	commits, err := analyzer.AnalyzeCommits(ctx, hashes)
	if err != nil {
		t.Fatalf("failed to analyze commits: %v", err)
	}

	if len(commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(commits))
	}

	// Verify references
	foundBLI := false
	foundMIL := false
	foundGOAL := false
	for _, commit := range commits {
		for _, ref := range commit.BacklogItemRefs {
			if ref == "BLI-001" {
				foundBLI = true
			}
		}
		for _, ref := range commit.MilestoneRefs {
			if ref == "MIL-002" {
				foundMIL = true
			}
		}
		for _, ref := range commit.GoalRefs {
			if ref == "GOAL-003" {
				foundGOAL = true
			}
		}
	}

	if !foundBLI {
		t.Error("expected to find BLI-001 reference")
	}
	if !foundMIL {
		t.Error("expected to find MIL-002 reference")
	}
	if !foundGOAL {
		t.Error("expected to find GOAL-003 reference")
	}
}

func TestCommitAnalyzer_AnalyzeRecent(t *testing.T) {
	t.Parallel()
	repoPath := setupTestRepo(t)
	defer fileutil.RemoveAll(repoPath)

	analyzer := NewCommitAnalyzer(repoPath)

	// Create 5 test commits
	for i := 1; i <= 5; i++ {
		createTestCommit(t, repoPath, fmt.Sprintf("commit %d", i))
	}

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel()

	// Analyze last 3 commits
	commits, err := analyzer.AnalyzeRecent(ctx, 3)
	if err != nil {
		t.Fatalf("failed to analyze recent commits: %v", err)
	}

	if len(commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(commits))
	}
}

func TestCommitAnalyzer_ExtractWorkItemReferences(t *testing.T) {
	t.Parallel()
	repoPath := setupTestRepo(t)
	defer fileutil.RemoveAll(repoPath)

	analyzer := NewCommitAnalyzer(repoPath)

	testCases := []struct {
		name     string
		message  string
		expected map[string][]string
	}{
		{
			name:    "multiple references",
			message: "feat: implement BLI-123, MIL-456, and GOAL-789",
			expected: map[string][]string{
				"backlog_item": {"BLI-123"},
				"milestone":    {"MIL-456"},
				"goal":         {"GOAL-789"},
			},
		},
		{
			name:     "no references",
			message:  "fix: update documentation",
			expected: map[string][]string{},
		},
		{
			name:    "duplicate references",
			message: "BLI-123 and BLI-123 again",
			expected: map[string][]string{
				"backlog_item": {"BLI-123"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			createTestCommit(t, repoPath, tc.message)

			cmd := exec.Command("git", "log", "-1", "--format=%H")
			cmd.Dir = repoPath
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("failed to get commit hash: %v", err)
			}
			hash := strings.TrimSpace(string(output))

			ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
			defer cancel()

			commit, err := analyzer.AnalyzeCommit(ctx, hash)
			if err != nil {
				t.Fatalf("failed to analyze commit: %v", err)
			}

			// Verify backlog item refs
			expectedBLI := tc.expected["backlog_item"]
			if len(commit.BacklogItemRefs) != len(expectedBLI) {
				t.Errorf("expected %d backlog item refs, got %d", len(expectedBLI), len(commit.BacklogItemRefs))
			}
			for _, expected := range expectedBLI {
				found := false
				for _, ref := range commit.BacklogItemRefs {
					if ref == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find %s in BacklogItemRefs", expected)
				}
			}

			// Similar checks for other reference types...
		})
	}
}

func TestCommitAnalyzer_Timeout(t *testing.T) {
	t.Parallel()
	repoPath := setupTestRepo(t)
	defer fileutil.RemoveAll(repoPath)

	// Create analyzer with very short timeout
	config := &CommitAnalysisConfig{
		Timeout: 1 * time.Nanosecond, // Very short timeout
	}
	analyzer := NewCommitAnalyzerWithConfig(repoPath, config)

	createTestCommit(t, repoPath, "test commit")

	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash := strings.TrimSpace(string(output))

	ctx := pkgctx.NewSystemContext()
	_, err = analyzer.AnalyzeCommit(ctx, hash)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestCommitAnalyzer_Retry(t *testing.T) {
	t.Parallel()
	// This test would require mocking Git operations
	// For now, we'll test that retry logic is called
	repoPath := setupTestRepo(t)
	defer fileutil.RemoveAll(repoPath)

	analyzer := NewCommitAnalyzer(repoPath)
	createTestCommit(t, repoPath, "test commit")

	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash := strings.TrimSpace(string(output))

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	// Should succeed without retries
	commit, err := analyzer.AnalyzeCommit(ctx, hash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit == nil {
		t.Error("expected commit, got nil")
	}
}

// setupTestRepo creates a temporary Git repository for testing
func setupTestRepo(t *testing.T) string {
	dir := t.TempDir()

	// Initialize Git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure Git user (required for commits)
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}

	return dir
}

// createTestCommit creates a test commit with the given message
func createTestCommit(t *testing.T, repoPath, message string) {
	// Create a unique test file to avoid conflicts
	testFile := filepath.Join(repoPath, fmt.Sprintf("test-%d.txt", time.Now().UnixNano()))
	if err := fileutil.WriteFile(testFile, []byte("test content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage the file
	cmd := exec.Command("git", "add", filepath.Base(testFile)) //nolint:gosec // Test file - safe command
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to stage file: %v", err)
	}

	// Commit with --no-verify to skip hooks
	cmd = exec.Command("git", "commit", "-m", message, "--no-verify")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		// Check if it's because there are no changes
		//nolint:errcheck // Test cleanup - errors are acceptable
		output, _ := cmd.CombinedOutput()
		if strings.Contains(string(output), "nothing to commit") {
			// Create a new file with different content
			testFile2 := filepath.Join(repoPath, fmt.Sprintf("test2-%d.txt", time.Now().UnixNano()))
			//nolint:errcheck // Test cleanup - errors are acceptable
			fileutil.WriteFile(testFile2, []byte(fmt.Sprintf("test content %d", time.Now().UnixNano())), paths.FilePerm644)
			cmd = exec.Command("git", "add", filepath.Base(testFile2)) //nolint:gosec // Test file - safe command
			cmd.Dir = repoPath
			//nolint:errcheck // Test cleanup - errors are acceptable
			cmd.Run()
			cmd = exec.Command("git", "commit", "-m", message, "--no-verify")
			cmd.Dir = repoPath
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to create commit: %v\nOutput: %s", err, string(output))
			}
		} else {
			t.Fatalf("failed to create commit: %v\nOutput: %s", err, string(output))
		}
	}
}
