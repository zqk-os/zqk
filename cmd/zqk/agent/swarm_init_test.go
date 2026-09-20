package agent

import (
	"testing"
)

func TestAgentCmd_SwarmInitSubcommand(t *testing.T) {
	cmd := NewAgentCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "swarm-init" {
			found = true
			if c.Use != "swarm-init" {
				t.Errorf("expected Use 'swarm-init', got %q", c.Use)
			}
			if c.Flags().Lookup("pipeline") == nil {
				t.Errorf("missing --pipeline flag on agent swarm-init")
			}
			if c.Flags().Lookup("workflow") == nil {
				t.Errorf("missing --workflow flag on agent swarm-init")
			}
			if c.Flags().Lookup("dry-run") == nil {
				t.Errorf("missing --dry-run flag on agent swarm-init")
			}
			break
		}
	}
	if !found {
		t.Fatal("expected 'swarm-init' subcommand under 'agent' command")
	}
}
