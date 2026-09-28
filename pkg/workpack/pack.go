// Package workpack is the included planning and verification pack.
// The composition root links it by default. A root that passes the build tag
// zqk_omit_workpack leaves this package out of the binary.
package workpack

import (
	"github.com/spf13/cobra"

	packbldr "github.com/zqk-os/zqk/packs/work/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// Name is the pack id the composition root links.
	Name = "work"
	// ToolDir is the instance builder tool directory for this pack.
	// Generated builders and enums are written beside it.
	ToolDir = "packs/work/instance_builders"
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

var (
	enabled    bool
	linkedRoot bool
	builders   = []any{
		packbldr.NewGoalInstanceBuilder,
		packbldr.NewRequirementInstanceBuilder,
		packbldr.NewCriteriaInstanceBuilder,
		packbldr.NewTestCaseInstanceBuilder,
	}
)

// BuilderCount is the number of generated constructors this pack links.
func BuilderCount() int { return len(builders) }

// Enable records that the composition root linked this pack.
func Enable() { enabled = true }

// Enabled reports whether the composition root linked this pack.
func Enabled() bool { return enabled }

// Register records that the composition root handed this pack the command tree.
// Commands that speak this pack's kinds still register from cmd/zqk.
func Register(root *cobra.Command) {
	Enable()
	if root == nil {
		return
	}
	linkedRoot = true
}

// LinkedRoot reports whether Register received the composition root command.
func LinkedRoot() bool { return linkedRoot }
