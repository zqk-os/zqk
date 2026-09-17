package bldr_v2_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"
)

func TestPriorityPlanBuilder_CodeLocation(t *testing.T) {
	b := bldr_v2.NewPriorityPlanBuilder()
	spec := b.Build()

	if _, ok := spec.Fields["code_location"]; !ok {
		t.Errorf("Expected 'code_location' field on priority_plan spec")
	}
}
