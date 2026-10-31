package validation

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TRACK: RIS-1793446317704936000-61dc49df / BLI-1793470168999105000-a0236cca —
// YAML [] must not fail StrictMode when confidential-filtered ResolvedFields omit epic_refs.
func TestStrictMode_YAMLEmptyEpicRefsOnPriorityPlan(t *testing.T) {
	t.Parallel()
	raw := []byte(`id: PRI-TEST-EMPTY-EPIC
kind: priority_plan
status: originated
title: test empty epic residue
schema_version: "2.0.0"
namespace_id: zqk:kernel
version_context: default
epic_refs: []
persona_refs:
  - PER-DEFAULT-OPERATOR
`)
	var obj map[string]any
	if err := yaml.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	t.Logf("epic_refs type=%T val=%#v", obj["epic_refs"], obj["epic_refs"])
	gv := NewGoValidator()
	opts := &ValidationOptions{StrictMode: true}
	res, err := gv.Validate(context.Background(), obj, objects.KindPriorityPlan, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Errors {
		if e.Field == "epic_refs" || (e.Rule == "unknown_field" && e.Field == "epic_refs") {
			t.Fatalf("empty YAML epic_refs must not fail StrictMode: %+v (all=%v)", e, res.Errors)
		}
	}
}
