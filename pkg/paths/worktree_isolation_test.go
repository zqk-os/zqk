package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestWorktreeIsolation_FunctionalAcceptance verifies that AgentWorktreeDir assigns
// paths strictly outside the primary project tree (REQ-SWARM-WORKTREE-ISOLATION-001).
func TestWorktreeIsolation_FunctionalAcceptance(t *testing.T) {
	proj := t.TempDir()
	taskID := "ATK-TEST-WORKTREE-001"
	wt := AgentWorktreeDir(proj, taskID)

	rel, err := filepath.Rel(proj, wt)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != ".." {
		t.Fatalf("worktree must not live under project root: project=%s wt=%s rel=%s", proj, wt, rel)
	}
	if !strings.Contains(wt, taskID) {
		t.Fatalf("expected worktree path to contain task id %s: got %s", taskID, wt)
	}
}

// TestWorktreeIsolation_BoundaryAndErrorHandling verifies handling of empty task IDs
// and refusal of nested project overrides.
func TestWorktreeIsolation_BoundaryAndErrorHandling(t *testing.T) {
	proj := t.TempDir()

	// Empty / whitespace task id should fallback to unnamed
	unnamed := AgentWorktreeDir(proj, "   ")
	if !strings.Contains(unnamed, "unnamed") {
		t.Fatalf("expected empty task id to yield 'unnamed' path segment, got %s", unnamed)
	}

	// Refuse in-project override even if environment variable points inside project
	inside := filepath.Join(proj, ".zqk", "worktrees")
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), inside)
	wt := AgentWorktreeDir(proj, "ATK-BOUNDARY-001")
	rel, err := filepath.Rel(proj, wt)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		t.Fatalf("refused in-project override: still under project: %s", wt)
	}
}

// TestWorktreeIsolation_IntegrationAndConformance verifies custom worktree root environment variable overrides.
func TestWorktreeIsolation_IntegrationAndConformance(t *testing.T) {
	base := t.TempDir()
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), base)
	proj := filepath.Join(base, "proj")
	taskID := "ATK-CONFORMANCE-001"
	got := AgentWorktreeDir(proj, taskID)
	want := filepath.Join(base, taskID)
	if got != want {
		t.Fatalf("worktree override mismatch: got %s want %s", got, want)
	}
}
