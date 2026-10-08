package agent

import (
	"strings"
	"testing"
)

func TestNewEvaluateCmd(t *testing.T) {
	cmd := NewEvaluateCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil evaluate command")
	}
	if cmd.Flags().Lookup("session-id") == nil {
		t.Errorf("expected --session-id flag")
	}
}

func TestEvaluateCmd_MissingSessionID(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "evaluate-run")
	if err == nil || !strings.Contains(err.Error(), "requires --session-id") {
		t.Fatalf("expected requires --session-id error, got: %v (out: %s)", err, out)
	}
}

func TestEvaluateCmd_Success(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "evaluate-run", "--session-id", "SES-12345")
	if err != nil {
		t.Fatalf("unexpected error executing evaluate: %v", err)
	}

	if !strings.Contains(out, "Evaluated session SES-12345") {
		t.Errorf("expected output to contain session id, got: %s", out)
	}
}
