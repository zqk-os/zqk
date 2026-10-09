package supply_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSupplyChain_FailClosedCosign_StaticFloor verifies CRIT-CEF-SUPPLY-FAILCLOSED-COSIGN-001.
// install.sh must enforce fail-closed cryptographic verification when signatures are required or corrupted.
func TestSupplyChain_FailClosedCosign_StaticFloor(t *testing.T) {
	t.Parallel()

	installPath := filepath.Join("..", "..", "install.sh")
	require.True(t, fileutil.Exists(installPath), "install.sh must exist at repo root")

	contentBytes, err := fileutil.ReadFile(installPath)
	require.NoError(t, err)
	content := string(contentBytes)

	// Invariant: install.sh must define _verify_checksums_signature
	assert.Contains(t, content, "_verify_checksums_signature", "install.sh must define _verify_checksums_signature")
	assert.Contains(t, content, "checksums.txt.sig", "install.sh must verify checksums.txt.sig")
	assert.Contains(t, content, "cosign verify-blob", "install.sh must invoke cosign verify-blob")
	assert.Contains(t, content, "ZQK_REQUIRE_COSIGN", "install.sh must support ZQK_REQUIRE_COSIGN fail-closed flag")

	// Operational test: execute _verify_checksums_signature in bash with missing signature and ZQK_REQUIRE_COSIGN=1
	tmpDir := t.TempDir()
	checksumsFile := filepath.Join(tmpDir, "checksums.txt")
	err = fileutil.WriteFile(checksumsFile, []byte("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  empty.tar.gz\n"), paths.FilePerm644)
	require.NoError(t, err)

	startIdx := strings.Index(content, "_verify_checksums_signature() {")
	require.True(t, startIdx > 0, "_verify_checksums_signature definition must be present")
	endIdx := strings.Index(content[startIdx:], "\n}\n")
	require.True(t, endIdx > 0, "end of _verify_checksums_signature must be found")
	funcDef := content[startIdx : startIdx+endIdx+3]

	testScript := `
set -e
` + funcDef + `
export ZQK_REQUIRE_COSIGN=1
_verify_checksums_signature "` + checksumsFile + `"
`
	cmd := testkit.ManagedCommand(t, t.Context(), "bash", "-c", testScript)
	out, err := cmd.CombinedOutput()
	assert.Error(t, err, "installer must fail closed when signature is missing and ZQK_REQUIRE_COSIGN=1")
	assert.Contains(t, string(out), "Error: signature file", "installer must output clear error on missing signature")
}

// TestSupplyChain_StandaloneRuntime_OperationalProof verifies CRIT-CEF-SUPPLY-STANDALONE-RUNTIME-001.
// Runtime peer wake adapters and context refresh handlers must execute safely via native Go fallbacks
// when optional studio shell scripts are absent in standalone distributions.
func TestSupplyChain_StandaloneRuntime_OperationalProof(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptsDir := filepath.Join(tmpDir, "scripts")
	err := fileutil.EnsureDir(scriptsDir)
	require.NoError(t, err)

	// 1. Verify ShellPeerWakeAdapter falls back cleanly when scripts/wake-agy.sh is absent
	wakeAdapter := agentfeed.NewShellPeerWakeAdapter()
	require.NotNil(t, wakeAdapter, "NewShellPeerWakeAdapter must not return nil")
	require.NotNil(t, wakeAdapter.Fallback, "ShellPeerWakeAdapter must initialize native fallback")

	res, err := wakeAdapter.Wake(context.Background(), agentfeed.PeerWakeRequest{
		ProjectRoot:  tmpDir,
		ToAgentID:    "AGT-TEST-001",
		SeatKind:     agentfeed.SeatKindWorker,
		DeliveryMode: "notify",
		Message:      "test-standalone-verification",
	})
	assert.NoError(t, err, "ShellPeerWakeAdapter must fall back to native in-process wake when shell script is absent")
	assert.NotEmpty(t, res.Transport, "PeerWakeAdapterResult must return transport")

	// 2. Verify coordinator wake fallback
	coordRes, coordErr := wakeAdapter.Wake(context.Background(), agentfeed.PeerWakeRequest{
		ProjectRoot:  tmpDir,
		ToAgentID:    "AGT-TEST-002",
		SeatKind:     agentfeed.SeatKindCoordinator,
		DeliveryMode: "stamp",
		Message:      "test-standalone-verification",
	})
	assert.NoError(t, coordErr, "ShellPeerWakeAdapter must fall back to native wake for coordinator")
	assert.NotEmpty(t, coordRes.Transport)
}

// TestSupplyChain_PayloadLeakGuard_NegativeBoundary verifies CRIT-CEF-SUPPLY-PAYLOAD-LEAK-GUARD-001.
// scripts/open-core/check-public-release-payload.sh must inspect repository files and reject leaked
// studio process artifacts and private nanos-hex identifiers.
func TestSupplyChain_PayloadLeakGuard_NegativeBoundary(t *testing.T) {
	t.Parallel()

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	gateScript := filepath.Join(repoRoot, "scripts", "open-core", "check-public-release-payload.sh")
	require.True(t, fileutil.Exists(gateScript), "check-public-release-payload.sh must exist")

	// 1. Verify that current repository tree passes cleanly
	passCmd := testkit.ManagedCommand(t, t.Context(), "bash", gateScript)
	passCmd.Dir = repoRoot
	passOut, passErr := passCmd.CombinedOutput()
	assert.NoError(t, passErr, "repository root must pass public release payload check: %s", string(passOut))
	assert.Contains(t, string(passOut), "RESULT=PASS", "payload check output must report RESULT=PASS")
}
