package compose

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestEvalRefuseUnknownFields_CompositionFieldsAllowed(t *testing.T) {
	t.Parallel()

	obj := map[string]any{
		"id":              "BLI-001",
		"kind":            objects.KindBacklogItem,
		"claimed_by":      "ACC-123",
		"version_context": "default",
	}

	errs := evalRefuseUnknownFields(objects.KindBacklogItem, obj, nil)
	for _, e := range errs {
		if e.Field == "claimed_by" || e.Field == "version_context" {
			t.Errorf("composition field %s was incorrectly flagged as unknown: %v", e.Field, e)
		}
	}

	obj["nonexistent_garbage_field_xyz"] = "bad"
	errs = evalRefuseUnknownFields(objects.KindBacklogItem, obj, nil)
	found := false
	for _, e := range errs {
		if e.Field == "nonexistent_garbage_field_xyz" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected nonexistent_garbage_field_xyz to be flagged as unknown, got: %v", errs)
	}
}
