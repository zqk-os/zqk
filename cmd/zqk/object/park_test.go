package object

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestLifecycleAllowsTransition_WildcardAndExact(t *testing.T) {
	life := &objects.Lifecycle{
		Transitions: []objects.Transition{
			{From: "exploring", To: "deferred", Manual: true},
			{From: "*", To: "archived", Manual: true},
		},
	}
	if !lifecycleAllowsTransition(life, "exploring", "deferred") {
		t.Fatal("expected exploring→deferred")
	}
	if !lifecycleAllowsTransition(life, "rejected", "archived") {
		t.Fatal("expected *→archived from rejected")
	}
	if lifecycleAllowsTransition(life, "rejected", "deferred") {
		t.Fatal("rejected→deferred must be false without an edge")
	}
	if lifecycleAllowsTransition(life, "exploring", "planned") {
		t.Fatal("missing edge must be false")
	}
}
