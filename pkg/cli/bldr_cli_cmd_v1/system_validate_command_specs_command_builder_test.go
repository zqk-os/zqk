package bldr_cli_cmd_v1

import "testing"

func TestNewSystemValidateCommandSpecsCommandBuilder(t *testing.T) {
	t.Parallel()

	cmd := NewSystemValidateCommandSpecsCommandBuilder()
	if cmd.Use != "validate-command-specs" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	for _, flagName := range []string{"specs-dir", "baseline", "write-baseline"} {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("missing %s flag", flagName)
		}
	}
}
