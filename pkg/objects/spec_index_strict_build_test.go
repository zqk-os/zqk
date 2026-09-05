package objects

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestBuildSpecIndexFromSpecsDirStrict_rejectsInvalidStorageProfile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	specYAML := `ontology: bad_prof
schema_version: "2.0.0"
visibility: internal
storage_profile: not_a_real_profile
fields:
  id:
    type: string
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "bad_prof.yaml"), []byte(specYAML), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	_, err := BuildSpecIndexFromSpecsDirStrict(specsDir)
	if err == nil {
		t.Fatal("expected strict build to fail on invalid storage_profile")
	}
	if !strings.Contains(err.Error(), "unknown storage_profile") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Lenient build skips the broken file (legacy behavior for partial trees).
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("lenient build: %v", err)
	}
	if _, ok := idx.Kinds["bad_prof"]; ok {
		t.Fatal("expected broken spec to be omitted from lenient index")
	}
}
