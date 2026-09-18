package system

import (
	"bytes"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestStartHereCmd(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := NewStartHereCmd()
	cmd.SetContext(pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), buf))
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected non-empty output")
	}
	if !strings.Contains(output, "Zero-Config MCP") {
		t.Errorf("expected output to contain 'Zero-Config MCP', got: %s", output)
	}
	if !strings.Contains(output, "agent-onboard") {
		t.Errorf("expected output to contain 'agent-onboard', got: %s", output)
	}
	if !strings.Contains(output, "EDGE_HEADLESS_FIRST_RUN") {
		t.Errorf("expected output to contain edge headless guide, got: %s", output)
	}
	if !strings.Contains(output, "workflow whats-next") {
		t.Errorf("expected output to contain 'workflow whats-next', got: %s", output)
	}
	if !strings.Contains(output, "object list mission") {
		t.Errorf("expected starter graph list commands, got: %s", output)
	}
	if !strings.Contains(output, "starter_kernel_graph") {
		t.Errorf("expected seed.sh pointer, got: %s", output)
	}
}
