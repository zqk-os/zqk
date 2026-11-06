package swarminit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewConversationProbe_emptyConversation(t *testing.T) {
	t.Parallel()
	probe := NewConversationProbe("", "")
	err := probe(context.Background(), "peer-agent-1", agentfeed.PeerSeatRecord{})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("got %v", err)
	}
}

func TestNewConversationProbe_noBinFailClosed(t *testing.T) {
	// Isolate LookPathAgentAPI fallback (PATH + antigravity home).
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	probe := NewConversationProbe("", "")
	err := probe(context.Background(), "peer-agent-1", agentfeed.PeerSeatRecord{
		Conversation: "dead-uuid",
		PID:          1,
	})
	if err == nil || !strings.Contains(err.Error(), "trajectory not found") {
		t.Fatalf("want trajectory not found, got %v", err)
	}
}

func TestProbeConversationViaAgentAPI_emptyID(t *testing.T) {
	t.Parallel()
	if err := ProbeConversationViaAgentAPI(context.Background(), "agentapi", "", "  "); err == nil {
		t.Fatal("empty conversation must fail")
	}
}

func TestNewConversationProbe_csrfSoftPass(t *testing.T) {
	t.Parallel()
	bin := writeFakeAgentAPI(t, "#!/bin/sh\necho 'Missing CSRF token' >&2\nexit 1\n")
	probe := NewConversationProbe(bin, "127.0.0.1:9")
	if err := probe(context.Background(), "agy-1", agentfeed.PeerSeatRecord{
		Conversation: "live-uuid",
		PID:          1,
	}); err != nil {
		t.Fatalf("CSRF without trajectory-missing must soft-pass: %v", err)
	}
}

func TestNewConversationProbe_trajectoryStillHardFail(t *testing.T) {
	t.Parallel()
	bin := writeFakeAgentAPI(t, "#!/bin/sh\necho 'trajectory not found' >&2\nexit 1\n")
	probe := NewConversationProbe(bin, "127.0.0.1:9")
	err := probe(context.Background(), "agy-1", agentfeed.PeerSeatRecord{
		Conversation: "dead-uuid",
		PID:          1,
	})
	if err == nil || !strings.Contains(err.Error(), "trajectory not found") {
		t.Fatalf("want trajectory not found, got %v", err)
	}
}

func writeFakeAgentAPI(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "agentapi")
	if err := fileutil.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
