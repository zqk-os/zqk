package ontology

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestOntologyConstantsAndTypes(t *testing.T) {
	t.Parallel()

	if ConstPrimarySystemOntology != "Primary system ontology" {
		t.Fatalf("unexpected ConstPrimarySystemOntology: %s", ConstPrimarySystemOntology)
	}
	if ConstQueryVersionContextContains != "%s WHERE n.version_context CONTAINS '%s'" {
		t.Fatalf("unexpected ConstQueryVersionContextContains: %s", ConstQueryVersionContextContains)
	}
	if ConstVersionContext != "version_context" {
		t.Fatalf("unexpected ConstVersionContext: %s", ConstVersionContext)
	}
	if ConstMissingRequiredFieldsKindOrID != "missing required fields: kind or id" {
		t.Fatalf("unexpected ConstMissingRequiredFieldsKindOrID: %s", ConstMissingRequiredFieldsKindOrID)
	}

	if ClassificationMixin != "mixin" {
		t.Fatalf("unexpected ClassificationMixin: %s", ClassificationMixin)
	}
	if ClassificationPersistedMissing != "persisted-missing" {
		t.Fatalf("unexpected ClassificationPersistedMissing: %s", ClassificationPersistedMissing)
	}
	if ClassificationThinExam != "thin-exam" {
		t.Fatalf("unexpected ClassificationThinExam: %s", ClassificationThinExam)
	}

	if ErrClassNotFound == nil || ErrInvalidInstance == nil {
		t.Fatal("expected ErrClassNotFound and ErrInvalidInstance to be non-nil")
	}
}

func TestSystemOntology_LoaderNilAndEmpty(t *testing.T) {
	t.Parallel()

	ontNil := SystemOntologyFromLoader(nil)
	if ontNil.ID != systemOntologyID {
		t.Fatalf("expected ID %q, got %q", systemOntologyID, ontNil.ID)
	}
	if len(ontNil.Classes) != 0 {
		t.Fatalf("expected 0 classes when loader is nil, got %d", len(ontNil.Classes))
	}

	tempDir := t.TempDir()
	emptyLoader := objects.NewSpecLoader(tempDir)
	ontEmpty := SystemOntologyFromLoader(emptyLoader)
	if ontEmpty.ID != systemOntologyID {
		t.Fatalf("expected ID %q, got %q", systemOntologyID, ontEmpty.ID)
	}
	if len(ontEmpty.Classes) != 0 {
		t.Fatalf("expected 0 classes when specs missing, got %d", len(ontEmpty.Classes))
	}
}

func TestSystemOntology_ClassFromSpecVariants(t *testing.T) {
	t.Parallel()

	// Variant 1: spec with nil ResolvedFields
	specMissing := &objects.Spec{
		ResolvedFields: nil,
	}
	clsMissing := classFromSpec("test_kind", specMissing)
	if clsMissing.Name != "test_kind" {
		t.Fatalf("expected class name test_kind, got %s", clsMissing.Name)
	}
	if len(clsMissing.Properties) != len(identityFieldKeys) {
		t.Fatalf("expected %d identity properties, got %d", len(identityFieldKeys), len(clsMissing.Properties))
	}

	// Variant 2: spec with untyped refs, non-ref fields, invalid map values
	specWithMix := &objects.Spec{
		ResolvedFields: map[string]any{
			"plain_text_field": map[string]any{
				objects.FieldKeyType: "string",
			},
			objects.FieldKeyRelatedObjectRefs: map[string]any{
				objects.FieldKeyType: "list",
			},
			"invalid_map_ref": "not-a-map",
			"parent_goal_ref": map[string]any{
				objects.FieldKeyType: "string",
				"required":           true,
			},
		},
	}
	clsMixed := classFromSpec("mixed_kind", specWithMix)
	if _, found := clsMixed.Properties["plain_text_field"]; found {
		t.Fatal("non-ref field plain_text_field should not be projected as graph property")
	}
	if _, found := clsMixed.Properties[objects.FieldKeyRelatedObjectRefs]; found {
		t.Fatal("untyped ref related_object_refs should be skipped")
	}
	if _, found := clsMixed.Properties["invalid_map_ref"]; found {
		t.Fatal("ref with non-map raw value should be skipped")
	}
	if prop, found := clsMixed.Properties["parent_goal_ref"]; !found {
		t.Fatal("typed ref parent_goal_ref should be projected")
	} else if !prop.Required {
		t.Fatal("parent_goal_ref was marked required, expected Required: true")
	}
}

func TestSystemOntology_IdentityPropertyVariants(t *testing.T) {
	t.Parallel()

	// Missing resolved fields
	p1 := identityProperty("custom_key", nil)
	if p1.Type != "string" || !p1.Required {
		t.Fatalf("expected string and required=true for nil spec, got type=%s required=%v", p1.Type, p1.Required)
	}

	// Key not found in resolved fields
	specMissingKey := &objects.Spec{ResolvedFields: map[string]any{}}
	p2 := identityProperty("missing_key", specMissingKey)
	if p2.Type != "string" || !p2.Required {
		t.Fatalf("expected string and required=true for missing key, got type=%s required=%v", p2.Type, p2.Required)
	}

	// Key raw value is not a map
	specNonMap := &objects.Spec{ResolvedFields: map[string]any{"raw_key": 123}}
	p3 := identityProperty("raw_key", specNonMap)
	if p3.Type != "string" || !p3.Required {
		t.Fatalf("expected string and required=true for non-map raw field, got type=%s required=%v", p3.Type, p3.Required)
	}

	// Key has explicit type and required=false
	specCustom := &objects.Spec{
		ResolvedFields: map[string]any{
			"status": map[string]any{
				objects.FieldKeyType: "enum",
				"required":           false,
			},
		},
	}
	p4 := identityProperty("status", specCustom)
	if p4.Type != "enum" || p4.Required {
		t.Fatalf("expected enum and required=false, got type=%s required=%v", p4.Type, p4.Required)
	}
}

func TestSystemOntology_PropertyFromFieldDefVariants(t *testing.T) {
	t.Parallel()

	spec := &objects.Spec{}

	// Field without explicit type (should default to "list")
	fieldMapDefaultType := map[string]any{
		"required": true,
	}
	prop1 := propertyFromFieldDef("demo_kind", "items_refs", fieldMapDefaultType, spec)
	if prop1.Type != "list" {
		t.Fatalf("expected default type 'list', got %s", prop1.Type)
	}
	if !prop1.Required {
		t.Fatal("expected Required: true")
	}

	// Field with explicit type and validation.required
	fieldMapValidationReq := map[string]any{
		objects.FieldKeyType: "string",
		"validation": map[string]any{
			"required": true,
		},
	}
	prop2 := propertyFromFieldDef("demo_kind", "single_ref", fieldMapValidationReq, spec)
	if prop2.Type != "string" {
		t.Fatalf("expected type 'string', got %s", prop2.Type)
	}
	if !prop2.Required {
		t.Fatal("expected Required: true from validation.required")
	}
}

func TestSystemOntology_HelperPredicates(t *testing.T) {
	t.Parallel()

	// isTypedGraphRefField
	if !isTypedGraphRefField("goal_ref") {
		t.Fatal("expected true for goal_ref")
	}
	if !isTypedGraphRefField("goal_refs") {
		t.Fatal("expected true for goal_refs")
	}
	if isTypedGraphRefField("description") {
		t.Fatal("expected false for description")
	}
	if isTypedGraphRefField("reference") {
		t.Fatal("expected false for reference")
	}

	// fieldType
	if fieldType(map[string]any{objects.FieldKeyType: "int"}) != "int" {
		t.Fatal("expected 'int' from fieldType")
	}
	if fieldType(map[string]any{objects.FieldKeyType: 42}) != "" {
		t.Fatal("expected empty string when type is not string")
	}
	if fieldType(map[string]any{}) != "" {
		t.Fatal("expected empty string when type is absent")
	}

	// fieldRequired
	if !fieldRequired(map[string]any{"required": true}) {
		t.Fatal("expected true when required=true")
	}
	if fieldRequired(map[string]any{"required": false}) {
		t.Fatal("expected false when required=false and no validation")
	}
	if !fieldRequired(map[string]any{"validation": map[string]any{"required": true}}) {
		t.Fatal("expected true when validation.required=true")
	}
	if fieldRequired(map[string]any{"validation": map[string]any{"required": false}}) {
		t.Fatal("expected false when validation.required=false")
	}
	if fieldRequired(map[string]any{"validation": "not-a-map"}) {
		t.Fatal("expected false when validation is not a map")
	}
}

func TestOntologyManager_EdgeCasesAndErrors(t *testing.T) {
	t.Parallel()

	mgr := NewManager()

	// Resolve unknown class
	_, err := mgr.ResolveClass("NonExistentClass")
	if err != ErrClassNotFound {
		t.Fatalf("expected ErrClassNotFound, got: %v", err)
	}

	// Validate against unknown class
	err = mgr.ValidateInstance("NonExistentClass", map[string]any{"key": "val"})
	if err != ErrClassNotFound {
		t.Fatalf("expected ErrClassNotFound for ValidateInstance with unknown class, got: %v", err)
	}

	// Register ontology with optional and required fields
	o := Ontology{
		ID:            "test-ont-001",
		Version:       "1.0",
		SchemaVersion: "2.0.0",
		Classes: map[string]Class{
			"Widget": {
				Name: "Widget",
				Properties: map[string]Property{
					"sku":   {Name: "sku", Type: "string", Required: true},
					"color": {Name: "color", Type: "string", Required: false},
				},
			},
		},
	}
	err = mgr.Register(o)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Successful validation with required field only
	err = mgr.ValidateInstance("Widget", map[string]any{"sku": "WIDGET-001"})
	if err != nil {
		t.Fatalf("expected valid validation, got: %v", err)
	}

	// Failed validation missing required field
	err = mgr.ValidateInstance("Widget", map[string]any{"color": "blue"})
	if err != ErrInvalidInstance {
		t.Fatalf("expected ErrInvalidInstance, got: %v", err)
	}
}

func TestOntologyManager_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	var wg sync.WaitGroup
	const workerCount = 10

	for i := 0; i < workerCount; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(
			fmt.Sprintf("ont_worker_%d", workerID),
			"concurrent_registration_and_validation",
		).WithWaitGroup(&wg).Start(func() error {
			className := fmt.Sprintf("Class_%d", workerID)
			ont := Ontology{
				ID: fmt.Sprintf("ont_%d", workerID),
				Classes: map[string]Class{
					className: {
						Name: className,
						Properties: map[string]Property{
							"token": {Name: "token", Type: "string", Required: true},
						},
					},
				},
			}
			regErr := mgr.Register(ont)
			if regErr != nil {
				return regErr
			}

			resolved, resErr := mgr.ResolveClass(className)
			if resErr != nil {
				return resErr
			}
			if resolved.Name != className {
				return fmt.Errorf("resolved name %s != expected %s", resolved.Name, className)
			}

			valErr := mgr.ValidateInstance(className, map[string]any{"token": "auth"})
			if valErr != nil {
				return valErr
			}
			return nil
		})
	}

	wg.Wait()
}

func TestContextualizer_Comprehensive(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	c := NewContextualizer(mgr)

	ctx := context.Background()

	// Missing both kind and id
	_, err := c.Contextualize(ctx, map[string]any{})
	if err == nil || err.Error() != ConstMissingRequiredFieldsKindOrID {
		t.Fatalf("expected %q, got %v", ConstMissingRequiredFieldsKindOrID, err)
	}

	// Missing id only
	_, err = c.Contextualize(ctx, map[string]any{"kind": "goal"})
	if err == nil || err.Error() != ConstMissingRequiredFieldsKindOrID {
		t.Fatalf("expected %q, got %v", ConstMissingRequiredFieldsKindOrID, err)
	}

	// Missing kind only
	_, err = c.Contextualize(ctx, map[string]any{"id": "G-001"})
	if err == nil || err.Error() != ConstMissingRequiredFieldsKindOrID {
		t.Fatalf("expected %q, got %v", ConstMissingRequiredFieldsKindOrID, err)
	}

	// Valid kind and id
	res, err := c.Contextualize(ctx, map[string]any{
		"kind": "goal",
		"id":   "G-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res["mapped_kind"] != "goal" {
		t.Fatalf("expected mapped_kind goal, got %v", res["mapped_kind"])
	}
	if res["mapped_id"] != "G-001" {
		t.Fatalf("expected mapped_id G-001, got %v", res["mapped_id"])
	}
	if res[objects.FieldKeyLayer] != "active_layer" {
		t.Fatalf("expected layer active_layer, got %v", res[objects.FieldKeyLayer])
	}
}
