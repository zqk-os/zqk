package idebridge

import "strings"

// ProofOfLifePrefix marks human-facing mesh pulses on the IDE bridge control bus.
// Chat may stay quiet; this is the front-channel indicator that back-channel work continues.
const ProofOfLifePrefix = "PROOF-OF-LIFE"

const defaultProofOfLifeDetail = "mesh back-channel active (chat may be quiet — work continues on the feed)"

// FormatProofOfLifeMessage builds a stable toast/status-bar string for the IDE bridge.
func FormatProofOfLifeMessage(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = defaultProofOfLifeDetail
	}
	upper := strings.ToUpper(detail)
	if strings.HasPrefix(upper, ProofOfLifePrefix) {
		return truncateRunes(detail, maxWakeAttnRunes)
	}
	return truncateRunes(ProofOfLifePrefix+": "+detail, maxWakeAttnRunes)
}

// QueueProofOfLife appends zqk.wake.attn with a PROOF-OF-LIFE message (best-effort).
// Opt out with ZQK_IDE_BRIDGE_WAKE=0. Does not fail the caller path.
func QueueProofOfLife(projectRoot, detail string) bool {
	return QueueWakeAttn(projectRoot, FormatProofOfLifeMessage(detail))
}
