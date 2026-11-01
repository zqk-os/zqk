package interfacepack_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/interfacepack"
)

func TestInterfacePackConstantsAndKinds(t *testing.T) {
	if interfacepack.Name != "interface" {
		t.Fatalf("expected Name to be 'interface', got %q", interfacepack.Name)
	}
	if interfacepack.SpecDir != "packs/interface/specs" {
		t.Fatalf("expected SpecDir to be 'packs/interface/specs', got %q", interfacepack.SpecDir)
	}
	if interfacepack.LifecycleDir != "packs/interface/lifecycles" {
		t.Fatalf("expected LifecycleDir to be 'packs/interface/lifecycles', got %q", interfacepack.LifecycleDir)
	}
	if interfacepack.OmitBuildTag != "zqk_omit_interfacepack" {
		t.Fatalf("expected OmitBuildTag to be 'zqk_omit_interfacepack', got %q", interfacepack.OmitBuildTag)
	}

	kinds := interfacepack.Kinds()
	if len(kinds) != 2 {
		t.Fatalf("expected 2 kinds, got %d", len(kinds))
	}
	expected := map[string]bool{
		"api_spec":     true,
		"command_spec": true,
	}
	for _, k := range kinds {
		if !expected[k] {
			t.Errorf("unexpected kind %q in interfacepack", k)
		}
	}
}
