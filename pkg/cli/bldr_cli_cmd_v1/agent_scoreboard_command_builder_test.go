package bldr_cli_cmd_v1

import (
	"testing"
)

func TestAgentScoreboardCommandBuilder(t *testing.T) {
	cmd := NewAgentScoreboardCommandBuilder()
	if cmd == nil {
		t.Fatal("expected command to not be nil")
	}
	if cmd.Use != "scoreboard" {
		t.Errorf("expected Use to be 'scoreboard', got %s", cmd.Use)
	}
}
