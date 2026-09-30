package ideadapter

import "time"

// Ping timeout and fail threshold: a single slow ping under system-check load
// must not closeConn+reconnect (subscriber-count storms).
const (
	heartbeatPingTimeout    = 8 * time.Second
	heartbeatFailBeforeDrop = 3
)

func heartbeatDropAfterConsecutiveFails(consecutive int) bool {
	return consecutive >= heartbeatFailBeforeDrop
}
