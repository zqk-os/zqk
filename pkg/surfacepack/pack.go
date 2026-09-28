package surfacepack

import (
	_ "github.com/zqk-os/zqk/packs/surface/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/surface/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/surface/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "surface"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/surface/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/surface/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_surfacepack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"api_spec",
		"command_spec",
		"component",
		"context_refresh_schedule",
		"display",
		"evolution_management",
		"import_tracking",
		"metadata_package",
		"process_hygiene_rule",
		"scenario",
		"workflow",
	}
}
