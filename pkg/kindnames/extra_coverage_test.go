package kindnames

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadKindNamesFromSpecsDir_EdgeCases(t *testing.T) {
	t.Parallel()

	// 1. Empty specsDir
	if _, err := LoadKindNamesFromSpecsDir(""); err == nil {
		t.Errorf("expected error on empty specsDir")
	}

	// 2. Non-existent specsDir
	if _, err := LoadKindNamesFromSpecsDir("/nonexistent/specs/dir"); err == nil {
		t.Errorf("expected error on non-existent specsDir")
	}

	// 3. Directory with non-yaml files, placeholder, appledouble, subdirectories, empty ontology, and bad yaml
	root := t.TempDir()

	// Subdirectory (should be skipped)
	subDir := filepath.Join(root, "subdir")
	_ = fileutil.MkdirAll(subDir, paths.DirPerm750)

	// Non-yaml file (should be skipped)
	_ = fileutil.WriteFile(filepath.Join(root, "notes.txt"), []byte("not yaml"), paths.FilePerm600)

	// Placeholder spec (should be skipped)
	_ = fileutil.WriteFile(filepath.Join(root, "_placeholder.yaml"), []byte("ontology: skip_me"), paths.FilePerm600)

	// Apple double file (should be skipped)
	_ = fileutil.WriteFile(filepath.Join(root, "._spec.yaml"), []byte("apple double"), paths.FilePerm600)

	// Bad yaml file (should be skipped during parse)
	_ = fileutil.WriteFile(filepath.Join(root, "bad.yaml"), []byte("invalid: yaml: ["), paths.FilePerm600)

	// Empty ontology file (should be skipped)
	_ = fileutil.WriteFile(filepath.Join(root, "empty_ontology.yaml"), []byte("title: no ontology\n"), paths.FilePerm600)

	// Since no valid ontology exists, LoadKindNamesFromSpecsDir must return errNoOntologyTemplate
	if _, err := LoadKindNamesFromSpecsDir(root); err == nil {
		t.Errorf("expected error when no valid ontologies found")
	}

	validRoot := t.TempDir()
	// Now add valid .yml and .yaml files
	_ = fileutil.WriteFile(filepath.Join(validRoot, "item1.yaml"), []byte("ontology: custom_kind_one\n"), paths.FilePerm600)
	_ = fileutil.WriteFile(filepath.Join(validRoot, "item2.yml"), []byte("ontology: custom_kind_two\n"), paths.FilePerm600)

	kinds, err := LoadKindNamesFromSpecsDir(validRoot)
	if err != nil {
		t.Fatalf("LoadKindNamesFromSpecsDir failed: %v", err)
	}
	if len(kinds) != 2 {
		t.Errorf("expected 2 kinds, got %d: %v", len(kinds), kinds)
	}
	if _, ok := kinds["custom_kind_one"]; !ok {
		t.Errorf("expected custom_kind_one in kinds")
	}
	if _, ok := kinds["custom_kind_two"]; !ok {
		t.Errorf("expected custom_kind_two in kinds")
	}
}
