package ops

import (
	"testing"
)

func TestNewLevelUpCmd(t *testing.T) {
	cmd := NewLevelUpCmd()
	if cmd.Use != "level-up [agent_skill_id]" {
		t.Errorf("Expected Use 'level-up [agent_skill_id]', got '%s'", cmd.Use)
	}
}
