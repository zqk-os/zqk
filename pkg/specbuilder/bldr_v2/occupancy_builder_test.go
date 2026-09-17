package bldr_v2

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestOccupancyBuilderExtendsWorkInterval(t *testing.T) {
	t.Parallel()
	spec := NewOccupancyBuilder().Build()
	if spec.Extends != objects.KindWorkInterval {
		t.Fatalf("occupancy extends=%q want work_interval (not work_unit)", spec.Extends)
	}
	hasOccupiable := false
	for _, tr := range spec.Traits {
		if tr == "occupiable" {
			hasOccupiable = true
			break
		}
	}
	if !hasOccupiable {
		t.Fatalf("occupancy traits=%v missing occupiable", spec.Traits)
	}
	for _, key := range []string{objects.FieldKeyClaimedBy, objects.FieldKeyClaimedAt} {
		if _, ok := spec.Fields[key]; !ok {
			t.Errorf("occupancy must declare %s", key)
		}
	}
}

func TestAgentTaskBuilderComposesOccupancy(t *testing.T) {
	t.Parallel()
	spec := NewAgentTaskBuilder().Build()
	if spec.Extends != objects.KindWorkUnit {
		t.Fatalf("agent_task extends=%q want work_unit", spec.Extends)
	}
	composed := false
	for _, name := range spec.Composes {
		if name == objects.KindOccupancy {
			composed = true
			break
		}
	}
	if !composed {
		t.Fatalf("agent_task composes=%v want occupancy", spec.Composes)
	}
	if _, ok := spec.Fields[objects.FieldKeyClaimedBy]; ok {
		t.Fatal("agent_task must not redeclare claimed_by")
	}
}
