package workflowpack

import (
	_ "github.com/zqk-os/zqk/packs/workflow/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/workflow/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/workflow/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "workflow"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/workflow/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/workflow/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_workflowpack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"workflow",
	}
}
