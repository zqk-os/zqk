package ideadapter

import "time"

// Ping timeout and fail threshold: a single slow ping under system-check load
// must not closeConn+reconnect (subscriber-count storms). TRACK: BLI-CEF-R2-REL-MCP-RECONNECT
const (
	heartbeatPingTimeout    = 8 * time.Second
	heartbeatFailBeforeDrop = 3
)

func heartbeatDropAfterConsecutiveFails(consecutive int) bool {
	return consecutive >= heartbeatFailBeforeDrop
}
