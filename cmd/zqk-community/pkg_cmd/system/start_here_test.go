package system

import (
	"bytes"
	"strings"
	"testing"
)

func TestStartHereCmd(t *testing.T) {
	cmd := NewStartHereCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected non-empty output")
	}
	if !strings.Contains(output, "Zero-Config MCP") {
		t.Errorf("expected output to contain 'Zero-Config MCP', got: %s", output)
	}
	if !strings.Contains(output, "workflow whats-next") {
		t.Errorf("expected output to contain 'workflow whats-next', got: %s", output)
	}
}
