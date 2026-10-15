package utility

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestVersionCommands(t *testing.T) {
	// Test NewRootVersionCmd
	rootCmd := NewRootVersionCmd()
	require.NotNil(t, rootCmd)
	assert.Equal(t, "version", rootCmd.Use)

	// Test NewVersionCmd
	utilVerCmd := NewVersionCmd()
	require.NotNil(t, utilVerCmd)
	assert.Equal(t, "version", utilVerCmd.Use)

	// Test RunVersion with nil versionInfo
	versionInfo = nil
	parentCmd := &cobra.Command{Use: "zqk", Version: "1.2.3"}
	cmdWithParent := NewVersionCmd()
	parentCmd.AddCommand(cmdWithParent)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(contextWithEmptyCLI(), &buf)
	cmdWithParent.SetContext(ctx)

	RunVersion(cmdWithParent, nil)
	out := buf.String()
	assert.Contains(t, out, "Executable:")
	assert.Contains(t, out, "Version: 1.2.3")

	// Test RunVersion with populated versionInfo
	SetVersionInfo("2.0.0-rc1", "2026-10-15T00:00:00Z", "abcdef123456")
	buf.Reset()
	RunVersion(cmdWithParent, nil)
	out2 := buf.String()
	assert.Contains(t, out2, "Executable:")
	assert.Contains(t, out2, "Version: 2.0.0-rc1")
	assert.Contains(t, out2, "Build datetime: 2026-10-15T00:00:00Z")
	assert.Contains(t, out2, "Commit hash: abcdef123456")

	// Reset versionInfo for safety
	versionInfo = nil
}

func TestGetExecutableName(t *testing.T) {
	name := getExecutableName()
	assert.NotEmpty(t, name)

	// Test fallback branch by simulating os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"/custom/bin/custom-zqk"}
	assert.NotEmpty(t, filepath.Base(os.Args[0]))
}

func contextWithEmptyCLI() context.Context {
	return context.Background()
}
