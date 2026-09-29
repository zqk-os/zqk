package agentfeed

import (
	"fmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"os"
	"strings"
)

// Chat paste is an **opt-in** wake/ack ring only (delivery_mode=paste / --chat) — full steer
// bodies stay on the agent feed. Ship default is notify (--notify-only / MCP ActionRequired).
// Escape hatch: MESH_WAKE_PASTE_FULL=1 pastes the raw message when paste mode is already on.
// TRACK closed (default): core-backlog Phase A / core-backlog.

const (
	// Paste roles mirror the seat kinds in wake_adapter.go rather than vendor names: which vendor
	// occupies a worker or coordinator seat can change without this code changing.
	WakePasteRoleWorker      = SeatKindWorker
	WakePasteRoleCoordinator = SeatKindCoordinator

	envWakePasteFull = "MESH_WAKE_PASTE_FULL"

	// Attention prefixes for the chat ring, addressed by seat role (not vendor).
	attnPrefixPeer        = "ATTN PEER"
	attnPrefixCoordinator = "ATTN TPM"

	// Role placeholders when ToAgentID is empty and peer_seats.json has no matching
	// duty. Match scripts/mesh/peer_seats.example.json — not a vendor product.
	// TRACK: TDE-MESH-DEFAULT-SEAT-PEER-AGENT-01-001 — prefer live ToAgentID, then seating config.
	FallbackPasteAgentWorker      = "peer-agent-1"
	FallbackPasteAgentCoordinator = "peer-operator-1"
)

// ResolvePasteAgentID returns the seat id for a whats-next pointer.
// Preference: explicit agentID → peer_seats duty/wake for the role → example placeholders.
// Vendor names appear only when seating config (or the caller) supplies them.
func ResolvePasteAgentID(projectRoot, role, agentID string) string {
	if aid := strings.TrimSpace(agentID); aid != "" {
		return aid
	}
	coordinator := strings.EqualFold(strings.TrimSpace(role), WakePasteRoleCoordinator)
	if root := strings.TrimSpace(projectRoot); root != "" {
		if coordinator {
			if id := CoordinatorSeatID(root); id != "" {
				return id
			}
		} else if id := WorkerSeatID(root); id != "" {
			return id
		}
	}
	if coordinator {
		return FallbackPasteAgentCoordinator
	}
	return FallbackPasteAgentWorker
}

// wakePasteFormat renders "<prefix> — wake [<event>] — <whats-next pointer> (substance on feed)".
// The --agent-id pointer always carries the destination seat, never the event ID.
func wakePasteFormat(attnPrefix, eventID, agentID string) string {
	wake := "wake"
	if eventID != "" {
		wake += " " + eventID
	}
	return paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("%s — %s — zqk workflow whats-next --format json --skip-measure --agent-id %s (substance on feed)", attnPrefix, wake, agentID))
}

// WakePasteStub builds a short chat-paste line (wake/ack). Substance lives on the feed.
// agentID should be the destination seat (e.g. peer-agent-2); empty falls back to
// seating config when projectRoot is known, else the example role placeholders.
func WakePasteStub(role, eventID, agentID string) string {
	return WakePasteStubIn("", role, eventID, agentID)
}

// WakePasteStubIn is WakePasteStub with a project root so peer_seats.json can supply seat ids.
func WakePasteStubIn(projectRoot, role, eventID, agentID string) string {
	eid := strings.TrimSpace(eventID)
	aid := ResolvePasteAgentID(projectRoot, role, agentID)
	if strings.EqualFold(strings.TrimSpace(role), WakePasteRoleCoordinator) {
		return wakePasteFormat(attnPrefixCoordinator, eid, aid)
	}
	return wakePasteFormat(attnPrefixPeer, eid, aid)
}

// commsCheckWakeStub preserves COMMS protocol metadata in the short agentapi
// notification. The challenge body remains on the feed, but the doorbell must
// carry the nonce and required response shape so a generic auto-ack cannot make
// the challenge disappear from the seat's unacked inbox before it acts.
func commsCheckWakeStub(projectRoot, role, eventID, agentID, fullMessage string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(fullMessage))
	if len(fields) < 2 || fields[0] != "COMMS-CHECK" || !strings.HasPrefix(fields[1], "CC-") {
		return "", false
	}
	prefix := attnPrefixPeer
	if strings.EqualFold(strings.TrimSpace(role), WakePasteRoleCoordinator) {
		prefix = attnPrefixCoordinator
	}
	eid := strings.TrimSpace(eventID)
	aid := ResolvePasteAgentID(projectRoot, role, agentID)
	coordinatorID := ResolvePasteAgentID(projectRoot, WakePasteRoleCoordinator, "")
	return fmt.Sprintf(
		"%s — COMMS-CHECK %s — wake %s — run whats-next --agent-id %s; feed ack nonce; feed steer WORK priority+inbox to %s",
		prefix, fields[1], eid, aid, coordinatorID,
	), true
}

// PeerAckPasteStub is the chat ring when a peer_ack completes an await.
func PeerAckPasteStub(eventID string) string {
	eid := strings.TrimSpace(eventID)
	if eid == "" {
		return attnPrefixCoordinator + " — peer_ack received — check feed / whats-next (substance on feed)"
	}
	return attnPrefixCoordinator + " — peer_ack received for " + eid
}

// ResolveWakePasteText returns stub paste unless MESH_WAKE_PASTE_FULL=1.
// agentID is the wake destination seat for the whats-next --agent-id pointer.
func ResolveWakePasteText(role, eventID, fullMessage, agentID string) string {
	return ResolveWakePasteTextIn("", role, eventID, fullMessage, agentID)
}

// ResolveWakePasteTextIn is ResolveWakePasteText with a project root for seating lookup.
func ResolveWakePasteTextIn(projectRoot, role, eventID, fullMessage, agentID string) string {
	if strings.TrimSpace(os.Getenv(envWakePasteFull)) == "1" {
		msg := strings.TrimSpace(fullMessage)
		if msg != "" {
			return msg
		}
	}
	if stub, ok := commsCheckWakeStub(projectRoot, role, eventID, agentID, fullMessage); ok {
		return stub
	}
	return WakePasteStubIn(projectRoot, role, eventID, agentID)
}
