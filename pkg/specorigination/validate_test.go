package specorigination

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestValidateOptions(t *testing.T) {
	t.Parallel()
	if err := ValidateOptions(Options{}); err == nil {
		t.Fatal("expected error for empty options")
	}
	if err := ValidateOptions(Options{ProjectRoot: "/tmp", Ontology: ""}); err == nil {
		t.Fatal("expected error for empty ontology")
	}
	if err := ValidateOptions(Options{ProjectRoot: "", Ontology: "x"}); err == nil {
		t.Fatal("expected error for empty project root")
	}
	if err := ValidateOptions(Options{ProjectRoot: "/tmp", Ontology: "Bad"}); err == nil {
		t.Fatal("expected error for invalid ontology stem")
	}
	if err := ValidateOptions(Options{ProjectRoot: "/tmp", Ontology: "ok_kind"}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestAuthoredFieldValidationSpecScopesInheritedFields(t *testing.T) {
	spec := &objects.Spec{
		Fields: map[string]any{
			"authored": map[string]any{objects.FieldKeyType: "string"},
		},
		ResolvedFields: map[string]any{
			"authored":  map[string]any{objects.FieldKeyType: "string"},
			"inherited": map[string]any{objects.FieldKeyType: "string"},
		},
		ResolvedTraits: []string{"base_object_traits"},
	}

	scoped := authoredFieldValidationSpec(spec)

	if len(scoped.ResolvedFields) != 1 || scoped.ResolvedFields["authored"] == nil {
		t.Fatalf("scoped fields = %#v, want only authored field", scoped.ResolvedFields)
	}
	if len(spec.ResolvedFields) != 2 {
		t.Fatalf("source spec mutated: %#v", spec.ResolvedFields)
	}
	if len(scoped.ResolvedTraits) != 1 || scoped.ResolvedTraits[0] != "base_object_traits" {
		t.Fatalf("resolved traits not preserved: %#v", scoped.ResolvedTraits)
	}
}
