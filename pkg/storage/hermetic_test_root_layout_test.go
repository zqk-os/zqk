package storage

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	hermeticRealSpec      = "ontology: backlog_item\n# authoritative repository spec\n"
	hermeticRealLifecycle = "lifecycle: base_object\n"
	hermeticFixtureSpec   = "kind: backlog_item\nname: Fixture\n"
	hermeticSpecFile      = "backlog_item.yaml"
	hermeticLifecycleFile = "base_object_lifecycle.yaml"
)

// seedYAML writes content at root/relDir/name, creating parents.
func seedYAML(t *testing.T, root, relDir, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, relDir)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	full := filepath.Join(dir, name)
	if err := fileutil.WriteFile(full, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

func expectFileContent(t *testing.T, path, want, what string) {
	t.Helper()
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: read %s: %v", what, path, err)
	}
	if string(data) != want {
		t.Errorf("%s:\n got: %q\nwant: %q", what, string(data), want)
	}
}

// seedSourceRoot returns a stand-in repository root holding authoritative specs and
// lifecycles, and binds it as the bootstrap source.
func seedSourceRoot(t *testing.T) (srcRoot, specPath string) {
	t.Helper()
	srcRoot = t.TempDir()
	specPath = seedYAML(t, srcRoot, paths.ProcessInternalObjectSpecsDir, hermeticSpecFile, hermeticRealSpec)
	seedYAML(t, srcRoot, paths.ProcessInternalLifecyclesDir, hermeticLifecycleFile, hermeticRealLifecycle)
	t.Setenv(zqkenv.ProjectRoot().Name(), srcRoot)
	return srcRoot, specPath
}

// TestEnsureHermeticTestRootLayoutNeverWritesIntoSourceRoot pins the copy direction.
// The arguments to testenvroot.CopyObjectSpecsFromProject are (destination, source);
// reversing them made an isolated test overwrite the repository's real object specs
// with its own fixtures, which then read back as the kernel's schema.
// TRACK: PRI-STABILIZE-FAILCLOSED-READS-001
func TestEnsureHermeticTestRootLayoutNeverWritesIntoSourceRoot(t *testing.T) {
	// Exercises the spec copy itself: an empty test root is the only state in which it
	// runs, so a reversed direction is invisible to any case that pre-seeds specs.
	t.Run("empty test root receives specs and lifecycles", func(t *testing.T) {
		_, srcSpecPath := seedSourceRoot(t)
		testRoot := t.TempDir()

		if err := ensureHermeticTestRootLayout(testRoot); err != nil {
			t.Fatalf("ensureHermeticTestRootLayout: %v", err)
		}

		expectFileContent(t, srcSpecPath, hermeticRealSpec, "source root spec must be untouched")
		expectFileContent(t,
			filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir, hermeticSpecFile),
			hermeticRealSpec, "test root must receive the real spec")
		expectFileContent(t,
			filepath.Join(testRoot, paths.ProcessInternalLifecyclesDir, hermeticLifecycleFile),
			hermeticRealLifecycle, "test root must receive the real lifecycle")
	})

	// Specs and lifecycles are filled independently: a test that authored its own
	// schema keeps it even though lifecycles still need seeding.
	t.Run("existing fixture specs survive lifecycle seeding", func(t *testing.T) {
		_, srcSpecPath := seedSourceRoot(t)
		testRoot := t.TempDir()
		testSpecPath := seedYAML(t, testRoot, paths.ProcessInternalObjectSpecsDir, hermeticSpecFile, hermeticFixtureSpec)

		if err := ensureHermeticTestRootLayout(testRoot); err != nil {
			t.Fatalf("ensureHermeticTestRootLayout: %v", err)
		}

		expectFileContent(t, srcSpecPath, hermeticRealSpec, "source root spec must be untouched")
		expectFileContent(t, testSpecPath, hermeticFixtureSpec, "fixture spec must not be overwritten")
		expectFileContent(t,
			filepath.Join(testRoot, paths.ProcessInternalLifecyclesDir, hermeticLifecycleFile),
			hermeticRealLifecycle, "lifecycle must still be seeded")
	})
}
