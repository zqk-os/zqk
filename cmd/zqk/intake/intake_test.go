package intake

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestIntakeCommand_NoArgs(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tempDir, nil))
		_ = fileutil.RemoveAll(filepath.Join(tempDir, ".zqk"))
	})
	_, err := testenvroot.Setup(tempDir)
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	cmd := NewIntakeCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = cmd.Execute()
	if err == nil {
		t.Fatal("Expected error when no arguments or stdin is provided, got nil")
	}

	if !strings.Contains(err.Error(), "no intent context provided") {
		t.Fatalf("Expected 'no intent context provided' error, got %v", err)
	}
}
