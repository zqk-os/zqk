package mcp

import (
	"os/exec"
	"runtime"
	"testing"
)

// TestSetDetach ensures setDetach correctly attaches OS-specific process attributes.
func TestSetDetach(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "echo", "test")

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
