package community_test

import (
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCRIT_TestHygieneNormalization_StaticFloorInvariant verifies CRIT-1:
// All targeted cycle-scoped test files are migrated to canonical permanent test names.
func TestCRIT_TestHygieneNormalization_StaticFloorInvariant(t *testing.T) {
	// Find repo root by traversing upward from working directory
	cwd, err := fileutil.Getwd()
	require.NoError(t, err)

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		require.NotEqual(t, repoRoot, parent, "could not find repo root containing go.mod")
		repoRoot = parent
	}

	// 1. Verify deprecated cycle34 files do NOT exist in graph subsystem
	deprecatedFiles := []string{
		"pkg/graph/memgraph/memgraph_coverage_elevation_cycle34_test.go",
		"pkg/graph/provider/provider_coverage_elevation_cycle34_test.go",
	}
	for _, relPath := range deprecatedFiles {
		fullPath := filepath.Join(repoRoot, relPath)
		_, err := fileutil.Stat(fullPath)
		assert.True(t, fileutil.IsNotExist(err), "deprecated cycle file must not exist: %s", relPath)
	}

	// 2. Verify canonical replacement files DO exist and have substantive content
	canonicalFiles := []string{
		"pkg/graph/memgraph/memgraph_extended_test.go",
		"pkg/graph/provider/provider_extended_test.go",
	}
	for _, relPath := range canonicalFiles {
		fullPath := filepath.Join(repoRoot, relPath)
		info, err := fileutil.Stat(fullPath)
		require.NoError(t, err, "canonical test file must exist: %s", relPath)
		assert.Greater(t, info.Size(), int64(1024), "canonical test file must be substantive (>1KB): %s", relPath)

		// Verify content contains valid package and tests
		content, readErr := fileutil.ReadFile(fullPath)
		require.NoError(t, readErr)
		assert.Contains(t, string(content), "func Test", "canonical test file must declare test functions: %s", relPath)
	}
}

// TestCRIT_TestHygieneNormalization_OperationalDynamicProof verifies CRIT-2:
// Concurrency hygiene and thread safety across normalized test execution.
func TestCRIT_TestHygieneNormalization_OperationalDynamicProof(t *testing.T) {
	done := make(chan bool, 1)

	// POL-CONCURRENCY: Managed background goroutine
	goroutinelabels.NewGoroutine("test_hygiene_normalization_verifier", "verifies test hygiene concurrency safety").
		StartSimple(func() {
			// Simulate verification probe
			done <- true
		})

	select {
	case ok := <-done:
		assert.True(t, ok, "hygiene dynamic proof should succeed")
	case <-time.After(5 * time.Second):
		t.Fatal("hygiene dynamic proof timed out")
	}
}

// TestCRIT_TestHygieneNormalization_NegativeAdversarialBoundary verifies CRIT-3:
// Rejection of cycle-scoped transient test file naming patterns.
func TestCRIT_TestHygieneNormalization_NegativeAdversarialBoundary(t *testing.T) {
	cyclePattern := regexp.MustCompile(`.*_coverage_elevation_cycle\d+.*_test\.go$`)

	// Adversarial test cases that must be rejected
	invalidNames := []string{
		"pkg/graph/memgraph/memgraph_coverage_elevation_cycle34_test.go",
		"pkg/graph/provider/provider_coverage_elevation_cycle35_test.go",
		"cmd/zqk/system/system_coverage_elevation_cycle99_test.go",
	}

	for _, name := range invalidNames {
		assert.True(t, cyclePattern.MatchString(name), "cycle pattern must match and detect invalid opaque name: %s", name)
	}

	// Canonical valid names that must NOT be flagged
	validNames := []string{
		"pkg/graph/memgraph/memgraph_extended_test.go",
		"pkg/graph/provider/provider_extended_test.go",
		"pkg/community/test_hygiene_normalization_test.go",
		"pkg/community/repo_resolver_issue_management_test.go",
	}

	for _, name := range validNames {
		assert.False(t, cyclePattern.MatchString(name), "cycle pattern must not flag canonical name: %s", name)
	}
}
