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

	// Compile the CLI binary to avoid IsInTest() returning true via 'go-build' heuristics
	tmpBin := filepath.Join(t.TempDir(), "zqk")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", tmpBin, "./cmd/zqk")
	buildCmd.Dir = projectRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build zqk: %v\n%s", err, string(out))
	}

	cmd := exec.CommandContext(ctx, tmpBin, "system", "check", "scheduler_job", "--format", "json")
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
