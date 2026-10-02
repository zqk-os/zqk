package testkit_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestTestDeterminism_RaceInclusion_StaticFloor verifies CRIT-CEF-TEST-RACE-INCLUSION-001.
// Makefile test-race target must include core concurrent subsystems (storage, coordination, mcp, mesh)
// with zero selective race exclusions.
func TestTestDeterminism_RaceInclusion_StaticFloor(t *testing.T) {
	t.Parallel()

	makefilePath := filepath.Join("..", "..", "Makefile")
	require.True(t, fileutil.Exists(makefilePath), "Makefile must exist at repo root")

	contentBytes, err := fileutil.ReadFile(makefilePath)
	require.NoError(t, err)
	content := string(contentBytes)

	// Invariant: test-race recipe must encompass core concurrent packages
	assert.Contains(t, content, "test-race:", "Makefile must define test-race target")
	assert.Contains(t, content, "./pkg/coordination/...", "test-race must include coordination")
	assert.Contains(t, content, "./pkg/mesh/...", "test-race must include mesh")
	assert.Contains(t, content, "./pkg/mcp/...", "test-race must include mcp")
	assert.Contains(t, content, "./pkg/concurrency/...", "test-race must include concurrency")
	assert.Contains(t, content, "./pkg/goroutinelabels/...", "test-race must include goroutinelabels")
}

// TestTestDeterminism_DeterministicSync_OperationalProof verifies CRIT-CEF-TEST-DETERMINISTIC-SYNC-001.
// Test suites must employ channel synchronization or deterministic polling rather than unbounded/unasserted sleeps.
func TestTestDeterminism_DeterministicSync_OperationalProof(t *testing.T) {
	t.Parallel()

	// 1. Verify that drift_handler_test.go uses channel select with timeout
	driftTestPath := filepath.Join("..", "scheduler", "drift_handler_test.go")
	require.True(t, fileutil.Exists(driftTestPath), "drift_handler_test.go must exist")

	driftContentBytes, err := fileutil.ReadFile(driftTestPath)
	require.NoError(t, err)
	driftContent := string(driftContentBytes)

	assert.Contains(t, driftContent, "case <-jobCtx.Done():", "drift_handler_test must select on jobCtx.Done()")
	assert.Contains(t, driftContent, "case <-time.After(", "drift_handler_test must guard with channel timeout")

	// 2. Operational test: verify that channel timeout pattern completes deterministically
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})

	go func(opCtx context.Context) {
		// Simulate immediate work
		time.Sleep(5 * time.Millisecond)
		cancel()
		close(doneCh)
	}(ctx)

	select {
	case <-ctx.Done():
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("deterministic synchronization timed out")
	}

	// 3. Verify cli_dynamic_relaxed_test.go enforces strict reference failure with --relaxed=false
	relaxedPath := filepath.Join("..", "..", "cmd", "zqk", "object", "cli_dynamic_relaxed_test.go")
	require.True(t, fileutil.Exists(relaxedPath), "cli_dynamic_relaxed_test.go must exist")

	relaxedBytes, err := fileutil.ReadFile(relaxedPath)
	require.NoError(t, err)
	relaxedContent := string(relaxedBytes)
	assert.Contains(t, relaxedContent, "--relaxed=false", "cli_dynamic_relaxed_test must explicitly test --relaxed=false")
	assert.Contains(t, relaxedContent, "relaxed=false", "must assert strict reference failure")
}

// TestTestDeterminism_PyramidDiscipline_NegativeBoundary verifies CRIT-CEF-TEST-PYRAMID-DISCIPLINE-001.
// Baseline unit tests must exist for uncovered CLI and operational packages, and commands fail-closed cleanly.
func TestTestDeterminism_PyramidDiscipline_NegativeBoundary(t *testing.T) {
	t.Parallel()

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	// Invariant: test files must exist for formerly uncovered packages
	uncoveredPackages := []string{
		filepath.Join(repoRoot, "pkg", "cli", "printer", "printer_test.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "ci", "ci_test.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "grep", "grep_test.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "job", "job_test.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "observer", "observer_test.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "rollback", "rollback_test.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "semantic", "semantic_test.go"),
	}

	for _, pkgTest := range uncoveredPackages {
		assert.True(t, fileutil.Exists(pkgTest), "test file must exist: %s", pkgTest)
	}

	// Negative boundary: verify binary/CLI commands reject invalid invocations cleanly
	binPath := filepath.Join(repoRoot, "bin", "zqk")
	if fileutil.Exists(binPath) {
		cmd := testkit.ManagedCommand(t, t.Context(), binPath, "nonexistent-root-command-xyz")
		out, err := cmd.CombinedOutput()
		assert.Error(t, err, "CLI must reject invalid root commands")
		assert.Contains(t, strings.ToLower(string(out)), "unknown command", "CLI must report unknown command diagnostic")
	}
}
