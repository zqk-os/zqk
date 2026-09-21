package object

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestRunInteractiveWizard(t *testing.T) {
	testEnv := SetupTestEnvironment(t)
	tempDir := t.TempDir()
	editorPath := filepath.Join(tempDir, "mock_editor.sh")
	editorScript := `#!/bin/sh
sed -i.bak 's/title:.*/title: Mocked Title/' "$1"
sed -i.bak 's/description:.*/description: Mocked Description/' "$1"
`
	if err := fileutil.WriteFile(editorPath, []byte(editorScript), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create mock editor: %v", err)
	}

	t.Setenv(zqkenv.OSEditor().Name(), editorPath)

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
