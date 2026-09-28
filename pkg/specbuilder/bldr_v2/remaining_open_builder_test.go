package bldr_v2

import (
	"testing"

	packspec "github.com/zqk-os/zqk/packs/work/bldr_v2"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestRemainingOpenBuilderExtendsWorkInterval(t *testing.T) {
	t.Parallel()
	spec := NewRemainingOpenBuilder().Build()
	if spec.Extends != objects.KindWorkInterval {
		t.Fatalf("remaining_open extends=%q want work_interval", spec.Extends)
	}
	has := false
	for _, tr := range spec.Traits {
		if tr == objects.TraitOpenCountable {
			has = true
			break
		}
	}
	if !has {
		t.Fatalf("remaining_open traits=%v missing open_countable", spec.Traits)
	}
	if _, ok := spec.Fields[objects.FieldKeyRemainingOpenCount]; !ok {
		t.Fatal("remaining_open must declare remaining_open_count")
	}
}

func TestPriorityPlanBuilderComposesRemainingOpen(t *testing.T) {
	t.Parallel()
	spec := packspec.NewPriorityPlanBuilder().Build()
	composed := false
	for _, name := range spec.Composes {
		if name == objects.KindRemainingOpen {
			composed = true
			break
		}
	}
	if !composed {
		t.Fatalf("priority_plan composes=%v want remaining_open", spec.Composes)
	}
	if _, ok := spec.Fields[objects.FieldKeyRemainingOpenCount]; ok {
		t.Fatal("priority_plan must not redeclare remaining_open_count")
	}
}
