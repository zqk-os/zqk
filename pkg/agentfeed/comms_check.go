package agentfeed

import (
	"fmt"
	"strings"
)

// COMMS protocol tokens (POL-AGENT-COMMS-CHECK-001).
const (
	CommsCheckPrefix      = "COMMS-CHECK"
	CommsCheckReplyPrefix = "COMMS-CHECK-REPLY"
	CommsNoncePrefix      = "CC-"
	CommsLifeMarker       = "LIFE"
	CommsWorkMarker       = "WORK"
)

// CommsCheckChallenge is a parsed directed COMMS-CHECK challenge.
type CommsCheckChallenge struct {
	Nonce     string
	EventID   string
	FromSeat  string
	Message   string
	RawFields []string
}

// ParseCommsCheckChallenge extracts a COMMS-CHECK + CC-* nonce from a feed body.
// Returns ok=false when the text is not a COMMS challenge.
func ParseCommsCheckChallenge(message string) (CommsCheckChallenge, bool) {
	fields := strings.Fields(strings.TrimSpace(message))
	if len(fields) < 2 || fields[0] != CommsCheckPrefix {
		return CommsCheckChallenge{}, false
	}
	nonce := strings.TrimRight(fields[1], ":,;")
	if !strings.HasPrefix(nonce, CommsNoncePrefix) {
		return CommsCheckChallenge{}, false
	}
	return CommsCheckChallenge{
		Nonce:     nonce,
		Message:   strings.TrimSpace(message),
		RawFields: fields,
	}, true
}

// CommsLifeAckSummary is the peer_ack summary that proves life for a nonce.
func CommsLifeAckSummary(nonce string) string {
	return fmt.Sprintf("%s %s %s", CommsCheckPrefix, strings.TrimSpace(nonce), CommsLifeMarker)
}

// FormatCommsWorkReply builds the WORK steer body for a COMMS-CHECK-REPLY.
func FormatCommsWorkReply(nonce, priorityID string, inbox int) string {
	pri := strings.TrimSpace(priorityID)
	if pri == "" {
		pri = "none"
	}
	return fmt.Sprintf("%s %s %s priority=%s inbox=%d",
		CommsCheckReplyPrefix, strings.TrimSpace(nonce), CommsWorkMarker, pri, inbox)
}

// IsCommsCheckText reports whether s looks like a COMMS-CHECK challenge (doorbell or full body).
func IsCommsCheckText(s string) bool {
	_, ok := ParseCommsCheckChallenge(s)
	if ok {
		return true
	}
	// Wake stub form: "ATTN PEER — COMMS-CHECK CC-… — …"
	return strings.Contains(s, CommsCheckPrefix+" "+CommsNoncePrefix) ||
		strings.Contains(s, "— "+CommsCheckPrefix+" ")
}
