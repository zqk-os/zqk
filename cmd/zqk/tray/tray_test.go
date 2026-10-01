package tray

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestTrayRun_DryRunPassThrough(t *testing.T) {
	cmd := NewTrayCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(t.TempDir()))
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	cmd.SetArgs([]string{"run", "intake", "--dry-run", "test extra arg"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("tray run dry-run failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "test extra arg") {
		t.Fatalf("expected output to contain passed argument, got:\n%s", out)
	}
	if !strings.Contains(out, "dry_run: true") {
		t.Fatalf("expected dry_run output, got:\n%s", out)
	}
}
