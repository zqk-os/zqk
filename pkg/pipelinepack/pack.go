package pipelinepack

import (
	_ "github.com/zqk-os/zqk/packs/pipeline/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/pipeline/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "pipeline"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/pipeline/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/pipeline/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_pipelinepack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"pipeline",
		"pipeline_definition",
		"pipeline_execution",
		"pipeline_stage",
	}
}
