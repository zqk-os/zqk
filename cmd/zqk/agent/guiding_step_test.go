package agent

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/interactionpolicy"
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
