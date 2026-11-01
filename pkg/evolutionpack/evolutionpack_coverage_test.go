package evolutionpack_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/evolutionpack"
)

func TestEvolutionPackConstantsAndKinds(t *testing.T) {
	if evolutionpack.Name != "evolution" {
		t.Fatalf("expected Name to be 'evolution', got %q", evolutionpack.Name)
	}
	if evolutionpack.SpecDir != "packs/evolution/specs" {
		t.Fatalf("expected SpecDir to be 'packs/evolution/specs', got %q", evolutionpack.SpecDir)
	}
	if evolutionpack.LifecycleDir != "packs/evolution/lifecycles" {
		t.Fatalf("expected LifecycleDir to be 'packs/evolution/lifecycles', got %q", evolutionpack.LifecycleDir)
	}
	if evolutionpack.OmitBuildTag != "zqk_omit_evolutionpack" {
		t.Fatalf("expected OmitBuildTag to be 'zqk_omit_evolutionpack', got %q", evolutionpack.OmitBuildTag)
	}

	kinds := evolutionpack.Kinds()
	if len(kinds) != 1 {
		t.Fatalf("expected 1 kind, got %d", len(kinds))
	}
	if kinds[0] != "evolution_management" {
		t.Errorf("expected kind 'evolution_management', got %q", kinds[0])
	}
}
