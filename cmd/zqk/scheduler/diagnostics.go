package scheduler

import (
	"github.com/zqk-os/zqk/pkg/diagnostics"
)

// captureDiagnostics captures goroutine dump, heap profile, and process information
// Delegates to the shared diagnostics package
func captureDiagnostics(outputDir, prefix string) error {
	return diagnostics.CaptureDiagnostics(outputDir, prefix)
}
