package vocabularypack

import (
	_ "github.com/zqk-os/zqk/packs/vocabulary/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/vocabulary/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/vocabulary/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "vocabulary"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/vocabulary/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/vocabulary/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_vocabularypack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"glossary_term",
		"glossary_term_relation",
		"import_tracking",
		"vocabulary_scheme",
	}
}
