package agentfeed

import (
	"context"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// PeerWakeResult is the outcome of a best-effort peer wake after feed append.
type PeerWakeResult struct {
	Attempted          bool   `json:"attempted"`
	Skipped            string `json:"skipped,omitempty"`
	SkippedReason      string `json:"skipped_reason,omitempty"`
	WakeAuthorExcluded bool   `json:"wake_author_excluded,omitempty"`
	Script             string `json:"script,omitempty"` // legacy JSON key; opaque endpoint from adapter
	Error              string `json:"error,omitempty"`
	DeliveryReceipt    bool   `json:"delivery_receipt,omitempty"`
	DeliveryEventID    string `json:"delivery_event_id,omitempty"`
	InReplyTo          string `json:"in_reply_to,omitempty"`
	PasteText          string `json:"paste_text,omitempty"` // chat paste (stub by default)
	// Transport names the membrane used (e.g. tpm_stamp, agentapi_notify, tpm_paste, mcp_action_required).
	Transport string `json:"transport,omitempty"`
	// Live is true only when the membrane can resume a running peer turn
	// (AGY agentapi notify / MCP ActionRequired with IDE subscribers / explicit paste).
	// TPM stamp-only is NOT live
	Live bool `json:"live"`
	// IdeBridgeQueued is true when a zqk.wake.attn control event was appended for the IDE bridge.
	IdeBridgeQueued bool `json:"ide_bridge_queued,omitempty"`
}

// Wake transport names (stable for CLI JSON + unrepaired classification).
const (
	TransportTPMStamp          = "tpm_stamp"
	TransportTPMPaste          = "tpm_paste"
	TransportAgentAPINotify    = "agentapi_notify"
	TransportAGYNotify         = "agy_notify" // deprecated alias of TransportAgentAPINotify
	TransportMCPActionRequired = "mcp_action_required"
	TransportMCPNoSubscriber   = "mcp_no_subscriber"
)

// WakePeerOptions configures peer wake + optional delivery_receipt stamp.
type WakePeerOptions struct {
	ProjectRoot string
	Message     string
	InReplyTo   string // parent event_id for delivery_receipt
	FromAgentID string // agent stamping the receipt (usually sender)
	// DeliveryMode selects wake transport. Empty → read lite config; still empty → notify
	// (ship default). notify → adapter notify path; paste → adapter paste path.
	DeliveryMode string
	// ToAgentID selects a seat from the mesh peer_seats map when multiple peers run.
	ToAgentID string
	// PeerPID overrides seat-map pid (tests / explicit CLI).
	PeerPID int
	// Adapter overrides the process-wide PeerWakeAdapter when non-nil (tests / composition).
	Adapter PeerWakeAdapter
	// MCPIPCDelivered is true when ActionRequired was published to the MCP daemon
	// (coordinator / TPM interrupt path). Used with MCPSubscriberCount for CRIT-COMMS-003.
	MCPIPCDelivered bool
	// MCPSubscribersProbed is true when MCPSubscriberCount was obtained from the daemon
	// (false → leave shell-adapter Live/Transport unchanged).
	MCPSubscribersProbed bool
	// MCPSubscriberCount is events/subscribe count from the daemon (meaningful only when probed).
	MCPSubscriberCount int
}

// ShouldWakePeer reports whether lite delivery_mode requires a peer wake transport.
func ShouldWakePeer(deliveryMode string) bool {
	switch strings.TrimSpace(deliveryMode) {
	case datacell.DeliveryModeNotify, datacell.DeliveryModePaste:
		return true
	default:
		return false
	}
}

// WakePeer runs the configured peer-wake adapter so an idle peer receives the message.
// Best-effort: missing adapter endpoint or wake failure is returned in PeerWakeResult, not
// as a hard error (feed append already succeeded).
func WakePeer(ctx context.Context, projectRoot, message string) PeerWakeResult {
	return WakePeerOpts(ctx, WakePeerOptions{ProjectRoot: projectRoot, Message: message})
}

// resolveWakeDeliveryMode returns opts.DeliveryMode, else lite-file mode, else notify.
func resolveWakeDeliveryMode(root, explicit string) string {
	if m := strings.TrimSpace(explicit); m != "" {
		return m
	}
	cfg, err := datacell.ReadAgentChatChannelConfig(root)
	if err == nil {
		if m := strings.TrimSpace(cfg.DeliveryMode); m != "" {
			return m
		}
	}
	return datacell.DeliveryModeNotify
}

// IsTPMWakeSeat reports whether toAgentID matches legacy coordinator agent-id tokens.
// Deprecated for product routing — use ResolveWakeMembrane / peer_seats.wake.
func IsTPMWakeSeat(toAgentID string) bool {
	return legacyCoordinatorAgentID(toAgentID)
}

func legacyCoordinatorAgentID(toAgentID string) bool {
	a := strings.ToLower(strings.TrimSpace(toAgentID))
	if a == "" {
		return false
	}
	switch a {
	case "peer-tpm-02", "peer-tpm-01", "tpm", "tpm-seat":
		return true
	}
	if strings.Contains(a, "ide") && (strings.Contains(a, "tpm") || strings.Contains(a, "composer")) {
		return true
	}
	return strings.HasPrefix(a, "tpm")
}

const (
	FromAgentID          = "primary"
	skWakeAuthorExcluded = "author_excluded"
)

var SessionEnvKey = zqkenv.Session().Name()

// SeatWorkerBound returns whether the current process has an active ZQK_SESSION binding.
func SeatWorkerBound() (bool, string) {
	val := strings.TrimSpace(zqkenv.Session().Get())
	return val != "", val
}

// resolveFromAgentID resolves the author agent ID using options, session env, or fallback.
func resolveFromAgentID(opts WakePeerOptions) string {
	if f := strings.TrimSpace(opts.FromAgentID); f != "" {
		return f
	}
	if bound, session := SeatWorkerBound(); bound {
		return session
	}
	return FromAgentID
}

// WakePeerOpts wakes the peer via PeerWakeAdapter and, on success with InReplyTo set,
// appends delivery_receipt. Destination seat kind (worker vs coordinator) is resolved
// from ToAgentID; the adapter chooses the concrete transport.
func WakePeerOpts(ctx context.Context, opts WakePeerOptions) PeerWakeResult {
	from := resolveFromAgentID(opts)
	if to := strings.TrimSpace(opts.ToAgentID); to != "" && to == from {
		return PeerWakeResult{
			Skipped:            skWakeAuthorExcluded,
			SkippedReason:      "self_wake",
			WakeAuthorExcluded: true,
		}
	}

	msg := strings.TrimSpace(opts.Message)
	if msg == "" {
		return PeerWakeResult{Skipped: "empty_message"}
	}
	root := strings.TrimSpace(opts.ProjectRoot)
	if root == "" {
		return PeerWakeResult{Skipped: "empty_project_root"}
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
	}
	mode := resolveWakeDeliveryMode(root, opts.DeliveryMode)
	membrane := ResolveWakeMembrane(root, opts.ToAgentID)
	seatKind := SeatKindForWakeMembrane(membrane)
	paste := ResolveWakePasteTextIn(root, seatKind, opts.InReplyTo, msg, opts.ToAgentID)
	pid := opts.PeerPID
	var conversation string
	if pid <= 0 {
		if p, err := ResolvePeerPID(root, opts.ToAgentID); err == nil && p > 0 {
			pid = p
		}
	}
	if to := strings.TrimSpace(opts.ToAgentID); to != "" {
		if seats, err := LoadPeerSeats(root); err == nil {
			if rec, ok := seats.Seats[to]; ok {
				conversation = strings.TrimSpace(rec.Conversation)
			}
		}
	}

	adapter := opts.Adapter
	if adapter == nil {
		adapter = CurrentPeerWakeAdapter()
	}
	ares, err := adapter.Wake(ctx, PeerWakeRequest{
		ProjectRoot:  root,
		ToAgentID:    opts.ToAgentID,
		SeatKind:     seatKind,
		DeliveryMode: mode,
		PasteText:    paste,
		PeerPID:      pid,
		Conversation: conversation,
		Message:      msg,
	})
	res := PeerWakeResult{
		Attempted: true,
		Script:    ares.Endpoint,
		InReplyTo: strings.TrimSpace(opts.InReplyTo),
		PasteText: paste,
		Transport: ares.Transport,
		Live:      ares.Live,
	}
	if err != nil {
		if strings.Contains(err.Error(), "wake membrane missing") {
			res.Attempted = false
			res.Skipped = "wake_script_missing"
			res.Error = ""
		} else {
			res.Error = err.Error()
		}
		// MCP ActionRequired can still salvage a live wake when the shell membrane fails or is missing.
		ApplyMCPLiveInterrupt(&res, membrane, mode, opts.MCPIPCDelivered, opts.MCPSubscriberCount, opts.MCPSubscribersProbed)
		if res.Live || res.Transport == TransportMCPNoSubscriber {
			res.Skipped = ""
			res.Error = ""
			res.Attempted = true
		} else {
			return res
		}
	} else {
		ApplyMCPLiveInterrupt(&res, membrane, mode, opts.MCPIPCDelivered, opts.MCPSubscriberCount, opts.MCPSubscribersProbed)
	}

	if res.Live && res.Transport == TransportMCPActionRequired {
		// Queue zqk.wake.attn so the IDE bridge injects into the existing
		// Composer (toast/ActionRequired never starts a Cursor turn).
		if idebridge.QueueWakeAttn(root, msg) {
			res.IdeBridgeQueued = true
		}
	} else if res.Live && (res.Transport == TransportAgentAPINotify || res.Transport == TransportAGYNotify) {
		// Human-facing proof-of-life pulse (status bar + toast) — not chat inject.
		if idebridge.QueueProofOfLife(root, msg) {
			res.IdeBridgeQueued = true
		}
	}

	// Stamp delivery_receipt in-process (avoid double-stamp from shell when MESH_IN_REPLY_TO is set).
	if ir := strings.TrimSpace(opts.InReplyTo); ir != "" {
		if !res.Live {
			// Fail-closed when delivery_receipt without seat interrupt; ambient metric
			res.Error = "FAIL-CLOSED: delivery_receipt requires a live seat interrupt (live=false)"
			return res
		}

		from := strings.TrimSpace(opts.FromAgentID)
		if from == "" {
			from = "wake"
		}
		summary := ""
		if res.Transport == TransportMCPActionRequired {
			summary = "DELIVERY_RECEIPT — MCP ActionRequired delivered to IDE subscriber(s) for " + ir
		}
		dr, derr := AppendDeliveryReceipt(root, from, ir, summary)
		if derr == nil {
			res.DeliveryReceipt = true
			res.DeliveryEventID = dr.EventID
		}
	}
	return res
}

// ApplyMCPLiveInterrupt upgrades a wake when the destination seat's membrane is MCP
// and ActionRequired was published to ≥1 IDE events subscriber.
// That is transport, not a Cursor Composer turn — the IDE bridge must still
// inject via zqk.wake.attn.
// Any seat may declare wake=mcp in peer_seats — not limited to IDE/TPM agent ids.
// When probed is false, the shell-adapter result is left unchanged.
func ApplyMCPLiveInterrupt(res *PeerWakeResult, membrane, deliveryMode string, ipcOK bool, subscribers int, probed bool) {
	if res == nil || !probed {
		return
	}
	if NormalizeWakeMembrane(membrane) != WakeMembraneMCP {
		return
	}
	if strings.TrimSpace(deliveryMode) != datacell.DeliveryModeNotify {
		return
	}
	if ipcOK && subscribers > 0 {
		res.Live = true
		res.Transport = TransportMCPActionRequired
		res.Attempted = true
		return
	}
	if subscribers == 0 {
		res.Live = false
		res.Transport = TransportMCPNoSubscriber
		res.Attempted = true
	}
}

// ApplyCoordinatorMCPInterrupt is a deprecated alias for ApplyMCPLiveInterrupt
// that maps seatKind coordinator → mcp membrane.
// remove callers; use ApplyMCPLiveInterrupt.
func ApplyCoordinatorMCPInterrupt(res *PeerWakeResult, seatKind, deliveryMode string, ipcOK bool, subscribers int, probed bool) {
	membrane := WakeMembraneAgentAPI
	if seatKind == SeatKindCoordinator {
		membrane = WakeMembraneMCP
	}
	ApplyMCPLiveInterrupt(res, membrane, deliveryMode, ipcOK, subscribers, probed)
}
