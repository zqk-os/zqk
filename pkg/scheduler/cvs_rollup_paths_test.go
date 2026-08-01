package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestResolveCVSRollupLatestJSONPath_defaultMatchesPathsLayout(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	got := ResolveCVSRollupLatestJSONPath(root, "")
	want := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, paths.LogsDriftSubdir, convergenceOrchestrateRollupLatestFileName)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveCVSRollupLatestJSONPath_absoluteEnv(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	abs := filepath.Join(root, "custom", "rollup-out.json")
	got := ResolveCVSRollupLatestJSONPath(root, abs)
	if got != abs {
		t.Fatalf("got %q want %q", got, abs)
	}
}

func TestResolveCVSRollupLatestJSONPath_repoRelativeEnv(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rel := paths.ProjectDataDir + "/logs/" + paths.LogsDriftSubdir + "/custom.json"
	got := ResolveCVSRollupLatestJSONPath(root, rel)
	want := filepath.Join(root, paths.ProjectDataDir, "logs", paths.LogsDriftSubdir, "custom.json")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
