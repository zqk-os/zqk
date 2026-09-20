package swarminit

import (
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
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
	t.Parallel()
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
