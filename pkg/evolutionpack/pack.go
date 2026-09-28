package evolutionpack

import (
	_ "github.com/zqk-os/zqk/packs/evolution/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/evolution/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/evolution/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "evolution"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/evolution/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/evolution/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_evolutionpack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"evolution_management",
	}
}
