package system

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vds"
)

func TestAgentOnboard_SeedsVDSProfile(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipSetupTestEnvironment: true,
		SkipFileStorage:          true,
	})
	tmpDir := proj.Root

	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(originalDir)

	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	spinePath := filepath.Join(tmpDir, vds.DefaultSpineProfileRel)
	// Verify it does not exist before onboard
	if _, err := fileutil.Stat(spinePath); err == nil {
		t.Fatalf("expected spine profile to be absent initially")
	}

	cmd := NewAgentOnboardCmd()
	cmd.SetArgs([]string{"--headless", "--skip-seat", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent-onboard failed: %v", err)
	}

	// Verify spine profile was seeded
	if _, err := fileutil.Stat(spinePath); err != nil {
		t.Fatalf("expected spine profile to be seeded after agent-onboard: %v", err)
	}

	data, err := fileutil.ReadFile(spinePath)
	if err != nil {
		t.Fatalf("failed to read seeded spine profile: %v", err)
	}
	if string(data) != vds.DefaultEmbeddedSpineProfileYAML {
		t.Fatalf("seeded spine profile does not match DefaultEmbeddedSpineProfileYAML")
	}
}
