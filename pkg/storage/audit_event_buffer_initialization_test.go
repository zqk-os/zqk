package storage

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TestInitializeGlobalBufferWithConfig_PreservesDisabledState verifies that when the global
// audit buffer has been disabled (e.g. by tests via SetEnabled(false)), calling
// InitializeGlobalBufferWithConfig to replace the buffer preserves disabled state so the new
// buffer is also disabled. This prevents coordinator integration tests from seeing the
// buffer re-enabled when CreateAuditEventWithBuilder triggers InitializeGlobalBufferWithConfig.
func TestInitializeGlobalBufferWithConfig_PreservesDisabledState(t *testing.T) {
	// Get global buffer and disable it (simulates test setup)
	buf := GetGlobalAuditEventBuffer()
	if buf == nil {
		t.Fatal("GetGlobalAuditEventBuffer() returned nil")
	}
	buf.SetEnabled(false)
	if buf.IsEnabled() {
		t.Fatal("SetEnabled(false) did not disable buffer")
	}

	// Use a distinct project root so InitializeGlobalBufferWithConfig replaces the buffer
	// (same projectRoot would cause early return and no replacement)
	projectRoot := filepath.Join(t.TempDir(), "preserve-disabled-test")
	secCtx := pkgctx.NewSystemSecurityContext()

	if err := InitializeGlobalBufferWithConfig(projectRoot, secCtx); err != nil {
		t.Fatalf("InitializeGlobalBufferWithConfig() error = %v", err)
	}

	// Replacement buffer must still be disabled
	globalAfter := GetGlobalAuditEventBuffer()
	if globalAfter == nil {
		t.Fatal("GetGlobalAuditEventBuffer() returned nil after init")
	}
	if globalAfter.IsEnabled() {
		t.Error("InitializeGlobalBufferWithConfig replaced buffer but re-enabled it; expected disabled state to be preserved")
	}
}
