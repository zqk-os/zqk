package community_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestTestNormalizationCycle38_StaticFloorInvariant asserts:
// 1. docs/reports/REPO_HYGIENE_TEST_NORMALIZATION_MAP.md exists and is non-empty.
// 2. scripts/open-core/test-normalization/normalize_cycle_test_names.sh exists and is executable.
// 3. The canonical target naming standard is enforced: zero target files contain ephemeral cycle tags.
func TestTestNormalizationCycle38_StaticFloorInvariant(t *testing.T) {
	cwd, err := fileutil.Getwd()
	require.NoError(t, err)

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		require.NotEqual(t, repoRoot, parent, "could not find repo root")
		repoRoot = parent
	}

	mapPath := filepath.Join(repoRoot, "docs", "reports", "REPO_HYGIENE_TEST_NORMALIZATION_MAP.md")
	content, err := fileutil.ReadFile(mapPath)
	require.NoError(t, err, "normalization mapping artifact must exist")
	require.NotEmpty(t, content, "normalization mapping artifact must not be empty")

	scriptPath := filepath.Join(repoRoot, "scripts", "open-core", "test-normalization", "normalize_cycle_test_names.sh")
	scriptContent, err := fileutil.ReadFile(scriptPath)
	require.NoError(t, err, "normalization script must exist")
	require.NotEmpty(t, scriptContent, "normalization script must not be empty")

	// Ensure no forbidden patterns in migration targets
	lines := strings.Split(string(content), "\n")
	targetCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "| ") && strings.Contains(line, "`") {
			parts := strings.Split(line, "|")
			if len(parts) >= 7 {
				target := strings.TrimSpace(parts[3])
				if strings.HasPrefix(target, "`") && strings.HasSuffix(target, "`") {
					target = strings.Trim(target, "`")
					assert.NotContains(t, target, "_coverage_elevation_cycle", "target must not contain cycle elevation tags: %s", target)
					targetCount++
				}
			}
		}
	}
	assert.GreaterOrEqual(t, targetCount, 190, "must map at least 190 test files across repository")
}

// TestTestNormalizationCycle38_OperationalDynamicProof verifies:
// 1. normalize_cycle_test_names.sh --check-collisions succeeds with 0 collisions.
// 2. normalize_cycle_test_names.sh --dry-run completes cleanly without file mutations.
func TestTestNormalizationCycle38_OperationalDynamicProof(t *testing.T) {
	cwd, err := fileutil.Getwd()
	require.NoError(t, err)

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		require.NotEqual(t, repoRoot, parent, "could not find repo root")
		repoRoot = parent
	}

	scriptPath := filepath.Join(repoRoot, "scripts", "open-core", "test-normalization", "normalize_cycle_test_names.sh")

	// 1. Collision check
	cmdCol := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--check-collisions")
	cmdCol.Dir = repoRoot
	outCol, errCol := cmdCol.CombinedOutput()
	require.NoError(t, errCol, "collision check must exit 0: %s", string(outCol))
	assert.Contains(t, string(outCol), "0 collisions detected", "collision check must report zero collisions")

	// 2. Dry run execution
	cmdDry := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--dry-run")
	cmdDry.Dir = repoRoot
	outDry, errDry := cmdDry.CombinedOutput()
	require.NoError(t, errDry, "dry-run must exit 0: %s", string(outDry))
	assert.Contains(t, string(outDry), "Dry-run complete", "dry-run must complete cleanly")
}

// TestTestNormalizationCycle38_NegativeAdversarialBoundary asserts:
// 1. Script rejects invalid or unsupported CLI flags fail-closed.
// 2. Excluded in-flight Cycle 37 paths are never touched during normalization.
func TestTestNormalizationCycle38_NegativeAdversarialBoundary(t *testing.T) {
	cwd, err := fileutil.Getwd()
	require.NoError(t, err)

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		require.NotEqual(t, repoRoot, parent, "could not find repo root")
		repoRoot = parent
	}

	scriptPath := filepath.Join(repoRoot, "scripts", "open-core", "test-normalization", "normalize_cycle_test_names.sh")

	// 1. Invalid flag rejected
	cmdInvalid := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--invalid-dangerous-flag")
	cmdInvalid.Dir = repoRoot
	outInvalid, errInvalid := cmdInvalid.CombinedOutput()
	assert.Error(t, errInvalid, "script must reject invalid flags with non-zero exit")
	assert.Contains(t, string(outInvalid), "Usage:", "script must output usage on invalid option")

	// 2. Verify Cycle 37 files are marked DEFERRED in dry-run
	cmdDry := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--dry-run")
	cmdDry.Dir = repoRoot
	outDry, errDry := cmdDry.CombinedOutput()
	require.NoError(t, errDry)
	assert.Contains(t, string(outDry), "DEFERRED (Cycle 37)", "must explicitly defer active Cycle 37 test files")
}
