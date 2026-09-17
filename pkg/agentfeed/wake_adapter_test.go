package agentfeed

import (
	"testing"
)

func TestDefaultVendorBrainMarkers_coverKnownVendors(t *testing.T) {
	t.Parallel()
	p := VendorPathPolicy{ForbiddenMarkers: DefaultVendorBrainMarkers}
	for _, m := range []string{".gemini", ".claude", ".windsurf"} {
		if _, ok := p.FindLeak("/x/" + m + "/y"); !ok {
			t.Fatalf("expected marker %q", m)
		}
	}
}

func TestShellPeerWakeAdapter_scriptPathBySeatKind(t *testing.T) {
	t.Parallel()
	a := NewShellPeerWakeAdapter()
	root := t.TempDir()
	worker := a.scriptPath(root, SeatKindWorker)
	coord := a.scriptPath(root, SeatKindCoordinator)
	if worker == coord {
		t.Fatal("worker and coordinator membranes must differ")
	}
	if got := a.transport(PeerWakeRequest{SeatKind: SeatKindCoordinator, DeliveryMode: "notify"}); got != "tpm_stamp" {
		t.Fatalf("transport=%q", got)
	}
	if got := a.transport(PeerWakeRequest{SeatKind: SeatKindWorker}); got != TransportAgentAPINotify {
		t.Fatalf("transport=%q", got)
	}
}

func TestShellPeerWakeAdapter_liveWorkerDelivery(t *testing.T) {
	t.Parallel()
	a := NewShellPeerWakeAdapter()

	reqNotify := PeerWakeRequest{SeatKind: SeatKindWorker, DeliveryMode: "notify"}
	if !a.live(reqNotify) {
		t.Fatal("successful agentapi notify must be considered a live worker interrupt")
	}

	reqPaste := PeerWakeRequest{SeatKind: SeatKindWorker, DeliveryMode: "paste"}
	if !a.live(reqPaste) {
		t.Fatal("paste DeliveryMode must be considered live")
	}

	reqCoordinatorNotify := PeerWakeRequest{SeatKind: SeatKindCoordinator, DeliveryMode: "notify"}
	if a.live(reqCoordinatorNotify) {
		t.Fatal("coordinator notify is stamp-only until MCP confirms a live interrupt")
	}
}
