package agent

import (
	"strings"
	"testing"
)

func TestNewPrepareContextCmd(t *testing.T) {
	cmd := NewPrepareContextCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil prepare context command")
	}
	if cmd.Flags().Lookup("persona-ref") == nil {
		t.Errorf("expected --persona-ref flag")
	}
	if cmd.Flags().Lookup("description") == nil {
		t.Errorf("expected --description flag")
	}
}

func TestPrepareContextCmd_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "prepare-context",
		"--persona-ref", "PER-TESTER",
		"--description", "Sample test task description",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("unexpected error executing prepare-context: %v (out: %s)", err, out)
	}

	if !strings.Contains(out, "prompt") {
		t.Errorf("expected output to contain prompt key, got: %s", out)
	}
}
