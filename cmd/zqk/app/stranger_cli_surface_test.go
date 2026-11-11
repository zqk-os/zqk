package app

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger CLI surface
func TestShouldRegisterStudioPackCLI_defaultOff(t *testing.T) {
	t.Setenv(zqkenv.EnableStudioPackTools().Name(), "")
	t.Setenv(zqkenv.StudioDogfood().Name(), "")
	if shouldRegisterStudioPackCLI() {
		t.Fatal("stranger default must not register pack ontology CLI")
	}
}

func TestShouldRegisterStudioPackCLI_optIn(t *testing.T) {
	t.Setenv(zqkenv.EnableStudioPackTools().Name(), "1")
	t.Setenv(zqkenv.StudioDogfood().Name(), "")
	if !shouldRegisterStudioPackCLI() {
		t.Fatal("ZQK_ENABLE_STUDIO_PACK_TOOLS=1 must restore pack ontology CLI")
	}
}
