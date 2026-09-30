package agentfeed

import (
	"context"
	"strings"
	"sync/atomic"
)

// Seat kind constants classify destination seats for wake routing.
// Adapters map these kinds to concrete transports (MCP, HTTP, shell membrane, …).
// "coordinator" means MCP/IDE-class membrane — not a IDE-only channel.
const (
	SeatKindWorker      = "worker"      // agentapi / terminal-class peer
	SeatKindCoordinator = "coordinator" // MCP / IDE-class peer (any vendor)
)

// PeerWakeRequest is the vendor-neutral wake request handed to a PeerWakeAdapter.
type PeerWakeRequest struct {
	ProjectRoot  string
	ToAgentID    string
	SeatKind     string
	DeliveryMode string
	PasteText    string
	PeerPID      int
	Conversation string
	Message      string
}

// PeerWakeAdapterResult is the adapter outcome (no shell/script assumptions).
type PeerWakeAdapterResult struct {
	// Endpoint is an opaque diagnostic handle (URL, adapter id, membrane path).
	Endpoint  string
	Transport string
	Live      bool
	Output    string
}

// PeerWakeAdapter delivers a wake signal to a peer seat.
// Core feed code must depend on this interface — not on vendor shell scripts.
//
// shell membrane with MCP/HTTP adapters as those become the product transport.
type PeerWakeAdapter interface {
	Wake(ctx context.Context, req PeerWakeRequest) (PeerWakeAdapterResult, error)
}

// peerWakeAdapterBox wraps the interface so we can store it in atomic.Pointer
// (interfaces themselves are not a valid atomic.Pointer element type).
type peerWakeAdapterBox struct {
	adapter PeerWakeAdapter
}

var peerWakeAdapter atomic.Pointer[peerWakeAdapterBox]

func init() {
	peerWakeAdapter.Store(&peerWakeAdapterBox{adapter: NewShellPeerWakeAdapter()})
}

// SetPeerWakeAdapter replaces the process-wide wake adapter (tests / composition).
func SetPeerWakeAdapter(a PeerWakeAdapter) {
	if a == nil {
		a = NewShellPeerWakeAdapter()
	}
	peerWakeAdapter.Store(&peerWakeAdapterBox{adapter: a})
}

// CurrentPeerWakeAdapter returns the active wake adapter.
func CurrentPeerWakeAdapter() PeerWakeAdapter {
	if box := peerWakeAdapter.Load(); box != nil && box.adapter != nil {
		return box.adapter
	}
	return NewShellPeerWakeAdapter()
}

// SeatKindForAgent classifies a destination agent id for adapter routing.
// Prefer SeatKindForAgentIn(projectRoot, id) when a project root is available.
func SeatKindForAgent(toAgentID string) string {
	if strings.TrimSpace(toAgentID) == "" {
		return SeatKindWorker
	}
	return SeatKindForWakeMembrane(ResolveWakeMembrane("", toAgentID))
}

// SeatKindForAgentIn classifies using peer_seats.wake when present.
func SeatKindForAgentIn(projectRoot, toAgentID string) string {
	if strings.TrimSpace(toAgentID) == "" {
		return SeatKindWorker
	}
	return SeatKindForWakeMembrane(ResolveWakeMembrane(projectRoot, toAgentID))
}
