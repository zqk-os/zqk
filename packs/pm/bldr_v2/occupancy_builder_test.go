package bldr_v2

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
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
