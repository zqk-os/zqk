package datacell

import (
	"github.com/zqk-os/zqk/pkg/paths"
	"path/filepath"
	"testing"
)

func TestEffectiveAgentChatChannelEventsJSONLPath_overrideRelative(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	relPath := filepath.Join(paths.ProjectDataDir, paths.LogsDir, "ide-hooks", "x.jsonl")
	cfg := AgentChatChannelConfig{EventsJSONLPathOverride: relPath}
	got := EffectiveAgentChatChannelEventsJSONLPath(root, cfg)
	want := filepath.Join(root, relPath)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEffectiveAgentChatChannelEventsJSONLPath_default(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got := EffectiveAgentChatChannelEventsJSONLPath(root, AgentChatChannelConfig{})
	want := AgentChatChannelEventsJSONLPath(root)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
