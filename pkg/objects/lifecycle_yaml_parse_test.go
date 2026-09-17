package objects

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestParseLifecycleYAMLDir_RepoParses(t *testing.T) {
	t.Parallel()
	dir := repoLifecycleDir(t)
	issues, err := ParseLifecycleYAMLDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) > 0 {
		t.Fatalf("lifecycle YAML parse: %v", FormatLifecycleYAMLIssues(issues))
	}
}

func TestParseConfigYAMLDir_RepoParses(t *testing.T) {
	t.Parallel()
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := paths.ModuleRootFromPath(wd)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	issues, err := ParseConfigYAMLDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) > 0 {
		t.Fatalf("config YAML parse: %v", FormatLifecycleYAMLIssues(issues))
	}
}

func TestParseLifecycleYAMLDir_SmashedBoolFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := []byte(`object_type: smash_probe
statuses:
  - value: archived
    display: Archived
    terminal: true
    archive: true      - leftover precondition bullet
transitions: []
`)
	path := filepath.Join(dir, "smash_probe_lifecycle.yaml")
	if err := fileutil.WriteFile(path, body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	issues, err := ParseLifecycleYAMLDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d (%v)", len(issues), FormatLifecycleYAMLIssues(issues))
	}
	if !strings.Contains(issues[0].Err.Error(), "smashed bool flag") {
		t.Fatalf("want smash heuristic, got %v", issues[0].Err)
	}
}

func TestParseLifecycleYAMLDir_UnquotedColonPostcondition(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := []byte(`object_type: colon_probe
statuses:
  - value: planned
    display: Planned
transitions:
  - from: planned
    to: planned
    description: noop
    postconditions:
      - terminal re-entry: remaining occupancy is a new closed system
`)
	path := filepath.Join(dir, "colon_probe_lifecycle.yaml")
	if err := fileutil.WriteFile(path, body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	issues, err := ParseLifecycleYAMLDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue (map into []string), got %d (%v)", len(issues), FormatLifecycleYAMLIssues(issues))
	}
}

func TestLifecycleLoader_EnsureReady_WarnsInvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(dir, "base_object_lifecycle.yaml"), []byte(`object_type: base_object
statuses:
  - value: proposed
    display: Proposed
    origin: true
transitions: []
`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(dir, "bad_lifecycle.yaml"), []byte(`object_type: bad
statuses:
  - value: archived
    archive: true      - leftover
transitions: []
`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	loader := NewLifecycleLoader(dir)
	err := loader.EnsureReady(t.Context())
	if err == nil {
		t.Fatal("expected EnsureReady error on smashed YAML")
	}
	if !strings.Contains(err.Error(), "startup YAML parse") {
		t.Fatalf("want startup YAML parse error, got %v", err)
	}
}

func repoLifecycleDir(t *testing.T) string {
	t.Helper()
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := paths.ModuleRootFromPath(wd)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, paths.ProcessInternalLifecyclesDir)
}
