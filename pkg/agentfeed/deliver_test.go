package agentfeed

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/idebridge"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// fakeWakeAdapter records wake requests without invoking shell membranes.
type fakeWakeAdapter struct {
	mu        sync.Mutex
	calls     []PeerWakeRequest
	endpoint  string
	err       error
	live      bool
	transport string
}

func (f *fakeWakeAdapter) Wake(_ context.Context, req PeerWakeRequest) (PeerWakeAdapterResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	ep := f.endpoint
	if ep == "" {
		ep = "fake://" + req.SeatKind
	}
	tr := f.transport
	if tr == "" {
		if req.SeatKind == SeatKindCoordinator {
			tr = "tpm_stamp"
		} else {
			tr = TransportAgentAPINotify
		}
	}
	live := f.live
	if tr == "tpm_paste" || tr == TransportAgentAPINotify || tr == TransportAGYNotify {
		live = true
	}
	if f.err != nil {
		return PeerWakeAdapterResult{Endpoint: ep, Transport: tr, Live: live}, f.err
	}
	return PeerWakeAdapterResult{Endpoint: ep, Transport: tr, Live: live}, nil
}

func (f *fakeWakeAdapter) last() PeerWakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return PeerWakeRequest{}
	}
	return f.calls[len(f.calls)-1]
}

func TestShouldWakePeer(t *testing.T) {
	if !ShouldWakePeer(datacell.DeliveryModeNotify) {
		t.Fatal("notify should wake")
	}
	if !ShouldWakePeer(datacell.DeliveryModePaste) {
		t.Fatal("paste should wake")
	}
	if ShouldWakePeer(datacell.DeliveryModeLog) {
		t.Fatal("log should not wake")
	}
	if ShouldWakePeer(datacell.DeliveryModeOff) {
		t.Fatal("off should not wake")
	}
}

func TestSeatKindForAgent(t *testing.T) {
	for _, id := range []string{"peer-tpm-02", "peer-tpm-01", "tpm", "TPM-seat"} {
		if SeatKindForAgent(id) != SeatKindCoordinator {
			t.Fatalf("expected coordinator seat: %q", id)
		}
	}
	for _, id := range []string{"peer-agent-02", "peer-agent-01", ""} {
		if SeatKindForAgent(id) != SeatKindWorker {
			t.Fatalf("expected worker seat: %q", id)
		}
	}
}

func TestWakePeer_routesBySeatKindViaAdapter(t *testing.T) {
	fake := &fakeWakeAdapter{endpoint: "adapter://coordinator"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res := WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "peer-tpm-02",
		Message:      "ATTN TPM: coordination online probe",
		DeliveryMode: datacell.DeliveryModeNotify,
		Adapter:      fake,
	})
	if res.Error != "" {
		t.Fatalf("wake: %s", res.Error)
	}
	if !res.Attempted {
		t.Fatal("expected attempted wake")
	}
	if got := fake.last().SeatKind; got != SeatKindCoordinator {
		t.Fatalf("seat kind=%q", got)
	}
	if res.Transport != "tpm_stamp" {
		t.Fatalf("transport=%q", res.Transport)
	}
	if res.Live {
		t.Fatal("coordinator notify stamp must not be live")
	}
}

func TestWakePeer_workerSeatViaAdapter(t *testing.T) {
	fake := &fakeWakeAdapter{endpoint: "adapter://worker"}
	ctx := context.Background()
	res := WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "peer-agent-02",
		Message:      "hello worker",
		DeliveryMode: datacell.DeliveryModeNotify,
		Adapter:      fake,
	})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if fake.last().SeatKind != SeatKindWorker {
		t.Fatalf("seat=%q", fake.last().SeatKind)
	}
	if res.Transport != TransportAgentAPINotify || !res.Live {
		t.Fatalf("transport=%q live=%v (expected live agentapi notify)", res.Transport, res.Live)
	}
	if !res.IdeBridgeQueued {
		t.Fatal("expected ide_bridge_queued after live worker notify")
	}
}

func TestWakePeer_workerSeatPasteIsLive(t *testing.T) {
	fake := &fakeWakeAdapter{endpoint: "adapter://worker"}
	ctx := context.Background()
	res := WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "peer-agent-02",
		Message:      "hello worker",
		DeliveryMode: datacell.DeliveryModePaste,
		Adapter:      fake,
	})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if fake.last().SeatKind != SeatKindWorker {
		t.Fatalf("seat=%q", fake.last().SeatKind)
	}
	if res.Transport != TransportAgentAPINotify || !res.Live {
		t.Fatalf("transport=%q live=%v (expected true for paste)", res.Transport, res.Live)
	}
	if !res.IdeBridgeQueued {
		t.Fatal("expected ide_bridge_queued after live worker notify")
	}
}

func TestWakePeer_coordinatorMCPInterruptIsLive(t *testing.T) {
	fake := &fakeWakeAdapter{endpoint: "adapter://coordinator"}
	res := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot:          t.TempDir(),
		ToAgentID:            "peer-tpm-01",
		Message:              "ATTN TPM: hello from peer",
		DeliveryMode:         datacell.DeliveryModeNotify,
		Adapter:              fake,
		MCPIPCDelivered:      true,
		MCPSubscribersProbed: true,
		MCPSubscriberCount:   2,
	})
	if res.Error != "" {
		t.Fatalf("wake: %s", res.Error)
	}
	if !res.Live {
		t.Fatal("MCP ActionRequired with subscribers must be live")
	}
	if res.Transport != TransportMCPActionRequired {
		t.Fatalf("transport=%q", res.Transport)
	}
	if unrepaired, _, _ := IsUnrepairedWake(res); unrepaired {
		t.Fatal("live MCP interrupt must not be unrepaired")
	}
	if !res.IdeBridgeQueued {
		t.Fatal("expected ide_bridge_queued after live MCP wake")
	}
}

func TestWakePeer_coordinatorNoSubscriberNotLive(t *testing.T) {
	fake := &fakeWakeAdapter{endpoint: "adapter://coordinator"}
	res := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot:          t.TempDir(),
		ToAgentID:            "peer-tpm-01",
		Message:              "ATTN TPM",
		DeliveryMode:         datacell.DeliveryModeNotify,
		Adapter:              fake,
		MCPIPCDelivered:      true,
		MCPSubscribersProbed: true,
		MCPSubscriberCount:   0,
	})
	if res.Live {
		t.Fatal("zero IDE subscribers must not be live")
	}
	if res.Transport != TransportMCPNoSubscriber {
		t.Fatalf("transport=%q", res.Transport)
	}
	if unrepaired, reason, _ := IsUnrepairedWake(res); !unrepaired || reason != UnrepairedWakeStampNotLive {
		t.Fatalf("expected stamp_not_live unrepaired, got unrepaired=%v reason=%s", unrepaired, reason)
	}
}

func TestApplyCoordinatorMCPInterrupt_ignoresWorkers(t *testing.T) {
	res := PeerWakeResult{Attempted: true, Live: true, Transport: TransportAGYNotify}
	ApplyCoordinatorMCPInterrupt(&res, SeatKindWorker, datacell.DeliveryModeNotify, true, 0, true)
	if !res.Live || res.Transport != TransportAGYNotify {
		t.Fatalf("worker wake must be unchanged: live=%v transport=%q", res.Live, res.Transport)
	}
}

func TestApplyCoordinatorMCPInterrupt_unprobedLeavesStamp(t *testing.T) {
	res := PeerWakeResult{Attempted: true, Live: false, Transport: TransportTPMStamp}
	ApplyCoordinatorMCPInterrupt(&res, SeatKindCoordinator, datacell.DeliveryModeNotify, true, 0, false)
	if res.Transport != TransportTPMStamp || res.Live {
		t.Fatalf("unprobed must leave stamp: live=%v transport=%q", res.Live, res.Transport)
	}
}

func TestWakePeer_defaultModeIsNotify(t *testing.T) {
	fake := &fakeWakeAdapter{}
	res := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot: t.TempDir(),
		ToAgentID:   "peer-agent-02",
		Message:     "ping",
		Adapter:     fake,
	})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if fake.last().DeliveryMode != datacell.DeliveryModeNotify {
		t.Fatalf("mode=%q", fake.last().DeliveryMode)
	}
}

func TestWakePeer_pasteMode(t *testing.T) {
	fake := &fakeWakeAdapter{transport: "tpm_paste", live: true}
	res := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot:  t.TempDir(),
		ToAgentID:    "peer-tpm-02",
		Message:      "paste me",
		DeliveryMode: datacell.DeliveryModePaste,
		Adapter:      fake,
	})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if fake.last().DeliveryMode != datacell.DeliveryModePaste {
		t.Fatalf("mode=%q", fake.last().DeliveryMode)
	}
	if strings.TrimSpace(res.PasteText) == "" {
		t.Fatal("expected paste text")
	}
}

func TestWakePeer_missingMembraneSkipped(t *testing.T) {
	// Shell adapter without fallback on empty project: no scripts → skipped, not hard error.
	prev := CurrentPeerWakeAdapter()
	SetPeerWakeAdapter(NewShellPeerWakeAdapterWithFallback(nil))
	t.Cleanup(func() { SetPeerWakeAdapter(prev) })

	res := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot: t.TempDir(),
		ToAgentID:   "peer-agent-02",
		Message:     "no membrane",
	})
	if res.Skipped != "wake_script_missing" {
		t.Fatalf("skipped=%q err=%q", res.Skipped, res.Error)
	}
}

// TestWakePeer_E2EProof_LiveWakesCursorTPM verifies that a steer message targeting
// a wake=mcp seat queues zqk.wake.attn explicitly.
// Satisfies BLI-COMMS-CURSOR-TPM-DELIVER-ATTN-001 / BLI-COMMS-CURSOR-TPM-E2E-PROOF-001.
func TestWakePeer_E2EProof_LiveWakesCursorTPM(t *testing.T) {
	root := t.TempDir()
	fake := &fakeWakeAdapter{endpoint: "adapter://coordinator"}

	msg := "ATTN TPM: test steer for cursor composer"
	res := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot:          root,
		ToAgentID:            "peer-tpm-01",
		Message:              msg,
		DeliveryMode:         datacell.DeliveryModeNotify,
		Adapter:              fake,
		MCPIPCDelivered:      true,
		MCPSubscribersProbed: true,
		MCPSubscriberCount:   1, // Trigger live MCP interrupt
	})

	if res.Error != "" {
		t.Fatalf("wake: %s", res.Error)
	}
	if !res.Live || res.Transport != TransportMCPActionRequired {
		t.Fatalf("MCP ActionRequired with subscribers must be live, got live=%v transport=%s", res.Live, res.Transport)
	}
	if !res.IdeBridgeQueued {
		t.Fatal("expected ide_bridge_queued after live MCP wake")
	}

	data, err := fileutil.ReadFile(idebridge.ControlJSONLPath(root))
	if err != nil {
		t.Fatalf("failed to read bridge control JSONL: %v", err)
	}
	sdata := string(data)
	if !strings.Contains(sdata, "zqk.wake.attn") {
		t.Fatalf("expected zqk.wake.attn in queue, got:\n%s", sdata)
	}
	if !strings.Contains(sdata, "test steer for cursor composer") {
		t.Fatalf("expected steer message in queue, got:\n%s", sdata)
	}
}
