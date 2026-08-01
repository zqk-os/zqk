package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestLocalCapOrchestrator(t *testing.T) {
	// Set the brand executable name so EnvVar picks up ZQK_ environment variables correctly
	brand.SetExecutableName("zqk")

	ctx := context.Background()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	projectRoot, _ := filepath.Abs("../..")
	os.Setenv(zqkenv.Bin(), filepath.Join(projectRoot, "bin", "zqk"))
	defer os.Unsetenv(zqkenv.Bin())

	sp, err := storage.GetFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatal(err)
	}

	h := NewCapOrchestratorHandler(sp, projectRoot, logger).(*CapOrchestratorHandler)

	// Execute the full handler loop exactly as the background scheduler would
	err = h.Execute(ctx, &ScheduledJob{ID: "SCH-LOCAL-TEST", JobType: JobTypeCapOrchestrator})
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	t.Log("Success! CAP Orchestrator Loop Executed.")
}
