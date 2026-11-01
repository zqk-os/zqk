package displaypack_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/displaypack"
)

func TestDisplayPackConstantsAndKinds(t *testing.T) {
	if displaypack.Name != "display" {
		t.Fatalf("expected Name to be 'display', got %q", displaypack.Name)
	}
	if displaypack.SpecDir != "packs/display/specs" {
		t.Fatalf("expected SpecDir to be 'packs/display/specs', got %q", displaypack.SpecDir)
	}
	if displaypack.LifecycleDir != "packs/display/lifecycles" {
		t.Fatalf("expected LifecycleDir to be 'packs/display/lifecycles', got %q", displaypack.LifecycleDir)
	}
	if displaypack.OmitBuildTag != "zqk_omit_displaypack" {
		t.Fatalf("expected OmitBuildTag to be 'zqk_omit_displaypack', got %q", displaypack.OmitBuildTag)
	}

	kinds := displaypack.Kinds()
	if len(kinds) != 2 {
		t.Fatalf("expected 2 kinds, got %d", len(kinds))
	}
	expected := map[string]bool{
		"component": true,
		"display":   true,
	}
	for _, k := range kinds {
		if !expected[k] {
			t.Errorf("unexpected kind %q in displaypack", k)
		}
	}
}
