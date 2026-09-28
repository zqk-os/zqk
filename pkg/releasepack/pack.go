package releasepack

import (
	_ "github.com/zqk-os/zqk/packs/release/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/release/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "release"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/release/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/release/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_releasepack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"release",
	}
}
