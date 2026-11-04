package librarypack_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/librarypack"
)

func TestLibraryPackConstantsAndKinds(t *testing.T) {
	if librarypack.Name != "library" {
		t.Fatalf("expected Name to be 'library', got %q", librarypack.Name)
	}
	if librarypack.SpecDir != "packs/library/specs" {
		t.Fatalf("expected SpecDir to be 'packs/library/specs', got %q", librarypack.SpecDir)
	}
	if librarypack.LifecycleDir != "packs/library/lifecycles" {
		t.Fatalf("expected LifecycleDir to be 'packs/library/lifecycles', got %q", librarypack.LifecycleDir)
	}
	if librarypack.OmitBuildTag != "zqk_omit_librarypack" {
		t.Fatalf("expected OmitBuildTag to be 'zqk_omit_librarypack', got %q", librarypack.OmitBuildTag)
	}

	kinds := librarypack.Kinds()
	if len(kinds) == 0 {
		t.Fatal("expected non-empty kinds")
	}
	expected := map[string]bool{
		"doc_entry":      true,
		"library":        true,
		"technical_spec": true,
	}
	for _, k := range kinds {
		if !expected[k] {
			t.Errorf("unexpected kind %q in librarypack", k)
		}
	}
}
