package graph

import (
	"bytes"
	"strings"
	"testing"
)

func TestGraphExportCommand_Structure(t *testing.T) {
	cmd := NewExportCmd()
	if cmd.Use != "export" {
		t.Fatalf("expected Use 'export', got %q", cmd.Use)
	}

	formatFlag := cmd.Flag("format")
	if formatFlag == nil {
		t.Fatal("missing --format flag")
	}

	kindFlag := cmd.Flag("kind")
	if kindFlag == nil {
		t.Fatal("missing --kind flag")
	}

	outputFlag := cmd.Flag("output")
	if outputFlag == nil {
		t.Fatal("missing --output flag")
	}

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help execution failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "W3C Holon DataBook") {
		t.Errorf("expected help to mention W3C Holon DataBook, got:\n%s", output)
	}
}
