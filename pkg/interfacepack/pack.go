package interfacepack

import (
	_ "github.com/zqk-os/zqk/packs/interface/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/interface/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "interface"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/interface/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/interface/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_interfacepack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"api_spec",
		"command_spec",
	}
}
