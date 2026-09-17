package scheduler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestSchedulerJobSeedDataConformance validates that all scheduler_job seed
// data objects conform to their object specifications.
// It addresses [REDACTED-ID] by running 'zqk system check scheduler_job'.
func TestSchedulerJobSeedDataConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping seed data conformance test in short mode")
	}

	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	projectRoot := filepath.Join(cwd, "..", "..")
	if _, err := fileutil.Stat(filepath.Join(projectRoot, "go.mod")); fileutil.IsNotExist(err) {
		t.Fatalf("could not find project root at %s", projectRoot)
	}

	ctx := context.Background()

	// Use the pre-compiled CLI binary to avoid IsInTest() returning true via 'go-build' heuristics.
	// We skip the test if it's not present, rather than blocking on compilation,
	// which causes extreme timeouts when multiple test bundles run concurrently.
	binPath := filepath.Join(projectRoot, "bin", "zqk")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Skipf("compiled CLI binary not found at %s (build bin/zqk first)", binPath)
	}

	cmd := exec.CommandContext(ctx, binPath, "system", "check", "scheduler_job", "--format", "json") //nolint:gosec
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "ZQK_API_KEY=ACC-TEST-HARNESS")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scheduler_job seed data validation failed: %v\nOutput: %s", err, string(output))
	}

	outStr := string(output)
	if strings.Contains(outStr, `"level":"error"`) {
		t.Fatalf("scheduler_job seed data validation logged an error:\n%s", outStr)
	}
}
