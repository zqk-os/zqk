package agentfeed

import (
	"strings"
	"testing"
)

func TestWakePasteStub_shortAndPointsAtFeed(t *testing.T) {
	t.Parallel()
	worker := WakePasteStub(WakePasteRoleWorker, "AFE-abc", "peer-agent-2")
	if len(worker) > 220 {
		t.Fatalf("stub too long: %d %q", len(worker), worker)
	}
	for _, want := range []string{attnPrefixPeer, "AFE-abc", "whats-next", "substance on feed", "peer-agent-2"} {
		if !strings.Contains(worker, want) {
			t.Fatalf("worker stub missing %q: %q", want, worker)
		}
	}
	if strings.Contains(worker, FallbackPasteAgentWorker) {
		t.Fatalf("worker stub must use destination seat, not fallback: %q", worker)
	}
	coordinator := WakePasteStub(WakePasteRoleCoordinator, "AFE-xyz", "peer-operator-1")
	for _, want := range []string{attnPrefixCoordinator, "AFE-xyz", "whats-next", "peer-operator-1"} {
		if !strings.Contains(coordinator, want) {
			t.Fatalf("coordinator stub missing %q: %q", want, coordinator)
		}
	}
}

// The --agent-id pointer must carry the destination seat; the event ID belongs in the wake phrase.
func TestWakePasteStub_agentIDPointerIsSeatNotEvent(t *testing.T) {
	t.Parallel()
	got := WakePasteStub(WakePasteRoleWorker, "AFE-abc", "peer-agent-2")
	if !strings.Contains(got, "--agent-id peer-agent-2") {
		t.Fatalf("expected --agent-id to carry the seat: %q", got)
	}
	if strings.Contains(got, "--agent-id AFE-abc") {
		t.Fatalf("event id must not be passed as --agent-id: %q", got)
	}
}

func TestWakePasteStub_emptyAgentFallsBack(t *testing.T) {
	t.Parallel()
	worker := WakePasteStub(WakePasteRoleWorker, "AFE-1", "")
	if !strings.Contains(worker, FallbackPasteAgentWorker) {
		t.Fatalf("expected example worker placeholder: %q", worker)
	}
	if strings.Contains(worker, "antigravity") || strings.Contains(worker, "cursor-composer") {
		t.Fatalf("fallback must not bake vendor seat names: %q", worker)
	}
	coordinator := WakePasteStub(WakePasteRoleCoordinator, "AFE-1", "")
	if !strings.Contains(coordinator, FallbackPasteAgentCoordinator) {
		t.Fatalf("expected example coordinator placeholder: %q", coordinator)
	}
}

func TestWakePasteStubIn_readsSeatIDsFromPeerSeats(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePeerSeats(t, root, map[string]PeerSeatRecord{
		"galaxy-worker-9": {Wake: WakeMembraneAgentAPI, Duty: SeatKindWorker},
		"ide-operator-3":  {Wake: WakeMembraneMCP, Duty: SeatKindCoordinator},
	})
	worker := WakePasteStubIn(root, WakePasteRoleWorker, "AFE-cfg", "")
	if !strings.Contains(worker, "--agent-id galaxy-worker-9") {
		t.Fatalf("worker stub must surface seating-config id: %q", worker)
	}
	coordinator := WakePasteStubIn(root, WakePasteRoleCoordinator, "AFE-cfg", "")
	if !strings.Contains(coordinator, "--agent-id ide-operator-3") {
		t.Fatalf("coordinator stub must surface seating-config id: %q", coordinator)
	}
}

func TestPeerAckPasteStub(t *testing.T) {
	t.Parallel()
	got := PeerAckPasteStub("AFE-1785373283970614000-b6bd98be")
	if got != attnPrefixCoordinator+" — peer_ack received for AFE-1785373283970614000-b6bd98be" {
		t.Fatalf("got=%q", got)
	}
}

func TestResolveWakePasteText_defaultStubNotFullBody(t *testing.T) {
	t.Setenv(envWakePasteFull, "")
	full := "ATTN PEER — long mission body that must not be pasted into chat " + strings.Repeat("x", 400)
	got := ResolveWakePasteText(WakePasteRoleWorker, "AFE-1", full, "peer-agent-1")
	if len(got) > 220 {
		t.Fatalf("expected stub, got len=%d", len(got))
	}
	if strings.Contains(got, "long mission body") {
		t.Fatalf("full body leaked into paste: %q", got)
	}
	if !strings.Contains(got, "AFE-1") || !strings.Contains(got, "peer-agent-1") {
		t.Fatalf("stub missing event id or seat: %q", got)
	}
}

func TestResolveWakePasteText_commsCheckPreservesProtocolMetadata(t *testing.T) {
	t.Setenv(envWakePasteFull, "")
	nonce := "CC-20260814T233038Z-17606"
	full := "COMMS-CHECK " + nonce + ": full challenge body remains on feed"
	got := ResolveWakePasteText(WakePasteRoleWorker, "AFE-123", full, "peer-agent-1")
	for _, want := range []string{nonce, "AFE-123", "peer-agent-1", "whats-next", "feed ack nonce", "feed steer WORK priority+inbox"} {
		if !strings.Contains(got, want) {
			t.Fatalf("COMMS stub missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "full challenge body") {
		t.Fatalf("COMMS notification must remain a doorbell, not copy the full feed body: %q", got)
	}
	if strings.Contains(got, "cursor-composer") || strings.Contains(got, "antigravity") {
		t.Fatalf("COMMS stub must not bake vendor coordinator names: %q", got)
	}
	if !strings.Contains(got, "to "+FallbackPasteAgentCoordinator) {
		t.Fatalf("COMMS stub should point WORK at example coordinator placeholder: %q", got)
	}
}

func TestResolveWakePasteText_fullEscape(t *testing.T) {
	t.Setenv(envWakePasteFull, "1")
	msg := "ATTN PEER — full body for debug"
	got := ResolveWakePasteText(WakePasteRoleWorker, "AFE-1", msg, "peer-agent-1")
	if got != msg {
		t.Fatalf("got=%q", got)
	}
}
