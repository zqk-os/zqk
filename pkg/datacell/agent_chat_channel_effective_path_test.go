package datacell

import (
	"path/filepath"
	"testing"
)

func TestEffectiveAgentChatChannelEventsJSONLPath_overrideRelative(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := AgentChatChannelConfig{EventsJSONLPathOverride: ".zqk/logs/ide-hooks/x.jsonl"}
	got := EffectiveAgentChatChannelEventsJSONLPath(root, cfg)
	want := filepath.Join(root, ".zqk/logs/ide-hooks/x.jsonl")
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
