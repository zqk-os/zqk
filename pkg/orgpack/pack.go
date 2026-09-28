package orgpack

import (
	_ "github.com/zqk-os/zqk/packs/org/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/org/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/org/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "org"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/org/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/org/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_orgpack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"corporate_initiative",
		"department",
		"division",
		"organization",
		"organizational_change",
		"partnership",
		"stakeholder_profile",
		"team",
		"team_configuration",
	}
}
