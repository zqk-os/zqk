package object

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestPromoteTransitionTargets_OneHopOnly(t *testing.T) {
	lc := &objects.Lifecycle{
		Transitions: []objects.Transition{
			{From: "active", To: "in_progress", Auto: true},
			{From: "active", To: "complete", Auto: true},
			{From: "active", To: "paused", Manual: true},
			{From: "grooming", To: "prioritizing", Manual: true},
		},
	}
	got := promoteTransitionTargets(lc, "active")
	if _, ok := got["paused"]; !ok {
		t.Fatal("expected manual paused neighbor")
	}
	if _, ok := got["prioritizing"]; ok {
		t.Fatal("must not include non-neighbor prioritizing")
	}
	// TRACK: [REDACTED-ID] — auto-only edges are not promote targets.
	if _, ok := got["complete"]; ok {
		t.Fatal("auto-only active→complete must not be a promote candidate (overshoot)")
	}
	if _, ok := got["in_progress"]; ok {
		t.Fatal("auto-only active→in_progress is shockwave, not promote")
	}
}

func TestPromoteTransitionTargets_ManualAndAutoBothAllowed(t *testing.T) {
	lc := &objects.Lifecycle{
		Transitions: []objects.Transition{
			{From: "prioritizing", To: "active", Manual: true, Auto: false},
			{From: "active", To: "complete", Manual: true, Auto: true}, // explicit dual
		},
	}
	got := promoteTransitionTargets(lc, "active")
	if _, ok := got["complete"]; !ok {
		t.Fatal("Manual+Auto edge remains a promote candidate")
	}
}
