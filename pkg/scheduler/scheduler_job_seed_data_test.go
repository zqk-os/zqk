package scheduler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSchedulerJobSeedDataConformance validates that all scheduler_job seed
// data objects conform to their object specifications.
// It addresses ITEM-EXAMPLE by running 'zqk system check scheduler_job'.
func TestSchedulerJobSeedDataConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping seed data conformance test in short mode")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	projectRoot := filepath.Join(cwd, "..", "..")
	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); os.IsNotExist(err) {
		t.Fatalf("could not find project root at %s", projectRoot)
	}

	ctx := context.Background()

	// Use 'go run' to ensure we test the current codebase against the seed data
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/zqk", "system", "check", "scheduler_job", "--format", "json")
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "ZQK_API_KEY=account:system")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scheduler_job seed data validation failed: %v\nOutput: %s", err, string(output))
	}

	outStr := string(output)
	if strings.Contains(outStr, `"level":"error"`) {
		t.Fatalf("scheduler_job seed data validation logged an error:\n%s", outStr)
	}
}
