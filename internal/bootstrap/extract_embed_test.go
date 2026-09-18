package bootstrap

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestExtractEmbeddedToTempProject proves the binary's embedded archive is enough
// to seed a greenfield project without using the source repo as project root.
func TestExtractEmbeddedToTempProject(t *testing.T) {
	t.Parallel()
	if ManifestPaths() == nil {
		t.Skip("no embedded manifest (archive missing from this build)")
	}

	moduleRoot, err := findModuleRoot()
	if err != nil || moduleRoot == emptyValue {
		t.Fatalf("module root: %v", err)
	}
	// Guard: source tree must not become the project under test.
	repoMarker := filepath.Join(moduleRoot, ".zqk", "config", "project.json")
	hadRepoProject := false
	if _, err := fileutil.Stat(repoMarker); err == nil {
		hadRepoProject = true
	}

	projectRoot := t.TempDir()
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
	if err := ExtractTo(projectRoot, logger, true); err != nil {
		t.Fatalf("ExtractTo temp project: %v", err)
	}

	internal := filepath.Join(projectRoot, paths.ProcessInternalDir)
	specs := filepath.Join(internal, "objects")
	if st, err := fileutil.Stat(specs); err != nil || !st.IsDir() {
		specs = filepath.Join(internal, "object_specs")
		if st, err := fileutil.Stat(specs); err != nil || !st.IsDir() {
			t.Fatalf("expected objects or object_specs under %s after extract: %v", internal, err)
		}
	}
	yamlCount := 0
	_ = filepath.WalkDir(specs, func(path string, d os.DirEntry, err error) error {
		if err == nil && d != nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".yaml") {
			yamlCount++
		}
		return nil
	})
	if yamlCount < 10 {
		t.Fatalf("expected a usable objects set, got %d yaml files in %s", yamlCount, specs)
	}

	// Instance-shaped Studio leaks must not ship in the embedded archive.
	var leaked []string
	_ = filepath.Walk(projectRoot, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		base := info.Name()
		if strings.HasPrefix(base, "BLI-") || strings.Contains(base, "hacked") {
			leaked = append(leaked, path)
		}
		return nil
	})
	if len(leaked) > 0 {
		t.Fatalf("embedded bootstrap leaked instance-like files: %v", leaked)
	}

	if !hadRepoProject {
		if _, err := fileutil.Stat(repoMarker); err == nil {
			t.Fatalf("extract must not create %s (repo is not the user project root)", repoMarker)
		}
	}
}
