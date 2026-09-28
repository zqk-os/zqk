package bldr_v2

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

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
