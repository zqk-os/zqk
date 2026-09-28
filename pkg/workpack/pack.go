// Package workpack is the included planning and verification pack.
// The composition root links it by default. A root that passes the build tag
// zqk_omit_workpack leaves this package out of the binary.
package workpack

import "github.com/zqk-os/zqk/pkg/objects"

const (
	// Name is the pack id the composition root links.
	Name = "work"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_workpack"
)

// Kinds are the planning and verification kinds this pack owns.
func Kinds() []string {
	return []string{
		objects.KindGoal,
		objects.KindRequirement,
		objects.KindCriteria,
		objects.KindTestCase,
	}
}

var enabled bool

// Enable records that the composition root linked this pack.
func Enable() { enabled = true }

// Enabled reports whether the composition root linked this pack.
func Enabled() bool { return enabled }
