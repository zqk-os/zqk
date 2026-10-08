package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zqk-os/zqk/pkg/interactionpolicy"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestResolveGuidingEvent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		event, shell string
		want         string
		unmatched    bool
	}{
		{event: "idle", want: "idle"},
		{shell: "go test ./pkg/foo -timeout 30s", want: interactionpolicy.EventGoTest},
		{shell: "git worktree add .zqk/worktrees/ATK-1 HEAD", want: interactionpolicy.EventGitWorktreeAdd},
		{shell: "git worktree add /tmp/zqk-worktrees/repo/ATK-1 HEAD", unmatched: true},
		{shell: "zqk object get BLI-1", unmatched: true},
		{},
	}
	for _, tc := range cases {
		got, unmatched := resolveGuidingEvent(tc.event, tc.shell)
		if got != tc.want || unmatched != tc.unmatched {
			t.Errorf("resolveGuidingEvent(%q,%q)=(%q,%v) want (%q,%v)",
				tc.event, tc.shell, got, unmatched, tc.want, tc.unmatched)
		}
	}
}

func TestGuidingStep_CLI(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing flags (--event or --shell)
	_, err := executeAgentCommand(t, tempDir, provider, "guiding-step")
	assert.ErrorContains(t, err, "requires --event or --shell")

	// 2. Unmatched shell
	_, err = executeAgentCommand(t, tempDir, provider, "guiding-step", "--shell", "zqk object get BLI-1")
	assert.NoError(t, err)

	// 3. Matched event with persona
	_, err = executeAgentCommand(t, tempDir, provider, "guiding-step", "--event", "idle", "--persona-ref", objects.ConstPersonaDefaultOperator)
	assert.NoError(t, err)

	// 4. Non-existent persona ref
	_, err = executeAgentCommand(t, tempDir, provider, "guiding-step", "--event", "idle", "--persona-ref", "PER-NONEXISTENT")
	assert.Error(t, err)
}
