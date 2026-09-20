package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

const seatWorkerLaneUnknown = "seat"

// seatWorkerEngineID is the swarm engine label. It uses the orch lane
// (alpha/beta/gamma), not the leftover opaque seat id. Those ids still look
// like a vendor (antigravity-*) even though the live workers are not that
// product. remove when: seating
// agent_id is itself an opaque lane token and no leftover vendor-shaped ids
// remain in launchd / peer_seats.
func seatWorkerEngineID(personaRef, agentID string) string {
	return fmt.Sprintf("seat-worker-%s-%d", seatWorkerLane(personaRef, agentID), time.Now().UnixNano())
}

func seatWorkerLane(personaRef, agentID string) string {
	switch strings.ToUpper(strings.TrimSpace(personaRef)) {
	case objects.ConstPersonaOrchestratorAlpha:
		return "alpha"
	case objects.ConstPersonaOrchestratorBeta:
		return "beta"
	case objects.ConstPersonaOrchestratorGamma:
		return "gamma"
	}
	if id := strings.TrimSpace(agentID); id != "" {
		sum := sha256.Sum256([]byte(id))
		return hex.EncodeToString(sum[:4])
	}
	return seatWorkerLaneUnknown
}
