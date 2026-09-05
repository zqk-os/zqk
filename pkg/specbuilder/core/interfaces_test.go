package core

import (
	"testing"
)

// Wire values match pkg/objects field keys; core cannot import pkg/objects (import cycle via specbuilder/yaml).
const (
	testObjectFieldKeyID   = "id"
	testObjectFieldKeyName = "name"
)

// Test interfaces with minimal implementations to validate the pattern

// TestSpec is a minimal spec implementation for testing
type TestSpec struct {
	Name string
}

func (s TestSpec) Validate() error {
	if s.Name == emptyValue {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	return nil
}

func (s TestSpec) GetName() string {
	return s.Name
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// TestArtifact is a minimal artifact for testing
type TestArtifact struct {
	Name string
}

// TestBuilder is a minimal builder implementation
type TestBuilder struct {
	artifact *TestArtifact
}

func NewTestBuilder() *TestBuilder {
	return &TestBuilder{
		artifact: &TestArtifact{},
	}
}

func (b *TestBuilder) SetName(name string) *TestBuilder {
	b.artifact.Name = name
	return b
}

func (b *TestBuilder) Build() TestArtifact {
	return *b.artifact
}

// TestBuilderFactory creates builders from specs
type TestBuilderFactory struct{}

func (f *TestBuilderFactory) CreateBuilder(spec TestSpec) Builder[TestArtifact] {
	builder := NewTestBuilder()
	builder.SetName(spec.Name)
	return builder
}

// Test the core interfaces work together
func TestCoreInterfaces(t *testing.T) {
	t.Parallel()
	// Create a spec
	spec := TestSpec{Name: "test-spec"}

	// Validate spec
	if err := spec.Validate(); err != nil {
		t.Fatalf("Spec validation failed: %v", err)
	}

	// Create builder from spec using factory
	factory := &TestBuilderFactory{}
	builder := factory.CreateBuilder(spec)

	// Build artifact
	artifact := builder.Build()

	// Verify artifact
	if artifact.Name != "test-spec" {
		t.Errorf("Expected artifact name 'test-spec', got '%s'", artifact.Name)
	}

	// Test GetName
	if spec.GetName() != "test-spec" {
		t.Errorf("Expected spec name 'test-spec', got '%s'", spec.GetName())
	}
}

// Test validation error
func TestSpecValidation(t *testing.T) {
	t.Parallel()
	spec := TestSpec{Name: ""}

	err := spec.Validate()
	if err == nil {
		t.Error("Expected validation error for empty name")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}

	if validationErr.Field != "name" {
		t.Errorf("Expected field 'name', got '%s'", validationErr.Field)
	}
}

// TestConstants is a minimal constants implementation for testing
type TestConstants struct {
	ontology    string
	fieldMap    map[string]string
	packageName string
}

func NewTestConstants(ontology, packageName string, fieldMap map[string]string) *TestConstants {
	return &TestConstants{
		ontology:    ontology,
		fieldMap:    fieldMap,
		packageName: packageName,
	}
}

func (c *TestConstants) GetFieldName(fieldName string) string {
	return c.fieldMap[fieldName]
}

func (c *TestConstants) GetAllFieldNames() map[string]string {
	return c.fieldMap
}

func (c *TestConstants) GetPackage() string {
	return c.packageName
}

func (c *TestConstants) GetOntology() string {
	return c.ontology
}

// TestConstantsGenerator is a minimal constants generator for testing
type TestConstantsGenerator struct{}

func (g *TestConstantsGenerator) GenerateConstants(spec TestSpec) (Constants, error) {
	fieldMap := map[string]string{
		testObjectFieldKeyName: "FieldName",
	}
	return NewTestConstants(spec.Name, "test", fieldMap), nil
}

func (g *TestConstantsGenerator) GenerateConstantsFromSpecs(specs []TestSpec) ([]Constants, error) {
	constants := make([]Constants, 0, len(specs))
	for _, spec := range specs {
		c, err := g.GenerateConstants(spec)
		if err != nil {
			return nil, err
		}
		constants = append(constants, c)
	}
	return constants, nil
}

// Test constants interface
func TestConstantsInterface(t *testing.T) {
	t.Parallel()
	fieldMap := map[string]string{
		testObjectFieldKeyID:   "FieldId",
		testObjectFieldKeyName: "FieldName",
	}
	constants := NewTestConstants("test", "testpkg", fieldMap)

	if constants.GetOntology() != "test" {
		t.Errorf("Expected ontology 'test', got '%s'", constants.GetOntology())
	}

	if constants.GetPackage() != "testpkg" {
		t.Errorf("Expected package 'testpkg', got '%s'", constants.GetPackage())
	}

	if constants.GetFieldName("id") != "FieldId" {
		t.Errorf("Expected 'FieldId', got '%s'", constants.GetFieldName("id"))
	}

	allFields := constants.GetAllFieldNames()
	if len(allFields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(allFields))
	}
}

// Test constants generator integration
func TestConstantsGeneratorIntegration(t *testing.T) {
	t.Parallel()
	spec := TestSpec{Name: "test-spec"}
	generator := &TestConstantsGenerator{}

	constants, err := generator.GenerateConstants(spec)
	if err != nil {
		t.Fatalf("Failed to generate constants: %v", err)
	}

	if constants.GetOntology() != "test-spec" {
		t.Errorf("Expected ontology 'test-spec', got '%s'", constants.GetOntology())
	}

	if constants.GetFieldName("name") != "FieldName" {
		t.Errorf("Expected 'FieldName', got '%s'", constants.GetFieldName("name"))
	}
}
