package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestPublicCandidateGeneration validates that the sync script generates the open-core export tree
// and successfully passes the strict Phase 0 Inventory police checks.
func TestPublicCandidateGeneration(t *testing.T) {
	if zqkenv.EnablePublicCandidateTest().Get() != "1" {
		t.Skip("Skipping candidate generation test unless " + zqkenv.EnablePublicCandidateTest().Name() + "=1 is set")
	}

	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", "..", ".."))

	syncScript := filepath.Join(repoRoot, "scripts", "open-core", "sync-public-candidate.sh")
	exportDir := filepath.Join(filepath.Dir(repoRoot), "zqk-public-candidate-export")

	cmd := execwrap.Command("sh", syncScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), zqkenv.PublicCandidateDir().Name()+"="+exportDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sync-public-candidate.sh failed: %v\noutput:\n%s", err, string(output))
	}

	candidateDir := exportDir

	// Verify excluded paths are truly gone
	excludedPaths := []string{
		filepath.Join(candidateDir, "cmd", "zqk"),
		filepath.Join(candidateDir, "pkg", "mesh"),
		filepath.Join(candidateDir, "pkg", "agent"),
	}

	for _, p := range excludedPaths {
		if _, err := fileutil.Stat(p); !fileutil.IsNotExist(err) {
			t.Errorf("expected excluded path %s to not exist, but it was found", p)
		}
	}

	// The sync script internally runs the police check, so if it succeeds, we know it passes.
	t.Log("Public candidate generation and policing passed successfully.")
}
