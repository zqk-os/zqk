package displaypack

import (
	_ "github.com/zqk-os/zqk/packs/display/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/display/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/display/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "display"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/display/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/display/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_displaypack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"component",
		"display",
	}
}
