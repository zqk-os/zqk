package bldr_instance_v1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	lifecycleenum "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/lifecycle"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestLifecycleInstanceBuilder_Build(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	statuses := []any{
		map[string]any{
			"value":   "draft",
			"display": "Draft",
			"initial": true,
		},
		map[string]any{
			"value":    "active",
			"display":  "Active",
			"terminal": true,
		},
	}

	builder.SetID("LIFECYCLE-test_kind-v1_0_0")
	builder.SetObjectType("test_kind")
	builder.SetSourceType("internal")
	builder.SetField("statuses", statuses)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Verify required fields
	if instance["id"] != "LIFECYCLE-test_kind-v1_0_0" {
		t.Errorf("Expected id 'LIFECYCLE-test_kind-v1_0_0', got '%v'", instance["id"])
	}
	if instance["kind"] != "lifecycle" {
		t.Errorf("Expected kind 'lifecycle', got '%v'", instance["kind"])
	}
	if instance["schema_version"] != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema_version %q, got '%v'", objects.DefaultSchemaVersion, instance["schema_version"])
	}
	if instance["object_type"] != "test_kind" {
		t.Errorf("Expected object_type 'test_kind', got '%v'", instance["object_type"])
	}
	if instance["source_type"] != "internal" {
		t.Errorf("Expected source_type 'internal', got '%v'", instance["source_type"])
	}

	// Verify statuses
	statusesList, ok := instance["statuses"].([]any)
	if !ok {
		t.Fatalf("Expected statuses to be []any, got %T", instance["statuses"])
	}
	if len(statusesList) != 2 {
		t.Errorf("Expected 2 statuses, got %d", len(statusesList))
	}

	// Verify timestamps are set
	if _, ok := instance["created_at"]; !ok {
		t.Error("Expected created_at to be set")
	}
	if _, ok := instance["updated_at"]; !ok {
		t.Error("Expected updated_at to be set")
	}
}

func TestLifecycleInstanceBuilder_SetObjectType(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)
	builder.SetObjectType("backlog_item")
	builder.SetID("LIFECYCLE-test-v1_0_0")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if instance["object_type"] != "backlog_item" {
		t.Errorf("Expected object_type 'backlog_item', got '%v'", instance["object_type"])
	}
}

func TestLifecycleInstanceBuilder_SetSourceType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		sourceType string
		wantErr    bool
	}{
		{"built-in", "built-in", false},
		{"internal", "internal", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)
			builder.SetID("LIFECYCLE-test-v1_0_0")
			builder.SetSourceType(lifecycleenum.SourceType(tt.sourceType))
			instance, err := builder.Build()

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Build() failed: %v", err)
			}

			if instance["source_type"] != tt.sourceType {
				t.Errorf("Expected source_type '%s', got '%v'", tt.sourceType, instance["source_type"])
			}
		})
	}
}

func TestLifecycleInstanceBuilder_SetExtends(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)
	builder.SetID("LIFECYCLE-test-v1_0_0")
	builder.SetExtends("base_lifecycle")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if instance["extends"] != "base_lifecycle" {
		t.Errorf("Expected extends 'base_lifecycle', got '%v'", instance["extends"])
	}
}

func TestLifecycleInstanceBuilder_SetStatusMapping(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	statusMapping := map[string]any{
		"proposed": "draft",
		"approved": "active",
	}

	builder.SetID("LIFECYCLE-test-v1_0_0")
	builder.SetStatusMapping(statusMapping)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	mapping, ok := instance["status_mapping"].(map[string]any)
	if !ok {
		t.Fatalf("Expected status_mapping to be map[string]any, got %T", instance["status_mapping"])
	}

	if mapping["proposed"] != "draft" {
		t.Errorf("Expected status_mapping['proposed'] = 'draft', got '%v'", mapping["proposed"])
	}
	if mapping["approved"] != "active" {
		t.Errorf("Expected status_mapping['approved'] = 'active', got '%v'", mapping["approved"])
	}
}

func TestLifecycleInstanceBuilder_SetStatuses(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	statuses := []any{
		map[string]any{
			"value":   "draft",
			"display": "Draft",
			"initial": true,
		},
		map[string]any{
			"value":    "active",
			"display":  "Active",
			"terminal": true,
		},
		map[string]any{
			"value":    "archived",
			"display":  "Archived",
			"archive":  true,
			"terminal": true,
		},
	}

	builder.SetID("LIFECYCLE-test-v1_0_0")
	builder.SetField("statuses", statuses)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	statusesList, ok := instance["statuses"].([]any)
	if !ok {
		t.Fatalf("Expected statuses to be []any, got %T", instance["statuses"])
	}

	if len(statusesList) != 3 {
		t.Errorf("Expected 3 statuses, got %d", len(statusesList))
	}

	// Verify first status
	status1, ok := statusesList[0].(map[string]any)
	if !ok {
		t.Fatalf("Expected status to be map[string]any, got %T", statusesList[0])
	}
	if status1["value"] != "draft" {
		t.Errorf("Expected first status value 'draft', got '%v'", status1["value"])
	}
	if status1["initial"] != true {
		t.Errorf("Expected first status initial=true, got '%v'", status1["initial"])
	}
}

func TestLifecycleInstanceBuilder_SetTransitions(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	transitions := []any{
		map[string]any{
			"from":        "draft",
			"to":          "active",
			"description": "Activate",
			"manual":      true,
		},
		map[string]any{
			"from":        "active",
			"to":          "archived",
			"description": "Archive",
			"manual":      true,
		},
	}

	builder.SetID("LIFECYCLE-test-v1_0_0")
	builder.SetField("transitions", transitions)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	transitionsList, ok := instance["transitions"].([]any)
	if !ok {
		t.Fatalf("Expected transitions to be []any, got %T", instance["transitions"])
	}

	if len(transitionsList) != 2 {
		t.Errorf("Expected 2 transitions, got %d", len(transitionsList))
	}

	// Verify first transition
	transition1, ok := transitionsList[0].(map[string]any)
	if !ok {
		t.Fatalf("Expected transition to be map[string]any, got %T", transitionsList[0])
	}
	if transition1["from"] != "draft" {
		t.Errorf("Expected first transition from='draft', got '%v'", transition1["from"])
	}
	if transition1["to"] != "active" {
		t.Errorf("Expected first transition to='active', got '%v'", transition1["to"])
	}
}

func TestLifecycleInstanceBuilder_SetPercentComplete(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	percentComplete := map[string]any{
		"method": "status_defaults",
		"default_by_status": map[string]any{
			"draft":    0,
			"active":   50,
			"complete": 100,
		},
	}

	builder.SetID("LIFECYCLE-test-v1_0_0")
	builder.SetPercentComplete(percentComplete)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	pc, ok := instance["percent_complete"].(map[string]any)
	if !ok {
		t.Fatalf("Expected percent_complete to be map[string]any, got %T", instance["percent_complete"])
	}

	if pc["method"] != "status_defaults" {
		t.Errorf("Expected percent_complete method 'status_defaults', got '%v'", pc["method"])
	}

	defaults, ok := pc["default_by_status"].(map[string]any)
	if !ok {
		t.Fatalf("Expected default_by_status to be map[string]any, got %T", pc["default_by_status"])
	}

	if defaults["draft"] != 0 {
		t.Errorf("Expected default_by_status['draft'] = 0, got '%v'", defaults["draft"])
	}
}

func TestLifecycleInstanceBuilder_LoadFromYAML(t *testing.T) {
	t.Parallel()
	// Create a temporary YAML file
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "test-lifecycle.yaml")

	yamlContent := `id: LIFECYCLE-test_kind-v1_0_0
kind: lifecycle
schema_version: "` + objects.DefaultSchemaVersion + `"
object_type: test_kind
source_type: internal
statuses:
  - value: draft
    display: Draft
    initial: true
  - value: active
    display: Active
    terminal: true
transitions:
  - from: draft
    to: active
    description: Activate
    manual: true
`

	if err := fileutil.WriteSecureFile(yamlPath, []byte(yamlContent)); err != nil {
		t.Fatalf("Failed to write test YAML: %v", err)
	}

	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)
	instance, err := builder.LoadFromYAML(yamlPath)

	if err != nil {
		t.Fatalf("LoadFromYAML() failed: %v", err)
	}

	if instance["id"] != "LIFECYCLE-test_kind-v1_0_0" {
		t.Errorf("Expected id 'LIFECYCLE-test_kind-v1_0_0', got '%v'", instance["id"])
	}
	if instance["object_type"] != "test_kind" {
		t.Errorf("Expected object_type 'test_kind', got '%v'", instance["object_type"])
	}

	statuses, ok := instance["statuses"].([]any)
	if !ok {
		t.Fatalf("Expected statuses to be []any, got %T", instance["statuses"])
	}
	if len(statuses) != 2 {
		t.Errorf("Expected 2 statuses, got %d", len(statuses))
	}
}

func TestLifecycleInstanceBuilder_WriteToYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "output-lifecycle.yaml")

	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	statuses := []any{
		map[string]any{
			"value":   "draft",
			"display": "Draft",
			"initial": true,
		},
	}

	builder.SetID("LIFECYCLE-test-output-v1_0_0")
	builder.SetObjectType("test_kind")
	builder.SetSourceType("internal")
	builder.SetField("statuses", statuses)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if err := builder.WriteToYAML(instance, yamlPath); err != nil {
		t.Fatalf("WriteToYAML() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Fatalf("YAML file was not created: %v", err)
	}

	// Load it back and verify
	loadedInstance, err := builder.LoadFromYAML(yamlPath)
	if err != nil {
		t.Fatalf("LoadFromYAML() after WriteToYAML failed: %v", err)
	}

	if loadedInstance["id"] != "LIFECYCLE-test-output-v1_0_0" {
		t.Errorf("Round-trip failed: expected id 'LIFECYCLE-test-output-v1_0_0', got '%v'", loadedInstance["id"])
	}
	if loadedInstance["object_type"] != "test_kind" {
		t.Errorf("Round-trip failed: expected object_type 'test_kind', got '%v'", loadedInstance["object_type"])
	}
}

func TestLifecycleInstanceBuilder_ComplexLifecycle(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	// Create a complex lifecycle with all fields
	statusMapping := map[string]any{
		"proposed": "draft",
		"approved": "active",
	}

	statuses := []any{
		map[string]any{
			"value":       "draft",
			"display":     "Draft",
			"initial":     true,
			"description": "Initial draft state",
		},
		map[string]any{
			"value":       "active",
			"display":     "Active",
			"description": "Active state",
		},
		map[string]any{
			"value":       "archived",
			"display":     "Archived",
			"archive":     true,
			"terminal":    true,
			"description": "Archived state",
		},
	}

	transitions := []any{
		map[string]any{
			"from":        "draft",
			"to":          "active",
			"description": "Activate",
			"manual":      true,
		},
		map[string]any{
			"from":        "active",
			"to":          "archived",
			"description": "Archive",
			"manual":      true,
		},
	}

	percentComplete := map[string]any{
		"method": "status_defaults",
		"default_by_status": map[string]any{
			"draft":    0,
			"active":   50,
			"archived": 100,
		},
	}

	builder.SetID("LIFECYCLE-complex_test-v1_0_0")
	builder.SetObjectType("complex_test")
	builder.SetSourceType("internal")
	builder.SetExtends("base_lifecycle")
	builder.SetStatusMapping(statusMapping)
	builder.SetField("statuses", statuses)
	builder.SetField("transitions", transitions)
	builder.SetPercentComplete(percentComplete)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Verify all fields are set
	if instance["object_type"] != "complex_test" {
		t.Errorf("Expected object_type 'complex_test', got '%v'", instance["object_type"])
	}
	if instance["source_type"] != "internal" {
		t.Errorf("Expected source_type 'internal', got '%v'", instance["source_type"])
	}
	if instance["extends"] != "base_lifecycle" {
		t.Errorf("Expected extends 'base_lifecycle', got '%v'", instance["extends"])
	}

	statusesList, ok := instance["statuses"].([]any)
	if !ok || len(statusesList) != 3 {
		t.Errorf("Expected 3 statuses, got %d", len(statusesList))
	}

	transitionsList, ok := instance["transitions"].([]any)
	if !ok || len(transitionsList) != 2 {
		t.Errorf("Expected 2 transitions, got %d", len(transitionsList))
	}

	pc, ok := instance["percent_complete"].(map[string]any)
	if !ok || pc["method"] != "status_defaults" {
		t.Errorf("Expected percent_complete method 'status_defaults', got '%v'", pc["method"])
	}
}

func TestLifecycleInstanceBuilder_Registry(t *testing.T) {
	t.Parallel()
	registry := instance_builders.GetGlobalRegistry()

	builder, err := registry.GetBuilder("lifecycle", objects.DefaultSchemaVersion)
	if err != nil {
		t.Fatalf("Failed to get lifecycle instance builder from registry: %v", err)
	}

	if builder.GetKind() != "lifecycle" {
		t.Errorf("Expected kind 'lifecycle', got '%s'", builder.GetKind())
	}

	if builder.GetSchemaVersion() != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema version %q, got '%s'", objects.DefaultSchemaVersion, builder.GetSchemaVersion())
	}
}

func TestLifecycleInstanceBuilder_RequiredFields(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	// Try to build without ID (should fail)
	_, err := builder.Build()
	if err == nil {
		t.Error("Expected error when building without ID, but got nil")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("Expected error message to contain 'id is required', got: %v", err)
	}
}

func TestLifecycleInstanceBuilder_OptionalFields(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	// Build with only required fields (ID)
	builder.SetID("LIFECYCLE-minimal-v1_0_0")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed with minimal fields: %v", err)
	}

	// Verify required fields are set
	if instance["id"] == nil {
		t.Error("Expected id to be set")
	}
	if instance["kind"] != "lifecycle" {
		t.Error("Expected kind to be set to 'lifecycle'")
	}

	// Optional fields should not cause errors if not set
	// (they may be nil or absent, which is fine)
}

func TestLifecycleInstanceBuilder_SetField(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	// Use SetField for custom fields
	builder.SetField("custom_field", "custom_value")

	builder.SetID("LIFECYCLE-custom-v1_0_0")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if instance["custom_field"] != "custom_value" {
		t.Errorf("Expected custom_field 'custom_value', got '%v'", instance["custom_field"])
	}
}

func TestLifecycleInstanceBuilder_SetID(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	builder.SetID("LIFECYCLE-test_id-v1_0_0")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if instance["id"] != "LIFECYCLE-test_id-v1_0_0" {
		t.Errorf("Expected id 'LIFECYCLE-test_id-v1_0_0', got '%v'", instance["id"])
	}
}

func TestLifecycleInstanceBuilder_SystemFields(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	builder.SetID("LIFECYCLE-system-v1_0_0")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Verify system fields are automatically set
	if instance["kind"] != "lifecycle" {
		t.Errorf("Expected kind 'lifecycle', got '%v'", instance["kind"])
	}
	if instance["schema_version"] != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema_version %q, got '%v'", objects.DefaultSchemaVersion, instance["schema_version"])
	}
	if _, ok := instance["created_at"]; !ok {
		t.Error("Expected created_at to be set")
	}
	if _, ok := instance["updated_at"]; !ok {
		t.Error("Expected updated_at to be set")
	}
	if instance["created_by"] != "account:system" {
		t.Errorf("Expected created_by 'account:system', got '%v'", instance["created_by"])
	}
	if instance["updated_by"] != "account:system" {
		t.Errorf("Expected updated_by 'account:system', got '%v'", instance["updated_by"])
	}
}

func TestLifecycleInstanceBuilder_FluentAPI(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	// Test that lifecycle-specific setters return the builder for chaining
	result := builder.
		SetObjectType("test_kind").
		SetSourceType("internal").
		SetExtends("base_lifecycle")

	if result == nil {
		t.Fatal("Fluent API should return builder instance")
	}

	// Verify it's the same builder type (result is already *LifecycleInstanceBuilder)
	if result != builder {
		t.Error("Fluent API should return the same builder instance")
	}
}

func TestLifecycleInstanceBuilder_EmptyStatuses(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	// Empty statuses list should be allowed (validation happens at object level)
	builder.SetID("LIFECYCLE-empty-statuses-v1_0_0")
	builder.SetField("statuses", []any{})
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed with empty statuses: %v", err)
	}

	statuses, ok := instance["statuses"].([]any)
	if !ok {
		t.Fatalf("Expected statuses to be []any, got %T", instance["statuses"])
	}
	if len(statuses) != 0 {
		t.Errorf("Expected empty statuses list, got %d items", len(statuses))
	}
}

func TestLifecycleInstanceBuilder_WriteToSequence(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	seqPath := filepath.Join(tmpDir, "test-lifecycle.seq")

	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	statuses := []any{
		map[string]any{
			"value":   "draft",
			"display": "Draft",
			"initial": true,
		},
	}

	builder.SetID("LIFECYCLE-seq-test-v1_0_0")
	builder.SetObjectType("test_kind")
	builder.SetField("statuses", statuses)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Write to sequence format
	if err := builder.WriteToSequence(instance, seqPath); err != nil {
		t.Fatalf("WriteToSequence() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(seqPath); os.IsNotExist(err) {
		t.Fatalf("Sequence file was not created: %v", err)
	}

	// Load it back
	loadedInstance, err := builder.LoadFromSequence(seqPath)
	if err != nil {
		t.Fatalf("LoadFromSequence() failed: %v", err)
	}

	// Verify ID matches
	if loadedInstance["id"] != "LIFECYCLE-seq-test-v1_0_0" {
		t.Errorf("Round-trip failed: expected id 'LIFECYCLE-seq-test-v1_0_0', got '%v'", loadedInstance["id"])
	}
}

func TestLifecycleInstanceBuilder_LoadFromSequence(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	seqPath := filepath.Join(tmpDir, "test-lifecycle-load.seq")

	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	statuses := []any{
		map[string]any{
			"value":   "draft",
			"display": "Draft",
		},
	}

	builder.SetID("LIFECYCLE-load-seq-v1_0_0")
	builder.SetObjectType("test_kind")
	builder.SetField("statuses", statuses)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Write first
	if err := builder.WriteToSequence(instance, seqPath); err != nil {
		t.Fatalf("WriteToSequence() failed: %v", err)
	}

	// Load it back
	loadedInstance, err := builder.LoadFromSequence(seqPath)
	if err != nil {
		t.Fatalf("LoadFromSequence() failed: %v", err)
	}

	// Verify key fields
	if loadedInstance["id"] != "LIFECYCLE-load-seq-v1_0_0" {
		t.Errorf("Expected id 'LIFECYCLE-load-seq-v1_0_0', got '%v'", loadedInstance["id"])
	}
	if loadedInstance["object_type"] != "test_kind" {
		t.Errorf("Expected object_type 'test_kind', got '%v'", loadedInstance["object_type"])
	}
}

// TestLifecycleInstanceBuilder_StatusesWithPreconditions tests statuses with preconditions
func TestLifecycleInstanceBuilder_StatusesWithPreconditions(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	statuses := []any{
		map[string]any{
			"value":   "draft",
			"display": "Draft",
			"initial": true,
		},
		map[string]any{
			"value":         "active",
			"display":       "Active",
			"preconditions": []any{"priority_plan_ref is set", "milestone_refs is not empty"},
		},
	}

	builder.SetID("LIFECYCLE-preconditions-v1_0_0")
	builder.SetField("statuses", statuses)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	statusesList, ok := instance["statuses"].([]any)
	if !ok || len(statusesList) != 2 {
		t.Fatalf("Expected 2 statuses, got %d", len(statusesList))
	}

	activeStatus, ok := statusesList[1].(map[string]any)
	if !ok {
		t.Fatalf("Expected status to be map[string]any, got %T", statusesList[1])
	}

	preconditions, ok := activeStatus["preconditions"].([]any)
	if !ok || len(preconditions) != 2 {
		t.Errorf("Expected 2 preconditions, got %d", len(preconditions))
	}
}

// TestLifecycleInstanceBuilder_TransitionsWithPreconditions tests transitions with preconditions
func TestLifecycleInstanceBuilder_TransitionsWithPreconditions(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	transitions := []any{
		map[string]any{
			"from":          "draft",
			"to":            "active",
			"description":   "Activate",
			"manual":        true,
			"preconditions": []any{"title is set", "description is not empty"},
		},
	}

	builder.SetID("LIFECYCLE-transition-preconditions-v1_0_0")
	builder.SetField("transitions", transitions)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	transitionsList, ok := instance["transitions"].([]any)
	if !ok || len(transitionsList) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitionsList))
	}

	transition, ok := transitionsList[0].(map[string]any)
	if !ok {
		t.Fatalf("Expected transition to be map[string]any, got %T", transitionsList[0])
	}

	preconditions, ok := transition["preconditions"].([]any)
	if !ok || len(preconditions) != 2 {
		t.Errorf("Expected 2 preconditions, got %d", len(preconditions))
	}
}

// TestLifecycleInstanceBuilder_TypeAssertions tests type safety of returned values
func TestLifecycleInstanceBuilder_TypeAssertions(t *testing.T) {
	t.Parallel()
	builder := NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)

	statuses := []any{
		map[string]any{
			"value":   "draft",
			"display": "Draft",
		},
	}

	builder.SetID("LIFECYCLE-types-v1_0_0")
	builder.SetObjectType("test_kind")
	builder.SetField("statuses", statuses)
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Verify types are as expected
	if _, ok := instance["id"].(string); !ok {
		t.Errorf("Expected id to be string, got %T", instance["id"])
	}
	if _, ok := instance["kind"].(string); !ok {
		t.Errorf("Expected kind to be string, got %T", instance["kind"])
	}
	if _, ok := instance["schema_version"].(string); !ok {
		t.Errorf("Expected schema_version to be string, got %T", instance["schema_version"])
	}
	if _, ok := instance["object_type"].(string); !ok {
		t.Errorf("Expected object_type to be string, got %T", instance["object_type"])
	}
	if _, ok := instance["statuses"].([]any); !ok {
		t.Errorf("Expected statuses to be []any, got %T", instance["statuses"])
	}
}
