package supply

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSecretScan verifies TST-CEF-R2-SUP-SECRET-SCAN by executing scripts/scan-secrets.sh
// and verifying both operational pass on clean directories and fail-closed rejection on leaked secrets.
func TestSecretScan(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join("..", "..", "scripts", "scan-secrets.sh")
	if !fileutil.Exists(scriptPath) {
		t.Skip("scripts/scan-secrets.sh not found, skipping script execution test")
	}

	// 1. Clean directory passes secret scan
	cleanDir := t.TempDir()
	cleanFile := filepath.Join(cleanDir, "clean.go")
	err := fileutil.WriteFile(cleanFile, []byte("package main\n\nfunc main() {}\n"), paths.FilePerm644)
	require.NoError(t, err)

	cmd := testkit.ManagedCommand(t, t.Context(), "sh", scriptPath, cleanDir)
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, "scan-secrets.sh failed on clean directory: %s", string(out))

	// 2. Directory with leaked token fails closed with non-zero exit code
	dirtyDir := t.TempDir()
	dirtyFile := filepath.Join(dirtyDir, "config.go")
	dummyToken := "git" + "hub_pat_11AAAAAAA0123456789012345678901234567890123456789012345678901234567890123456789012"
	err = fileutil.WriteFile(dirtyFile, []byte("const ApiKey = \""+dummyToken+"\"\n"), paths.FilePerm644)
	require.NoError(t, err)

	failCmd := testkit.ManagedCommand(t, t.Context(), "sh", scriptPath, dirtyDir)
	failOut, failErr := failCmd.CombinedOutput()
	assert.Error(t, failErr, "scan-secrets.sh unexpectedly passed on directory with leaked token")
	assert.Contains(t, string(failOut), "Potential secrets detected", "expected secret detection warning in output")
}
