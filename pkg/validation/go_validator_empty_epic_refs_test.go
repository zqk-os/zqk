package validation

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TRACK: RIS-1793446317704936000-61dc49df — validateUnknownFields must ignore empty
// unknown residue even when stripEmptyUnknownFieldResidue did not delete the key.
func TestValidateUnknownFields_SkipsEmptyEpicRefsResidue(t *testing.T) {
	t.Parallel()
	spec := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyTitle:  map[string]any{"type": "string"},
			objects.FieldKeyStatus: map[string]any{"type": "string"},
		},
	}
	obj := map[string]any{
		objects.FieldKeyTitle:  "plan",
		objects.FieldKeyStatus: "conceptual",
		"epic_refs":            []any{},
		"stray_populated":      "keep-me",
	}
	gv := NewGoValidator()
	errs := gv.validateUnknownFields("priority_plan", obj, spec)
	if len(errs) != 1 || errs[0].Field != "stray_populated" {
		t.Fatalf("expected sole unknown_field on stray_populated, got %v", errs)
	}
}

func TestStripEmptyUnknownFieldResidue_DropsEmptyEpicRefs(t *testing.T) {
	t.Parallel()
	spec := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyTitle: map[string]any{"type": "string"},
		},
	}
	obj := map[string]any{
		objects.FieldKeyTitle: "plan",
		"epic_refs":           []any{},
		"stray_populated":     "x",
	}
	stripEmptyUnknownFieldResidue(objects.KindPriorityPlan, obj, spec)
	if _, ok := obj["epic_refs"]; ok {
		t.Fatal("empty epic_refs residue must be stripped")
	}
	if _, ok := obj["stray_populated"]; !ok {
		t.Fatal("non-empty unknown fields must remain for StrictMode to reject")
	}
}
