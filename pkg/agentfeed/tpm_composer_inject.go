package agentfeed

import (
	"strings"
)

// Ack-theatre events must not start a Cursor TPM turn that says "ack then hourglass".
// Parked peer_ack / hourglass-callback mesh_status were the 2026-09-03 ping-pong:
// TPM hourglass → AGY peer_ack → watchdog inject → TPM hourglass again.
// TRACK: POL-AGENT-ORCH-HOURGLASS-001 — live ATK / COMMS-CHECK steers still hourglass.

const (
	tpmHourglassCue = "ack then hourglass"
	tpmAckOnlyCue   = "ack; do substance; do not hourglass (no live ATK / COMMS-CHECK)"
)

// IsAckTheatreInboxItem is true when injecting Composer with an hourglass cue
// would only produce another ack, not work.
func IsAckTheatreInboxItem(item CorrespondenceItem) bool {
	et := strings.TrimSpace(item.EventType)
	from := strings.TrimSpace(item.FromAgentID)
	body := strings.TrimSpace(item.Message)
	if body == "" {
		body = strings.TrimSpace(item.Summary)
	}
	upper := strings.ToUpper(body)

	switch et {
	case FeedEventTypePeerAck:
		return true
	case FeedEventTypeMeshStatus:
		if strings.Contains(upper, "PEER_ACK RECEIVED") || strings.Contains(upper, "PEER ACK RECEIVED") {
			return true
		}
		if from == "peer-ack-callback" || from == FeedSenderMeshStatus {
			return strings.Contains(upper, "PEER_ACK")
		}
	case FeedEventTypeWake:
		if from == "scheduler-callback" {
			return true
		}
		if strings.Contains(upper, "STAY PARKED") ||
			strings.Contains(upper, "SEAT_WORKER_SKIPPED") ||
			strings.HasPrefix(upper, "PEER_ACK — RECEIVED") {
			return true
		}
	}
	if from == "scheduler-callback" {
		return true
	}
	return false
}

// FirstComposerHourglassItem returns the first inbox event that should start a
// TPM turn with an hourglass cue. Ack-theatre items are skipped.
func FirstComposerHourglassItem(inbox []CorrespondenceItem) (CorrespondenceItem, bool) {
	for _, item := range inbox {
		if !IsAckTheatreInboxItem(item) {
			return item, true
		}
	}
	return CorrespondenceItem{}, false
}

// InboxRequiresHourglass is true when the body names a live ATK or a COMMS-CHECK nonce.
// Other TPM mail (job diagnostics, park, leftover SCH ids) is ack-and-work, not a peer wake.
func InboxRequiresHourglass(item CorrespondenceItem) bool {
	if IsAckTheatreInboxItem(item) {
		return false
	}
	body := strings.TrimSpace(item.Message)
	if body == "" {
		body = strings.TrimSpace(item.Summary)
	}
	if NamedATKID(body) != "" {
		return true
	}
	return IsCommsCheckText(body)
}

// TPMComposerAttnCue is the zqk.wake.attn line for a real TPM inbox steer.
func TPMComposerAttnCue(item CorrespondenceItem, tpmAgentID string) string {
	aid := strings.TrimSpace(tpmAgentID)
	if aid == "" {
		aid = "cursor-composer"
	}
	cue := tpmAckOnlyCue
	if InboxRequiresHourglass(item) {
		cue = tpmHourglassCue
	}
	return "ATTN TPM inbox: " + item.EventID + " from " + item.FromAgentID +
		" — whats-next --agent-id " + aid + "; " + cue
}
