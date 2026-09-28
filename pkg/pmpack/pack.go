package pmpack

import (
	_ "github.com/zqk-os/zqk/packs/pm/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/pm/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/pm/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "pm"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/pm/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/pm/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_pmpack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"corporate_initiative",
		"decision",
		"doc_entry",
		"glossary_term",
		"glossary_term_relation",
		"impact_analysis",
		"important_date",
		"library",
		"occupancy",
		"question",
		"release",
		"remaining_open",
		"stakeholder_profile",
		"technical_debt",
		"technical_spec",
		"vocabulary_scheme",
		"work_interval",
		"work_unit",
		"workstream_transition",
	}
}
