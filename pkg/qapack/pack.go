package qapack

import (
	_ "github.com/zqk-os/zqk/packs/qa/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/qa/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/qa/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "qa"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/qa/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/qa/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_qapack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"code_quality_metric",
		"code_reference",
		"maturation_report",
		"qa_success",
		"test_audit_aggregation_metric",
		"test_command_rule",
		"validation_rule",
		"verification_matrix",
	}
}
