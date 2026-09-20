package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestMergeObjectFieldsIntoRoutingMeta_flagsOverrideObject(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCurrentPhase: "c1_scope",
		objects.FieldKeyFlowVariant:  "scheduler_fast",
	}
	meta := map[string]any{}
	mergeObjectFieldsIntoRoutingMeta(obj, "c4_act", "", meta)
	if meta["effective_current_phase"] != "c4_act" || meta["current_phase_source"] != "flag" {
		t.Fatalf("current_phase: %+v", meta)
	}
	if meta["effective_flow_variant"] != "scheduler_fast" || meta["flow_variant_source"] != "object" {
		t.Fatalf("flow_variant: %+v", meta)
	}
}

func TestMergeObjectFieldsIntoRoutingMeta_objectOnly(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCurrentPhase: "c5_verify",
		objects.FieldKeyFlowVariant:  "coordinator_async",
	}
	meta := map[string]any{}
	mergeObjectFieldsIntoRoutingMeta(obj, "", "", meta)
	if meta["effective_current_phase"] != "c5_verify" || meta["current_phase_source"] != "object" {
		t.Fatalf("current_phase: %+v", meta)
	}
	if meta["effective_flow_variant"] != "coordinator_async" || meta["flow_variant_source"] != "object" {
		t.Fatalf("flow_variant: %+v", meta)
	}
}
