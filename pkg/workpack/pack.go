// Package workpack is the included planning and verification pack.
// The composition root links it by default. A root that passes the build tag
// zqk_omit_workpack leaves this package out of the binary.
package workpack

import (
	"github.com/spf13/cobra"

	packbldr "github.com/zqk-os/zqk/packs/work/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/work/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/work/bldr_v2"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// Name is the pack id the composition root links.
	Name = "work"
	// ToolDir is the instance builder tool directory for this pack.
	// Generated builders and enums are written beside it.
	ToolDir = "packs/work/instance_builders"
	// SpecDir is the object spec directory this pack owns.
	// pkg/objects registers the same relative path when the omit tag is off.
	SpecDir = "packs/work/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	// pkg/objects registers the same relative path when the omit tag is off.
	LifecycleDir = "packs/work/lifecycles"
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
		objects.KindMission,
		objects.KindVision,
		objects.KindStrategicContext,
		objects.KindStrategicPlan,
		objects.KindRoadmap,
		objects.KindMilestone,
		objects.KindEpic,
		objects.KindBacklogItem,
		objects.KindWorkstream,
		objects.KindPriorityPlan,
		objects.KindRiskBlocker,
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
		packbldr.NewMissionInstanceBuilder,
		packbldr.NewVisionInstanceBuilder,
		packbldr.NewStrategicContextInstanceBuilder,
		packbldr.NewStrategicPlanInstanceBuilder,
		packbldr.NewRoadmapInstanceBuilder,
		packbldr.NewMilestoneInstanceBuilder,
		packbldr.NewEpicInstanceBuilder,
		packbldr.NewBacklogItemInstanceBuilder,
		packbldr.NewWorkstreamInstanceBuilder,
		packbldr.NewPriorityPlanInstanceBuilder,
		packbldr.NewRiskBlockerInstanceBuilder,
	}
)

// BuilderCount is the number of generated constructors this pack links.
func BuilderCount() int { return len(builders) }

// Enable verifies this pack's spec and lifecycle files, records those spec
// paths, and marks the pack linked. A failed verification leaves the pack disabled.
func Enable() {
	recorded, err := verifyOwnedKinds()
	if err != nil {
		enabled = false
		verifiedSpecs = nil
		verifyErr = err
		return
	}
	verifiedSpecs = recorded
	verifyErr = nil
	enabled = true
}

// Enabled reports whether the composition root linked this pack.
func Enabled() bool { return enabled }

// Register records that the composition root handed this pack the command tree.
// The composition root attaches this pack's kind commands through the object registrar.
func Register(root *cobra.Command) {
	Enable()
	if root == nil {
		return
	}
	linkedRoot = true
}

// LinkedRoot reports whether Register received the composition root command.
func LinkedRoot() bool { return linkedRoot }
