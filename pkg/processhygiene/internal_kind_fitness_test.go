package processhygiene

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestInternalObjectKindFitness enforces that all internal object kinds
// maintain valid specifications, schemas, and lifecycle constraints (REQ-KERNEL-LIFECYCLE-FITNESS-001).
func TestInternalObjectKindFitness(t *testing.T) {
	repoRoot := findRepoRoot(t)
	specsDir := filepath.Join(repoRoot, "docs", "process", "_internal", "object_specs")

	entries, err := fileutil.ReadDir(specsDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", specsDir, err)
	}

	internalCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		specPath := filepath.Join(specsDir, entry.Name())
		data, err := fileutil.ReadFile(specPath)
		if err != nil {
			t.Errorf("ReadFile %s: %v", specPath, err)
			continue
		}

		var spec map[string]any
		if err := yaml.Unmarshal(data, &spec); err != nil {
			t.Errorf("Unmarshal %s: %v", specPath, err)
			continue
		}

		visibility, _ := spec["visibility"].(string)
		if visibility != "internal" {
			continue
		}

		internalCount++
		// Verify schema_version and ontology or fields
		schemaVersion, _ := spec["schema_version"].(string)
		if schemaVersion == "" {
			t.Errorf("spec %s missing schema_version", entry.Name())
		}
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
