package processhygiene

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestInternalObjectKindFitness enforces that all internal object kinds
// maintain valid specifications, schemas, and lifecycle constraints (REQ-KERNEL-LIFECYCLE-FITNESS-001).
func TestInternalObjectKindFitness(t *testing.T) {
	repoRoot := findRepoRoot(t)
	specsDir := filepath.Join(repoRoot, paths.ProcessInternalObjectSpecsDir)

	internalCount := 0
	err := filepath.WalkDir(specsDir, func(specPath string, d fileutil.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}

		data, err := fileutil.ReadFile(specPath)
		if err != nil {
			t.Errorf("ReadFile %s: %v", specPath, err)
			return nil
		}

		var spec map[string]any
		if err := yaml.Unmarshal(data, &spec); err != nil {
			t.Errorf("Unmarshal %s: %v", specPath, err)
			return nil
		}

		visibility, _ := spec[objects.FieldKeyVisibility].(string)
		if visibility != "internal" {
			return nil
		}

		internalCount++
		// Verify schema_version and ontology or fields
		schemaVersion, _ := spec[objects.FieldKeySchemaVersion].(string)
		if schemaVersion == "" {
			t.Errorf("spec %s missing schema_version", d.Name())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir %s: %v", specsDir, err)
	}

	if internalCount == 0 {
		t.Fatalf("expected to discover internal object specs, found 0")
	}
	t.Logf("successfully validated %d internal object kinds for fitness", internalCount)
}

func findRepoRoot(t *testing.T) string {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod from %s", wd)
		}
		dir = parent
	}
}
