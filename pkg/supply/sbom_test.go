package supply

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSBOM verifies TST-CEF-R2-SUP-SBOM by verifying workflow configuration and OpenVEX attestation generation.
func TestSBOM(t *testing.T) {
	t.Parallel()

	// 1. Verify .github/workflows/sbom.yml configuration
	sbomPath := filepath.Join("..", "..", ".github", "workflows", "sbom.yml")
	if fileutil.Exists(sbomPath) {
		content, err := fileutil.ReadFile(sbomPath)
		require.NoError(t, err)
		sContent := string(content)
		assert.Contains(t, sContent, "anchore/sbom-action")
		assert.Contains(t, sContent, "spdx-json")
		assert.Contains(t, sContent, "sbom.spdx.json")
	}

	// 2. Verify OpenVEX generator script produces a spec-compliant document
	scriptPath := filepath.Join("..", "..", "scripts", "generate-openvex.sh")
	if !fileutil.Exists(scriptPath) {
		t.Skip("generate-openvex.sh not found, skipping script integration test")
	}

	tmpDir := t.TempDir()
	vexFile := filepath.Join(tmpDir, "openvex.json")

	cmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "v0.1.0-test", vexFile)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "generate-openvex.sh execution failed: %s", string(out))

	vexBytes, err := os.ReadFile(vexFile)
	require.NoError(t, err)

	v := NewVerifier()
	assert.NoError(t, v.VerifyOpenVEXBytes(vexBytes), "generated OpenVEX document failed verifier validation")

	// 3. Verify generate-openvex.sh --verify confirms validity
	verifyCmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--verify", vexFile)
	verifyOut, err := verifyCmd.CombinedOutput()
	assert.NoError(t, err, "--verify failed on generated OpenVEX document: %s", string(verifyOut))
}
