package systemcheck

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestCheckRegistration_missingID(t *testing.T) {
	obj := &parser.ParsedObject{
		ID:   "",
		Kind: "policy",
		Properties: map[string]any{
			objects.FieldKeyCreatedBy: "ACC-1",
			objects.FieldKeyUpdatedBy: "ACC-1",
		},
	}
	issues := CheckRegistration(obj, "policy")
	found := false
	for _, iss := range issues {
		if iss.Category == "registration" && iss.Tier == 1 && iss.Message == "Missing required field: id" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing id issue, got %#v", issues)
	}
}

func TestLooksHandCASMaterialized(t *testing.T) {
	hand := &parser.ParsedObject{Properties: map[string]any{}}
	if !LooksHandCASMaterialized(hand) {
		t.Fatal("expected hand-CAS when provenance missing")
	}
	cli := &parser.ParsedObject{Properties: map[string]any{
		objects.FieldKeyCreatedBy: "ACC-1",
	}}
	if LooksHandCASMaterialized(cli) {
		t.Fatal("expected not hand-CAS when created_by set")
	}
}

func TestCheckLifecycle_missingStatus(t *testing.T) {
	obj := &parser.ParsedObject{Properties: map[string]any{}}
	issues := CheckLifecycle(obj, "policy")
	if len(issues) == 0 {
		t.Fatal("expected missing status issue")
	}
	if issues[0].Category != objects.KindLifecycle {
		t.Fatalf("category=%q", issues[0].Category)
	}
}
