package agentfeed

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// EnforceDirectedHourglass rejects directed peer routing without an await-peer-ack
// registration request. POL-AGENT-ORCH-HOURGLASS-001: fire-and-forget directed
// steers/wakes leave both seats idle when delivery/ack fails.
//
// Untargeted (empty to_agent_id) broadcasts are allowed without await.
// No env privilege override — TRACK: BLI-ENV-BREAKGLASS-REMOVE-001 /
// DEC-ENV-BREAKGLASS-TO-SIGNED-LOGIN-001.
func EnforceDirectedHourglass(toAgentID string, awaitPeerAck bool) error {
	if strings.TrimSpace(toAgentID) == "" {
		return nil
	}
	if awaitPeerAck {
		return nil
	}
	return errfmt.Errorf(
		"POL-AGENT-ORCH-HOURGLASS-001: directed --to-agent-id requires --await-peer-ack (never fire-and-forget; no env override)",
	)
}
