package scheduler

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestLocalCapOrchestrator(t *testing.T) {
	// Set the brand executable name so EnvVar picks up ZQK_ environment variables correctly
	brand.SetExecutableName("zqk")

	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	ctx := context.Background()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	projectRoot := env.TestRoot

	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := paths.ModuleRootFromPath(wd)
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	binPath := filepath.Join(realRoot, "bin", "zqk")
	if info, statErr := fileutil.Stat(binPath); statErr != nil || info.IsDir() {
		t.Skipf("compiled CLI binary not found at %s (build bin/zqk first)", binPath)
	}
	t.Setenv(zqkenv.Bin().Name(), binPath)
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "1")

	sp := env.Storage.(storagepkg.ObjectStorageProvider)

	h := NewCapOrchestratorHandler(sp, projectRoot, logger).(*CapOrchestratorHandler)

	// Execute the full handler loop exactly as the background scheduler would
	err = h.Execute(ctx, &ScheduledJob{ID: "SCH-LOCAL-TEST", JobType: JobTypeCapOrchestrator})
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	t.Log("Success! CAP Orchestrator Loop Executed.")
}
