package generators

import (
	"strings"
	"testing"
)

func TestEntityGenerator_GenerateEntityCode_ValidationErrors(t *testing.T) {
	t.Parallel()

	gen := NewEntityGenerator()
	if gen == nil {
		t.Fatal("expected non-nil EntityGenerator")
	}

	// Missing PackageName
	_, err := gen.GenerateEntityCode(EntitySpec{
		PackageName: "",
		EntityName:  "MyEntity",
		Kind:        "my_kind",
	})
	if err == nil || !strings.Contains(err.Error(), "package name is required") {
		t.Errorf("expected package name error, got: %v", err)
	}

	// Whitespace PackageName
	_, err = gen.GenerateEntityCode(EntitySpec{
		PackageName: "   ",
		EntityName:  "MyEntity",
		Kind:        "my_kind",
	})
	if err == nil || !strings.Contains(err.Error(), "package name is required") {
		t.Errorf("expected package name error, got: %v", err)
	}

	// Missing EntityName
	_, err = gen.GenerateEntityCode(EntitySpec{
		PackageName: "mypkg",
		EntityName:  "",
		Kind:        "my_kind",
	})
	if err == nil || !strings.Contains(err.Error(), "entity name is required") {
		t.Errorf("expected entity name error, got: %v", err)
	}

	// Invalid Go type producing formatting error
	_, err = gen.GenerateEntityCode(EntitySpec{
		PackageName: "mypkg",
		EntityName:  "MyEntity",
		Kind:        "my_kind",
		Fields: []FieldDefinition{
			{Name: "BrokenField", Type: "broken syntax {{}}"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to format generated source code") {
		t.Errorf("expected format error for broken Go syntax, got: %v", err)
	}
}

func TestEntityGenerator_GenerateEntityCode_SuccessOptions(t *testing.T) {
	t.Parallel()

	gen := NewEntityGenerator()

	spec := EntitySpec{
		PackageName:      "testpkg",
		EntityName:       "WidgetEntity",
		Kind:             "widget",
		SchemaRef:        "2.5.0",
		IncludeAudit:     true,
		IncludeLifecycle: true,
		Fields: []FieldDefinition{
			{
				Name:    "Title",
				Type:    "string",
				JSONTag: "title_custom",
				YAMLTag: "title_custom_yaml",
				Comment: "Title of the widget",
			},
			{
				Name: "DefaultTags",
				Type: "int64",
				// No JSONTag or YAMLTag, should default to lower case "defaulttags"
			},
		},
	}

	code, err := gen.GenerateEntityCode(spec)
	if err != nil {
		t.Fatalf("unexpected error generating code: %v", err)
	}

	codeStr := string(code)

	// Verify package
	if !strings.Contains(codeStr, "package testpkg") {
		t.Errorf("missing package declaration")
	}

	// Verify struct & embeddings
	if !strings.Contains(codeStr, "type WidgetEntity struct") {
		t.Errorf("missing struct declaration")
	}
	if !strings.Contains(codeStr, "dna.BaseObject") {
		t.Errorf("missing BaseObject embedding")
	}
	if !strings.Contains(codeStr, "dna.Auditable") {
		t.Errorf("missing Auditable embedding")
	}
	if !strings.Contains(codeStr, "dna.Lifecycle") {
		t.Errorf("missing Lifecycle embedding")
	}

	// Verify field tags and comments
	if !strings.Contains(codeStr, "// Title of the widget") {
		t.Errorf("missing field comment")
	}
	if !strings.Contains(codeStr, `json:"title_custom" yaml:"title_custom_yaml"`) {
		t.Errorf("missing custom json/yaml tags, got:\n%s", codeStr)
	}
	if !strings.Contains(codeStr, `json:"defaulttags" yaml:"defaulttags"`) {
		t.Errorf("missing default json/yaml tags, got:\n%s", codeStr)
	}

	// Verify constructor and schema ref
	if !strings.Contains(codeStr, `func NewWidgetEntity(id string) (*WidgetEntity, error)`) {
		t.Errorf("missing constructor")
	}
	if !strings.Contains(codeStr, `"2.5.0"`) {
		t.Errorf("missing custom schema ref")
	}
}

func TestEntityGenerator_GenerateEntityCode_MinimalDefaults(t *testing.T) {
	t.Parallel()

	gen := NewEntityGenerator()

	// Minimal spec: no audit, no lifecycle, empty SchemaRef
	spec := EntitySpec{
		PackageName:      "minimal",
		EntityName:       "Item",
		Kind:             "item",
		SchemaRef:        "", // Should default to "1.0.0"
		IncludeAudit:     false,
		IncludeLifecycle: false,
	}

	code, err := gen.GenerateEntityCode(spec)
	if err != nil {
		t.Fatalf("unexpected error generating minimal code: %v", err)
	}

	codeStr := string(code)
	if strings.Contains(codeStr, "dna.Auditable") {
		t.Errorf("did not expect Auditable when IncludeAudit is false")
	}
	if strings.Contains(codeStr, "dna.Lifecycle") {
		t.Errorf("did not expect Lifecycle when IncludeLifecycle is false")
	}
	if !strings.Contains(codeStr, `"1.0.0"`) {
		t.Errorf("expected default schema ref 1.0.0")
	}
}
