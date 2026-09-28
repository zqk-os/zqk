package librarypack

import (
	_ "github.com/zqk-os/zqk/packs/library/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/library/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/library/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "library"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/library/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/library/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_librarypack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"doc_entry",
		"library",
		"technical_spec",
	}
}
