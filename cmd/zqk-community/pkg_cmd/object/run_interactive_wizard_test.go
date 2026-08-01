package object

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

func TestRunInteractiveWizard(t *testing.T) {
	testEnv := SetupTestEnvironment(t)
	tempDir := t.TempDir()
	editorPath := filepath.Join(tempDir, "mock_editor.sh")
	editorScript := `#!/bin/sh
# $1 is the file
sed -i.bak 's/title:.*/title: Mocked Title/' "$1"
echo "description: Mocked Description" >> "$1"
`
	if err := os.WriteFile(editorPath, []byte(editorScript), 0755); err != nil {
		t.Fatalf("failed to create mock editor: %v", err)
	}

	oldEditor := os.Getenv("EDITOR")
	defer os.Setenv("EDITOR", oldEditor)
	os.Setenv("EDITOR", editorPath)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var inBuf, outBuf bytes.Buffer
	inBuf.WriteString("\n")
	cmd.SetIn(&inBuf)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)

	// Ensure processor resolves the isolated test project root from SetupTestEnvironment.
	_ = testEnv
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	objData := make(map[string]any)
	err = runInteractiveWizard(cmd, "goal", objData, proc)
	if err != nil {
		t.Fatalf("runInteractiveWizard failed: %v", err)
	}

	t.Logf("Output buffer:\n%s", outBuf.String())
	if objData[objects.FieldKeyTitle] != "Mocked Title" {
		t.Errorf("expected title 'Mocked Title', got %v", objData[objects.FieldKeyTitle])
	}
	if objData[objects.FieldKeyDescription] != "Mocked Description" {
		t.Errorf("expected description 'Mocked Description', got %v", objData[objects.FieldKeyDescription])
	}
}
