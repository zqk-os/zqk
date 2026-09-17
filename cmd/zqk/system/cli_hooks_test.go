package system

import (
	"strings"
	"testing"
)

func TestRequireCliHooksArgs(t *testing.T) {
	t.Parallel()
	if err := requireCliHooksArgs([]string{"get"}, 2, cliHooksUsageGet); err == nil {
		t.Fatal("expected usage error when args too short")
	} else if !strings.Contains(err.Error(), "usage: cli-hooks") || !strings.Contains(err.Error(), cliHooksUsageGet) {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := requireCliHooksArgs([]string{"get", "x"}, 2, cliHooksUsageGet); err != nil {
		t.Fatalf("expected nil: %v", err)
	}
	if err := requireCliHooksArgs([]string{"set-tray", "id"}, 3, cliHooksUsageSetTray); err == nil {
		t.Fatal("expected usage error for set-tray with 2 args")
	}
}
