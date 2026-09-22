package ambient

import (
	"os"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// HostDaemonEnabled reports whether the ambient host daemon should start for root.
// Go test binaries must not detach Setsid children that outlive the test process.
func HostDaemonEnabled(projectRoot string) bool {
	if strings.TrimSpace(projectRoot) == "" {
		return false
	}
	if testing.Testing() || zqkenv.IsInTest() {
		return false
	}
	if exe, err := os.Executable(); err == nil && zqkenv.IsGoTestBinary(exe) {
		return false
	}
	return true
}
