package scheduler

import "fmt"

// loopbackListenAddr returns a host:port that binds only the loopback interface.
// TRACK: BLI-CEF-SEC-NETWORK-BINDS — REQ-CEF-SEC-001 / CRIT-CEF-SEC-001A.
func loopbackListenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
