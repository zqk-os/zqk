package system

import (
	"testing"
)

func TestFirstRunSystemCmdDropsCodegen(t *testing.T) {
	t.Parallel()
	cmd := NewSystemCmd()
	got := map[string]struct{}{}
	for _, sub := range cmd.Commands() {
		got[sub.Name()] = struct{}{}
	}
	for _, name := range []string{"spec-origination", "update-specs", "federate", "snapshot", "align", "cli-hooks", "validate-command-specs"} {
		if _, ok := got[name]; ok {
			t.Fatalf("open-core system still ships %q", name)
		}
	}
	for _, name := range []string{"init", "dashboard", "check", "status", "whoami", "validate"} {
		if _, ok := got[name]; !ok {
			t.Fatalf("open-core system missing first-run %q", name)
		}
	}
}
