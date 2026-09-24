package swarm

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestSessionAutoInject_FunctionalAcceptance verifies that mcpChildEnviron injects
// ZQK_SESSION and ZQK_SESSION_ID when absent from the parent environment (REQ-SWARM-SESSION-AUTO-INJECT-001).
func TestSessionAutoInject_FunctionalAcceptance(t *testing.T) {
	parent := []string{"PATH=/usr/bin:/bin", "HOME=/tmp"}
	workDir := "/tmp/worktree/ATK-1"

	child := mcpChildEnviron(parent, workDir)

	var hasSession, hasSessionID, hasWorktree bool
	var sessionVal string
	for _, e := range child {
		if strings.HasPrefix(e, "ZQK_SESSION=") {
			hasSession = true
			sessionVal = strings.TrimPrefix(e, "ZQK_SESSION=")
		}
		if strings.HasPrefix(e, "ZQK_SESSION_ID=") {
			hasSessionID = true
		}
		if strings.HasPrefix(e, zqkenv.AgentWorktreeRoot().Name()+"=") {
			hasWorktree = true
		}
	}

	if !hasSession {
		t.Errorf("expected ZQK_SESSION to be auto-injected in child env, got %v", child)
	}
	if !hasSessionID {
		t.Errorf("expected ZQK_SESSION_ID to be auto-injected in child env, got %v", child)
	}
	if sessionVal == "" {
		t.Errorf("auto-injected session value must not be empty")
	}
	if !hasWorktree {
		t.Errorf("expected worktree root env key to be present in child env")
	}
}

// TestSessionAutoInject_BoundaryAndErrorHandling verifies that existing session variables
// are preserved and not duplicated or clobbered.
func TestSessionAutoInject_BoundaryAndErrorHandling(t *testing.T) {
	existingSession := "ZQK-SES-EXPLICIT-12345"
	parent := []string{
		"PATH=/usr/bin",
		"ZQK_SESSION=" + existingSession,
		"ZQK_SESSION_ID=" + existingSession,
	}

	child := mcpChildEnviron(parent, "")

	sessionCount := 0
	sessionIDCount := 0
	for _, e := range child {
		if e == "ZQK_SESSION="+existingSession {
			sessionCount++
		}
		if e == "ZQK_SESSION_ID="+existingSession {
			sessionIDCount++
		}
	}

	if sessionCount != 1 {
		t.Errorf("expected exactly 1 instance of ZQK_SESSION, got %d", sessionCount)
	}
	if sessionIDCount != 1 {
		t.Errorf("expected exactly 1 instance of ZQK_SESSION_ID, got %d", sessionIDCount)
	}
}

// TestSessionAutoInject_IntegrationAndConformance verifies session resolution from the project state file.
func TestSessionAutoInject_IntegrationAndConformance(t *testing.T) {
	tmpRoot := t.TempDir()
	stateDir := filepath.Join(tmpRoot, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}
	expectedSession := "ZQK-SES-FROM-STATE-FILE-999"
	if err := fileutil.WriteFile(filepath.Join(stateDir, "session"), []byte(expectedSession), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}

	t.Setenv(zqkenv.ProjectRoot().Name(), tmpRoot)

	child := mcpChildEnviron([]string{"PATH=/bin"}, "")
	var foundSession string
	for _, e := range child {
		if strings.HasPrefix(e, "ZQK_SESSION=") {
			foundSession = strings.TrimPrefix(e, "ZQK_SESSION=")
			break
		}
	}

	if foundSession != expectedSession {
		t.Errorf("expected session %q resolved from state file, got %q", expectedSession, foundSession)
	}
}
