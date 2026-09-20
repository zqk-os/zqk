package bldr_cli_cmd_v1

import "testing"

func TestNewNewCommandSpecCommandBuilder(t *testing.T) {
	t.Parallel()

	cmd := NewNewCommandSpecCommandBuilder()
	if cmd.Use != "command-spec <command-path>" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	for _, flagName := range []string{"use", "short", "description", "force"} {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("missing %s flag", flagName)
		}
	}
}
