package system

import (
	"bytes"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestQuickstartCommand(t *testing.T) {
	cmd := NewQuickstartCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf))
	cmd.SetArgs([]string{"--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("quickstart command failed: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("ZQK Quickstart Guide")) {
		t.Errorf("expected json output to contain quickstart title, got: %s", buf.String())
	}

	if !bytes.Contains(buf.Bytes(), []byte("--with-onboarding-roadmap")) {
		t.Errorf("expected json output to include --with-onboarding-roadmap in system init step, got: %s", buf.String())
	}
}

// BLI-REDACTED: Deprecate 'quick' CLI command family in favor of 'new' / 'quickstart'
func TestQuickstartCommand_BLI_1788987316012200000_5097aded_DeprecateQuick(t *testing.T) {
	cmd := NewQuickstartCmd()
	if cmd.Use != "quickstart" {
		t.Errorf("expected quickstart command Use='quickstart', got %q", cmd.Use)
	}
	if len(cmd.Aliases) == 0 {
		t.Errorf("expected quickstart aliases to be defined")
	}
}

