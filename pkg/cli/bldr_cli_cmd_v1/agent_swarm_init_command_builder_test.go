package bldr_cli_cmd_v1

import "testing"

func TestNewAgentSwarmInitCommandBuilder(t *testing.T) {
	t.Parallel()

	cmd := NewAgentSwarmInitCommandBuilder()
	if cmd.Use != "swarm-init" {
		t.Fatalf("Use = %q, want 'swarm-init'", cmd.Use)
	}
	for _, flagName := range []string{"pipeline", "workflow", "from-stage", "plan-id", "agent-id", "persona-ref", "dry-run", "allow-chat"} {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("missing %s flag", flagName)
		}
	}
}
