package objects

import (
	"testing"
)

func TestFieldRegistry_LoadFields(t *testing.T) {
	t.Parallel()
	specLoader := NewSpecLoader("")
	registry := NewFieldRegistry(specLoader)

	err := registry.LoadFields()
	if err != nil {
		t.Fatalf("LoadFields failed: %v", err)
	}

	// Check that common fields were loaded
	commonFields, err := registry.GetCommonFields()
	if err != nil {
		t.Fatalf("GetCommonFields failed: %v", err)
	}

	if len(commonFields) == 0 {
		t.Error("expected common fields to be loaded")
	}

	// Verify common fields include expected base fields
	expectedCommonFields := []string{"id", "kind", "schema_version", "created_at", "created_by", "updated_at", "updated_by"}
	foundFields := make(map[string]bool)
	for _, field := range commonFields {
		foundFields[field.Name] = true
	}

	for _, expectedField := range expectedCommonFields {
		if !foundFields[expectedField] {
			t.Errorf("expected common field %s not found", expectedField)
		}
	}
}

func TestFieldRegistry_GetFieldsForKind(t *testing.T) {
	t.Parallel()
	specLoader := NewSpecLoader("")
	registry := NewFieldRegistry(specLoader)

	err := registry.LoadFields()
	if err != nil {
		t.Fatalf("LoadFields failed: %v", err)
	}

	// Test getting fields for a known kind
	kindFields, err := registry.GetFieldsForKind("backlog_item")
	if err != nil {
		t.Fatalf("GetFieldsForKind failed: %v", err)
	}

	if kindFields == nil {
		t.Fatal("expected kindFields to be non-nil")
	}

	if kindFields.Kind != "backlog_item" {
		t.Errorf("expected kind 'backlog_item', got %s", kindFields.Kind)
	}

	// Verify it has common fields
	if len(kindFields.CommonFields) == 0 {
		t.Error("expected common fields to be present")
	}

	// Verify all fields includes both common and specialized
	if len(kindFields.AllFields) < len(kindFields.CommonFields) {
		t.Error("expected AllFields to include at least common fields")
	}

	// Verify specialized fields exist (backlog_item should have some)
	if len(kindFields.SpecializedFields) == 0 {
		t.Log("warning: no specialized fields found for backlog_item (this might be expected)")
	}

	// extensible_object must not be in skip_specs: CLI template/create uses GetFieldsForKind.
	eo, err := registry.GetFieldsForKind("extensible_object")
	if err != nil {
		t.Fatalf("GetFieldsForKind(extensible_object): %v", err)
	}
	if eo == nil || eo.Kind != "extensible_object" {
		t.Fatalf("extensible_object kind fields: %#v", eo)
	}
	hasDomain := false
	for _, f := range eo.SpecializedFields {
		if f.Name == "domain" {
			hasDomain = true
			break
		}
	}
	if !hasDomain {
		t.Errorf("expected extensible_object specialized field domain, got %#v", eo.SpecializedFields)
	}
}

func TestFieldRegistry_GetAllKinds(t *testing.T) {
	t.Parallel()
	specLoader := NewSpecLoader("")
	registry := NewFieldRegistry(specLoader)

	err := registry.LoadFields()
	if err != nil {
		t.Fatalf("LoadFields failed: %v", err)
	}

	kinds, err := registry.GetAllKinds()
	if err != nil {
		t.Fatalf("GetAllKinds failed: %v", err)
	}

	if len(kinds) == 0 {
		t.Error("expected at least one kind to be loaded")
	}

	// Verify kinds are sorted
	for i := 1; i < len(kinds); i++ {
		if kinds[i] < kinds[i-1] {
			t.Errorf("kinds are not sorted: %s comes before %s", kinds[i], kinds[i-1])
		}
	}
}

func TestFieldRegistry_Reload(t *testing.T) {
	t.Parallel()
	specLoader := NewSpecLoader("")
	registry := NewFieldRegistry(specLoader)

	err := registry.LoadFields()
	if err != nil {
		t.Fatalf("LoadFields failed: %v", err)
	}

	// Get initial count
	//nolint:errcheck // Test helper - error acceptable
	kinds1, _ := registry.GetAllKinds()
	initialCount := len(kinds1)

	// Reload
	err = registry.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	//nolint:errcheck // Test helper - error acceptable
	// Get count after reload
	kinds2, _ := registry.GetAllKinds()
	reloadCount := len(kinds2)

	if reloadCount != initialCount {
		t.Errorf("expected same count after reload, got %d vs %d", reloadCount, initialCount)
	}
}

func TestGetGlobalFieldRegistry(t *testing.T) {
	t.Parallel()
	registry1 := GetGlobalFieldRegistry()
	registry2 := GetGlobalFieldRegistry()

	if registry1 != registry2 {
		t.Error("expected GetGlobalFieldRegistry to return the same instance")
	}
}
