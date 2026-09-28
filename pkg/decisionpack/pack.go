package decisionpack

import (
	_ "github.com/zqk-os/zqk/packs/decision/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/decision/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/decision/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "decision"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/decision/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/decision/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_decisionpack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"decision",
		"impact_analysis",
		"question",
	}
}
