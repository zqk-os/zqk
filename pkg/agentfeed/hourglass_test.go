package agentfeed

import (
	"strings"
	"testing"
)

func TestEnforceDirectedHourglass_broadcastOK(t *testing.T) {
	t.Parallel()
	if err := EnforceDirectedHourglass("", false); err != nil {
		t.Fatalf("broadcast without await: %v", err)
	}
	if err := EnforceDirectedHourglass("  ", false); err != nil {
		t.Fatalf("whitespace to_agent_id without await: %v", err)
	}
}

func TestEnforceDirectedHourglass_directedRequiresAwait(t *testing.T) {
	t.Parallel()
	err := EnforceDirectedHourglass("peer-agent-1", false)
	if err == nil {
		t.Fatal("expected fail-closed for directed without await")
	}
	if !strings.Contains(err.Error(), "POL-AGENT-ORCH-HOURGLASS-001") {
		t.Fatalf("want policy id in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "--await-peer-ack") {
		t.Fatalf("want await flag hint, got %v", err)
	}
}

func TestEnforceDirectedHourglass_directedWithAwaitOK(t *testing.T) {
	t.Parallel()
	if err := EnforceDirectedHourglass("peer-agent-1", true); err != nil {
		t.Fatalf("directed with await: %v", err)
	}
}
