package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestGitStashGuardOverDocsProcess verifies that git stash operations targeting docs/process are forbidden.
// TRACK: BLI-CEF-R20-STASH-CAS-GUARD-001, CRIT-CEF-R20-STASH-CAS-GUARD-001.
func TestGitStashGuardOverDocsProcess(t *testing.T) {
	t.Parallel()

	server := NewServer()
	ctx := context.Background()

	stashCmds := []string{
		"git stash push docs/process/",
		"git stash -u docs/process",
		"git stash push -m 'wip' docs/process/backlog/item.yaml",
		"git stash",
		"git stash -u",
		"git stash push -u",
		"git clean -f",
		"git clean -fd",
		"git clean -fdx",
	}

	for _, cmd := range stashCmds {
		args := map[string]any{objects.FieldKeyCommand: cmd}
		_, err := server.handleAgentExecuteBashTool(ctx, args)
		if err == nil {
			t.Errorf("expected access denied for git stash over docs/process: %s", cmd)
		}
		if err != nil && !strings.Contains(err.Error(), "access denied") {
			t.Errorf("expected 'access denied' error, got: %v", err)
		}
	}
}
