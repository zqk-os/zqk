package app

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TRACK: BLI-1794392273289214000-f82837c4
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
