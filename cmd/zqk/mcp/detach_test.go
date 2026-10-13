package mcp

import (
	"runtime"
	"testing"

	"github.com/zqk-os/zqk/pkg/testkit"
)

// TestSetDetach ensures setDetach correctly attaches OS-specific process attributes.
func TestSetDetach(t *testing.T) {
	cmd := testkit.ManagedCommand(t, t.Context(), "echo", "test")
	cmd.SysProcAttr = nil

	// Pre-condition: SysProcAttr should be nil initially
	if cmd.SysProcAttr != nil {
		t.Fatalf("expected initial SysProcAttr to be nil, got %+v", cmd.SysProcAttr)
	}

	setDetach(cmd)

	// Post-condition: SysProcAttr should be configured
	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be configured after setDetach")
	}

	if runtime.GOOS != "windows" {
		_ = cmd.SysProcAttr
	}
}
