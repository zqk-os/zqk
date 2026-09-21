package object

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestGoalAndMilestoneLifecycle_ForwardPercentCompleteDefaults validates goal and milestone lifecycle defaults.
// Goal originated (5) -> proposed (10) and Milestone originated (5) -> not_started (15) must be
// strictly forward by percent_complete so promotion is not skipped.
func TestGoalAndMilestoneLifecycle_ForwardPercentCompleteDefaults(t *testing.T) {
	lifecyclesDir := filepath.Join("..", "..", "..", paths.ProcessInternalLifecyclesDir)
	loader := objects.NewLifecycleLoader(lifecyclesDir)

	// 1. Goal lifecycle validation
	goalLifecycle, err := loader.LoadLifecycle("goal")
	if err != nil {
		t.Fatalf("Failed to load goal lifecycle: %v", err)
	}
	if goalLifecycle == nil {
		t.Fatal("Goal lifecycle is nil")
	}
	goalDefaults := goalLifecycle.PercentComplete.DefaultByStatus
	if goalDefaults == nil {
		t.Fatal("Goal lifecycle default_by_status is nil")
	}

	originatedGoalPercent := objects.LifecycleProgressPercent("originated", goalLifecycle.PercentComplete)
	proposedGoalPercent := objects.LifecycleProgressPercent("proposed", goalLifecycle.PercentComplete)

	if proposedGoalPercent <= originatedGoalPercent {
		t.Errorf("Goal proposed percent (%.1f) must be strictly greater than originated (%.1f) to permit forward promotion",
			proposedGoalPercent, originatedGoalPercent)
	}
	if proposedGoalPercent != 10 {
		t.Errorf("Expected goal proposed percent to be 10, got %.1f", proposedGoalPercent)
	}

	// 2. Milestone lifecycle validation
	milestoneLifecycle, err := loader.LoadLifecycle("milestone")
	if err != nil {
		t.Fatalf("Failed to load milestone lifecycle: %v", err)
	}
	if milestoneLifecycle == nil {
		t.Fatal("Milestone lifecycle is nil")
	}

	originatedMsPercent := objects.LifecycleProgressPercent("originated", milestoneLifecycle.PercentComplete)
	notStartedMsPercent := objects.LifecycleProgressPercent("not_started", milestoneLifecycle.PercentComplete)

	if notStartedMsPercent <= originatedMsPercent {
		t.Errorf("Milestone not_started percent (%.1f) must be strictly greater than originated (%.1f) to permit forward promotion",
			notStartedMsPercent, originatedMsPercent)
	}
	if notStartedMsPercent != 15 {
		t.Errorf("Expected milestone not_started percent to be 15, got %.1f", notStartedMsPercent)
	}
}
