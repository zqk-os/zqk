package object

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_lifecycle_v1"
)

func TestIsNonProgressLifecycleProbeCandidate_AllowsSuccessTerminals(t *testing.T) {
	t.Parallel()

	successMeta := objects.Status{Terminal: true}
	for _, status := range []string{
		objects.ObjectStatusComplete,
		objects.ObjectStatusCompleted,
		objects.ObjectStatusImplemented,
		objects.ObjectStatusSuccess,
	} {
		if isNonProgressLifecycleProbeCandidate(status, successMeta) {
			t.Fatalf("expected success terminal %q to remain probeable for promote", status)
		}
	}

	if !isNonProgressLifecycleProbeCandidate(objects.ObjectStatusRejected, objects.Status{Terminal: true}) {
		t.Fatal("expected rejected terminal to be skipped")
	}
	if !isNonProgressLifecycleProbeCandidate("archived", objects.Status{Archive: true, Terminal: true}) {
		t.Fatal("expected archive terminal to be skipped")
	}
	if !isNonProgressLifecycleProbeCandidate(objects.ObjectStatusError, objects.Status{System: true}) {
		t.Fatal("expected system error status to be skipped")
	}
}

func TestPromoteProbeOrderSkipsCurrentStatus(t *testing.T) {
	// * → draft recovery edges must not satisfy promote when already draft
	// (otherwise active is never probed).
	t.Parallel()
	lc := &objects.Lifecycle{
		Statuses: []objects.Status{
			{Value: "draft"},
			{Value: "active"},
			{Value: "error", System: true},
		},
		Transitions: []objects.Transition{
			{From: "draft", To: "active", Manual: true},
			{From: "*", To: "draft", Manual: true},
			{From: "*", To: "error", Manual: false},
		},
		PercentComplete: objects.PercentCompleteConfig{
			DefaultByStatus: map[string]any{"draft": 0, "active": 50, "error": 0},
		},
	}
	neighbors := promoteTransitionTargets(lc, "draft")
	if _, ok := neighbors["active"]; !ok {
		t.Fatal("expected active neighbor")
	}
	currentPercent := getPercentComplete("draft", lc.PercentComplete)
	var probed []string
	for _, cand := range []string{"draft", "error", "active"} {
		if _, ok := neighbors[cand]; !ok {
			continue
		}
		if cand == "draft" {
			continue
		}
		pct := getPercentComplete(cand, lc.PercentComplete)
		if pct <= currentPercent && !isSuccessLifecycleTerminal(cand) {
			continue
		}
		if isNonProgressLifecycleProbeCandidate(cand, lifecycleStatusByValue(lc, cand)) {
			continue
		}
		probed = append(probed, cand)
	}
	if len(probed) != 1 || probed[0] != "active" {
		t.Fatalf("expected probe [active], got %v", probed)
	}
}

func TestPromoteProbeOrderForwardOnlyFromActive(t *testing.T) {
	t.Parallel()
	lc := &objects.Lifecycle{
		Statuses: []objects.Status{
			{Value: "draft"},
			{Value: "active"},
			{Value: "complete", Terminal: true},
		},
		Transitions: []objects.Transition{
			{From: "active", To: "complete", Manual: true},
			{From: "*", To: "draft", Manual: true},
		},
		PercentComplete: objects.PercentCompleteConfig{
			DefaultByStatus: map[string]any{"draft": 0, "active": 50, "complete": 100},
		},
	}
	neighbors := promoteTransitionTargets(lc, "active")
	currentPercent := getPercentComplete("active", lc.PercentComplete)
	var probed []string
	for _, cand := range []string{"draft", "complete"} {
		if _, ok := neighbors[cand]; !ok {
			continue
		}
		if cand == "active" {
			continue
		}
		pct := getPercentComplete(cand, lc.PercentComplete)
		if pct <= currentPercent && !isSuccessLifecycleTerminal(cand) {
			continue
		}
		if isNonProgressLifecycleProbeCandidate(cand, lifecycleStatusByValue(lc, cand)) {
			continue
		}
		probed = append(probed, cand)
	}
	if len(probed) != 1 || probed[0] != "complete" {
		t.Fatalf("expected forward probe [complete] not draft, got %v", probed)
	}
}

// priority_plan: demote orders by percent_complete; missing in_progress sorted as 0 and
// inverted active ("calculated"→75) → in_progress into a false "demotion".
// TRACK: [REDACTED-ID]
func TestPriorityPlanPercentCompleteOrdersExecutionLockedAfterActive(t *testing.T) {
	t.Parallel()
	lc := bldr_lifecycle_v1.NewPriorityPlanLifecycleBuilder().Build()
	if lc == nil {
		t.Fatal("build priority_plan lifecycle: nil")
	}
	pc := lc.PercentComplete
	active := getPercentComplete("active", pc)
	inProgress := getPercentComplete("in_progress", pc)
	complete := getPercentComplete("complete", pc)
	if !(active < inProgress && inProgress < complete) {
		t.Fatalf("expected active(%v) < in_progress(%v) < complete(%v)", active, inProgress, complete)
	}
	// Regression: omitted in_progress must not sort as 0 under active.
	missing := objects.PercentCompleteConfig{
		DefaultByStatus: map[string]any{"active": "calculated", "complete": 100},
	}
	if getPercentComplete("in_progress", missing) >= getPercentComplete("active", missing) {
		t.Fatal("sanity: missing in_progress should sort at 0 below calculated active")
	}
}
