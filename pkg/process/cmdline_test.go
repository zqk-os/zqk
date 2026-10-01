package process

import (
	"os"
	"testing"
)

func TestProcessCommandLine(t *testing.T) {
	cmdLine := ProcessCommandLine(os.Getpid())
	if cmdLine == "" {
		// On windows this returns empty string; on linux/darwin it should be non-empty
		t.Logf("ProcessCommandLine returned empty for current PID")
	} else {
		t.Logf("ProcessCommandLine returned: %s", cmdLine)
	}

	// Invalid PID
	if invalid := ProcessCommandLine(-1); invalid != "" {
		t.Fatalf("expected empty string for negative PID, got: %s", invalid)
	}
}
