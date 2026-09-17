package builders_test

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2" // Import to trigger builder registration (uses builders.CurrentBuilderPackage)
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const generatorTestSpecFileExtYAML = ".yaml"

func TestAuditableBuilder(t *testing.T) {
	t.Parallel()
	registry := builders.GetGlobalRegistry()

	// Get builder from global registry
	builder, err := registry.GetBuilder("auditable", builders.CurrentBuilderVersion)
	if err != nil {
		t.Fatalf("Failed to get builder: %v", err)
	}

	// Verify builder metadata
	if builder.GetVersion() != builders.CurrentBuilderVersion {
		t.Errorf("Expected version %q, got %q", builders.CurrentBuilderVersion, builder.GetVersion())
	}
	if builder.GetOntology() != "auditable" {
		t.Errorf("Expected ontology 'auditable', got '%s'", builder.GetOntology())
	}

	// Build spec
	spec := builder.Build()
	if spec == nil {
		t.Fatal("Build() returned nil")
	}

	// Verify spec structure
	if spec.Ontology != "auditable" {
		t.Errorf("Expected ontology 'auditable', got '%s'", spec.Ontology)
	}
	if spec.SchemaVersion != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema version %q, got '%s'", objects.DefaultSchemaVersion, spec.SchemaVersion)
	}
	if spec.Extends != "null" {
		t.Errorf("Expected extends 'null', got '%s'", spec.Extends)
	}
	if len(spec.Fields) == 0 {
		t.Error("Expected fields to be populated")
	}

	// Verify created_at field exists
	if _, ok := spec.Fields["created_at"]; !ok {
		t.Error("Expected 'created_at' field to exist")
	}
}

func TestBaseObjectBuilder(t *testing.T) {
	t.Parallel()
	registry := builders.GetGlobalRegistry()

	// Get builder from global registry
	builder, err := registry.GetBuilder("base_object", "v2_0_0")
	if err != nil {
		t.Fatalf("Failed to get builder: %v", err)
	}

	// Verify builder metadata
	if builder.GetVersion() != builders.CurrentBuilderVersion {
		t.Errorf("Expected version %q, got %q", builders.CurrentBuilderVersion, builder.GetVersion())
	}
	if builder.GetOntology() != "base_object" {
		t.Errorf("Expected ontology 'base_object', got '%s'", builder.GetOntology())
	}

	// Build spec
	spec := builder.Build()
	if spec == nil {
		t.Fatal("Build() returned nil")
	}

	// Verify spec structure
	if spec.Ontology != "base_object" {
		t.Errorf("Expected ontology 'base_object', got '%s'", spec.Ontology)
	}
	if spec.Extends != "auditable" {
		t.Errorf("Expected extends 'auditable', got '%s'", spec.Extends)
	}
	if len(spec.Traits) == 0 {
		t.Error("Expected traits to be populated")
	}
	if len(spec.Fields) == 0 {
		t.Error("Expected fields to be populated")
	}
}

func TestVersionedBuilderRegistry(t *testing.T) {
	t.Parallel()
	registry := builders.NewVersionedBuilderRegistry()

	// Get builders from global registry to register in test registry
	globalRegistry := builders.GetGlobalRegistry()
	auditableBuilder, err := globalRegistry.GetBuilder("auditable", "v2_0_0")
	if err != nil {
		t.Fatalf("Failed to get auditable builder: %v", err)
	}
	baseObjectBuilder, err := globalRegistry.GetBuilder("base_object", "v2_0_0")
	if err != nil {
		t.Fatalf("Failed to get base_object builder: %v", err)
	}

	// Register builders
	registry.Register(auditableBuilder)
	registry.Register(baseObjectBuilder)

	// Get builder
	builder, err := registry.GetBuilder("auditable", builders.CurrentBuilderVersion)
	if err != nil {
		t.Fatalf("Failed to get builder: %v", err)
	}

	if builder.GetOntology() != "auditable" {
		t.Errorf("Expected ontology 'auditable', got '%s'", builder.GetOntology())
	}

	// Get latest version
	latest, err := registry.GetLatestVersion("auditable")
	if err != nil {
		t.Fatalf("Failed to get latest version: %v", err)
	}
	if latest != builders.CurrentBuilderVersion {
		t.Errorf("Expected latest version %q, got %q", builders.CurrentBuilderVersion, latest)
	}

	// Get builder that doesn't exist
	_, err = registry.GetBuilder("nonexistent", builders.CurrentBuilderVersion)
	if err == nil {
		t.Error("Expected error for nonexistent builder")
	}
}

func TestGlobalRegistry(t *testing.T) {
	t.Parallel()
	registry := builders.GetGlobalRegistry()

	// Verify auditable builder is registered
	builder, err := registry.GetBuilder("auditable", builders.CurrentBuilderVersion)
	if err != nil {
		t.Fatalf("Failed to get auditable builder: %v", err)
	}

	spec := builder.Build()
	if spec.Ontology != "auditable" {
		t.Errorf("Expected ontology 'auditable', got '%s'", spec.Ontology)
	}

	// Verify base_object builder is registered
	builder2, err := registry.GetBuilder("base_object", "v2_0_0")
	if err != nil {
		t.Fatalf("Failed to get base_object builder: %v", err)
	}

	spec2 := builder2.Build()
	if spec2.Ontology != "base_object" {
		t.Errorf("Expected ontology 'base_object', got '%s'", spec2.Ontology)
	}
}

func TestSpecGenerator(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	generator := builders.NewSpecGenerator(tmpDir)

	// Ensure output directory exists
	if err := generator.EnsureOutputDir(); err != nil {
		t.Fatalf("Failed to ensure output directory: %v", err)
	}

	// Generate spec
	if err := generator.GenerateLatestSpec("auditable"); err != nil {
		t.Fatalf("Failed to generate spec: %v", err)
	}

	// Verify file was created
	expectedFile := filepath.Join(tmpDir, "auditable.yaml")
	if _, err := fileutil.Stat(expectedFile); fileutil.IsNotExist(err) {
		t.Errorf("Expected file %s was not created", expectedFile)
	}
}

func TestSpecGeneratorGenerateAll(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	generator := builders.NewSpecGenerator(tmpDir)

	// Ensure output directory exists
	if err := generator.EnsureOutputDir(); err != nil {
		t.Fatalf("Failed to ensure output directory: %v", err)
	}

	// Generate all specs
	if err := generator.GenerateAllSpecs(); err != nil {
		t.Fatalf("Failed to generate all specs: %v", err)
	}

	// Get all ontologies from registry to verify all specs were generated
	registry := builders.GetGlobalRegistry()
	ontologies := registry.GetAllOntologies()

	if len(ontologies) == 0 {
		t.Fatal("No ontologies found in registry")
	}

	t.Logf("Generated %d specs", len(ontologies))

	// Verify all spec files were created
	for _, ontology := range ontologies {
		filePath := filepath.Join(tmpDir, ontology+generatorTestSpecFileExtYAML)
		if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
			t.Errorf("Expected file %s was not created for ontology %s", filePath, ontology)
		}
	}

	// Verify at least a few key specs exist
	expectedFiles := []string{"auditable.yaml", "base_object.yaml", "account.yaml"}
	for _, filename := range expectedFiles {
		filePath := filepath.Join(tmpDir, filename)
		if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
			t.Errorf("Expected key file %s was not created", filePath)
		}
	}
}
