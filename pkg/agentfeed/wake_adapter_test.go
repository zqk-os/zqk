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

func TestNativePeerWakeAdapter(t *testing.T) {
	t.Parallel()
	native := NewNativePeerWakeAdapter()

	// 1. Worker notify
	res, err := native.Wake(nil, PeerWakeRequest{
		SeatKind:     SeatKindWorker,
		DeliveryMode: "notify",
	})
	if err != nil {
		t.Fatalf("native wake worker failed: %v", err)
	}
	if !res.Live || res.Transport != TransportAgentAPINotify {
		t.Errorf("expected live=true, transport=%s; got live=%v transport=%s", TransportAgentAPINotify, res.Live, res.Transport)
	}

	// 2. Worker paste
	res, err = native.Wake(nil, PeerWakeRequest{
		SeatKind:     SeatKindWorker,
		DeliveryMode: "paste",
	})
	if err != nil {
		t.Fatalf("native wake worker paste failed: %v", err)
	}
	if !res.Live || res.Transport != TransportAgentAPINotify {
		t.Errorf("expected live=true, transport=%s; got live=%v transport=%s", TransportAgentAPINotify, res.Live, res.Transport)
	}

	// 3. Coordinator notify (stamp-only)
	res, err = native.Wake(nil, PeerWakeRequest{
		SeatKind:     SeatKindCoordinator,
		DeliveryMode: "notify",
	})
	if err != nil {
		t.Fatalf("native wake coordinator notify failed: %v", err)
	}
	if res.Live || res.Transport != TransportTPMStamp {
		t.Errorf("expected live=false, transport=%s; got live=%v transport=%s", TransportTPMStamp, res.Live, res.Transport)
	}

	// 4. Coordinator paste (live)
	res, err = native.Wake(nil, PeerWakeRequest{
		SeatKind:     SeatKindCoordinator,
		DeliveryMode: "paste",
	})
	if err != nil {
		t.Fatalf("native wake coordinator paste failed: %v", err)
	}
	if !res.Live || res.Transport != TransportTPMPaste {
		t.Errorf("expected live=true, transport=%s; got live=%v transport=%s", TransportTPMPaste, res.Live, res.Transport)
	}
}

func TestShellPeerWakeAdapter_FallbackToNative(t *testing.T) {
	t.Parallel()
	native := NewNativePeerWakeAdapter()
	shellWithFallback := NewShellPeerWakeAdapterWithFallback(native)

	res, err := shellWithFallback.Wake(nil, PeerWakeRequest{
		ProjectRoot:  t.TempDir(),
		SeatKind:     SeatKindWorker,
		DeliveryMode: "notify",
	})
	if err != nil {
		t.Fatalf("expected fallback wake to succeed when scripts absent, got: %v", err)
	}
	if !res.Live || res.Endpoint != "native://internal" {
		t.Errorf("expected native fallback outcome, got: %+v", res)
	}
}

func TestShellPeerWakeAdapter_DefaultHasNativeFallback(t *testing.T) {
	t.Parallel()
	adapter := NewShellPeerWakeAdapter()

	res, err := adapter.Wake(nil, PeerWakeRequest{
		ProjectRoot:  t.TempDir(),
		SeatKind:     SeatKindWorker,
		DeliveryMode: "notify",
	})
	if err != nil {
		t.Fatalf("expected default adapter to fall back to native without error, got: %v", err)
	}
	if !res.Live || res.Endpoint != "native://internal" {
		t.Errorf("expected native fallback endpoint, got: %+v", res)
	}
}
