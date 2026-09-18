package objects

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSpecLoader_ResolveInheritance tests that extends chains are properly resolved
// TDD: Write tests first, then implement
func TestSpecLoader_ResolveInheritance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		specFile       string
		expectedParent string
		wantErr        bool
	}{
		{
			name:           "account extends base_object",
			specFile:       "account.yaml",
			expectedParent: "base_object",
			wantErr:        false,
		},
		{
			name:           "base_object extends auditable",
			specFile:       "base_object.yaml",
			expectedParent: "auditable",
			wantErr:        false,
		},
		{
			name:           "auditable has no parent",
			specFile:       "auditable.yaml",
			expectedParent: "",
			wantErr:        false,
		},
		{
			name:           "extensible_object extends base_object",
			specFile:       "extensible_object.yaml",
			expectedParent: "base_object",
			wantErr:        false,
		},
		{
			name:           "work_interval extends base_object",
			specFile:       "work_interval.yaml",
			expectedParent: "base_object",
			wantErr:        false,
		},
		{
			name:           "work_unit extends work_interval",
			specFile:       "work_unit.yaml",
			expectedParent: "work_interval",
			wantErr:        false,
		},
		{
			name:           "occupancy extends work_interval",
			specFile:       "occupancy.yaml",
			expectedParent: "work_interval",
			wantErr:        false,
		},
		{
			name:           "remaining_open extends work_interval",
			specFile:       "remaining_open.yaml",
			expectedParent: "work_interval",
			wantErr:        false,
		},
		{
			name:           "agent_task extends work_unit",
			specFile:       "agent_task.yaml",
			expectedParent: "work_unit",
			wantErr:        false,
		},
		{
			name:           "backlog_item extends work_unit",
			specFile:       "backlog_item.yaml",
			expectedParent: "work_unit",
			wantErr:        false,
		},
		{
			name:           "roadmap extends work_interval",
			specFile:       "roadmap.yaml",
			expectedParent: "work_interval",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This will fail until LoadSpecWithInheritance is implemented
			loader := NewSpecLoader("")
			spec, err := loader.LoadSpecWithInheritance(tt.specFile)

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadSpecWithInheritance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if spec == nil {
					t.Fatal("LoadSpecWithInheritance() returned nil spec")
				}
				if spec.Extends != tt.expectedParent {
					t.Errorf("Expected extends = %q, got %q", tt.expectedParent, spec.Extends)
				}
			}
		})
	}
}

// TestSpecLoader_FieldInheritance tests that fields from parent specs are inherited
func TestSpecLoader_FieldInheritance(t *testing.T) {
	t.Parallel()
	// account.yaml extends base_object
	// base_object extends auditable
	// So account should have fields from all three specs

	loader := NewSpecLoader("")
	spec, err := loader.LoadSpecWithInheritance("account.yaml")

	if err != nil {
		t.Fatalf("LoadSpecWithInheritance() error = %v", err)
	}

	if spec == nil {
		t.Fatal("LoadSpecWithInheritance() returned nil spec")
	}

	// Account should have its own fields
	if _, ok := spec.ResolvedFields[FieldKeyUsername]; !ok {
		t.Error("Account spec should have 'username' field")
	}

	// Account should inherit from base_object
	if _, ok := spec.ResolvedFields[FieldKeyID]; !ok {
		t.Error("Account spec should inherit 'id' field from base_object")
	}
	if _, ok := spec.ResolvedFields[FieldKeyTitle]; !ok {
		t.Error("Account spec should inherit 'title' field from base_object")
	}
	if _, ok := spec.ResolvedFields[FieldKeyStatus]; !ok {
		t.Error("Account spec should inherit 'status' field from base_object")
	}

	// Account should inherit from auditable (via base_object)
	if _, ok := spec.ResolvedFields[FieldKeyCreatedAt]; !ok {
		t.Error("Account spec should inherit 'created_at' field from auditable")
	}
	if _, ok := spec.ResolvedFields[FieldKeyCreatedBy]; !ok {
		t.Error("Account spec should inherit 'created_by' field from auditable")
	}
	if _, ok := spec.ResolvedFields[FieldKeyUpdatedAt]; !ok {
		t.Error("Account spec should inherit 'updated_at' field from auditable")
	}
	if _, ok := spec.ResolvedFields[FieldKeyUpdatedBy]; !ok {
		t.Error("Account spec should inherit 'updated_by' field from auditable")
	}

	if _, ok := spec.ResolvedFields[FieldKeyEstimatedEffort]; ok {
		t.Error("Account spec must not inherit estimated_effort after BASE-LIFT (work_unit mixin)")
	}
	if _, ok := spec.ResolvedFields[FieldKeyActualEffort]; ok {
		t.Error("Account spec must not inherit actual_effort after BASE-LIFT (work_unit mixin)")
	}
	if _, ok := spec.ResolvedFields[FieldKeyStartedAt]; ok {
		t.Error("Account spec must not inherit started_at (not completable)")
	}
	if _, ok := spec.ResolvedFields[FieldKeyCompletedAt]; ok {
		t.Error("Account spec must not inherit completed_at (not completable)")
	}
}

func TestSpecLoader_WorkEnvelopeMixinFields(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")

	bli, err := loader.LoadSpecWithInheritance("backlog_item.yaml")
	if err != nil {
		t.Fatalf("backlog_item: %v", err)
	}
	for _, key := range []string{FieldKeyStartedAt, FieldKeyCompletedAt, FieldKeyEstimatedEffort, FieldKeyActualEffort} {
		if _, ok := bli.ResolvedFields[key]; !ok {
			t.Errorf("backlog_item must inherit %s via work_unit", key)
		}
	}

	roadmap, err := loader.LoadSpecWithInheritance("roadmap.yaml")
	if err != nil {
		t.Fatalf("roadmap: %v", err)
	}
	for _, key := range []string{FieldKeyStartedAt, FieldKeyCompletedAt} {
		if _, ok := roadmap.ResolvedFields[key]; !ok {
			t.Errorf("roadmap must inherit %s via work_interval", key)
		}
	}
	if _, ok := roadmap.ResolvedFields[FieldKeyEstimatedEffort]; ok {
		t.Error("roadmap must not inherit estimated_effort (not effort_aware)")
	}
	if _, ok := roadmap.ResolvedFields[FieldKeyActualEffort]; ok {
		t.Error("roadmap must not inherit actual_effort (not effort_aware)")
	}

	base, err := loader.LoadSpecWithInheritance("base_object.yaml")
	if err != nil {
		t.Fatalf("base_object: %v", err)
	}
	if _, ok := base.ResolvedFields[FieldKeyEstimatedEffort]; ok {
		t.Error("base_object must not declare estimated_effort after BASE-LIFT")
	}
	if _, ok := base.ResolvedFields[FieldKeyActualEffort]; ok {
		t.Error("base_object must not declare actual_effort after BASE-LIFT")
	}
}

func TestSpecLoader_OccupancyComposesOntoAgentTask(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")

	occ, err := loader.LoadSpecWithInheritance("occupancy.yaml")
	if err != nil {
		t.Fatalf("occupancy: %v", err)
	}
	if occ.Extends != KindWorkInterval {
		t.Fatalf("occupancy extends=%q want work_interval", occ.Extends)
	}
	if _, ok := occ.ResolvedFields[FieldKeyEstimatedEffort]; ok {
		t.Error("occupancy must not inherit estimated_effort (not work_unit)")
	}

	atk, err := loader.LoadSpecWithInheritance("agent_task.yaml")
	if err != nil {
		t.Fatalf("agent_task: %v", err)
	}
	if atk.Extends != KindWorkUnit {
		t.Fatalf("agent_task extends=%q want work_unit", atk.Extends)
	}
	composed := false
	for _, name := range atk.Composes {
		if name == KindOccupancy {
			composed = true
			break
		}
	}
	if !composed {
		t.Fatalf("agent_task composes=%v want occupancy", atk.Composes)
	}
	if atk.Fields != nil {
		if _, ok := atk.Fields[FieldKeyClaimedBy]; ok {
			t.Error("agent_task must not redeclare claimed_by")
		}
	}
	for _, key := range []string{FieldKeyClaimedBy, FieldKeyClaimedAt, FieldKeyEstimatedEffort, FieldKeyStartedAt} {
		if _, ok := atk.ResolvedFields[key]; !ok {
			t.Errorf("agent_task must resolve %s via work_unit + compose occupancy", key)
		}
	}

	bli, err := loader.LoadSpecWithInheritance("backlog_item.yaml")
	if err != nil {
		t.Fatalf("backlog_item: %v", err)
	}
	if _, ok := bli.ResolvedFields[FieldKeyClaimedBy]; ok {
		t.Error("backlog_item must not gain claimed_by (does not compose occupancy)")
	}

	pri, err := loader.LoadSpecWithInheritance("priority_plan.yaml")
	if err != nil {
		t.Fatalf("priority_plan: %v", err)
	}
	if _, ok := pri.ResolvedFields[FieldKeyClaimedBy]; ok {
		t.Error("priority_plan must not gain claimed_by until it extends occupancy")
	}
	if _, ok := pri.ResolvedFields[FieldKeyEstimatedEffort]; ok {
		t.Error("priority_plan must not inherit estimated_effort")
	}
}

func TestSpecLoader_RemainingOpenComposesOntoPriorityPlan(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")

	mix, err := loader.LoadSpecWithInheritance("remaining_open.yaml")
	if err != nil {
		t.Fatalf("remaining_open: %v", err)
	}
	if mix.Extends != KindWorkInterval {
		t.Fatalf("remaining_open extends=%q want work_interval", mix.Extends)
	}
	if _, ok := mix.ResolvedFields[FieldKeyRemainingOpenCount]; !ok {
		t.Fatal("remaining_open must declare remaining_open_count")
	}

	pri, err := loader.LoadSpecWithInheritance("priority_plan.yaml")
	if err != nil {
		t.Fatalf("priority_plan: %v", err)
	}
	composed := false
	for _, name := range pri.Composes {
		if name == KindRemainingOpen {
			composed = true
			break
		}
	}
	if !composed {
		t.Fatalf("priority_plan composes=%v want remaining_open", pri.Composes)
	}
	if pri.Fields != nil {
		if _, ok := pri.Fields[FieldKeyRemainingOpenCount]; ok {
			t.Error("priority_plan must not redeclare remaining_open_count")
		}
	}
	if _, ok := pri.ResolvedFields[FieldKeyRemainingOpenCount]; !ok {
		t.Error("priority_plan must resolve remaining_open_count via compose remaining_open")
	}

	bli, err := loader.LoadSpecWithInheritance("backlog_item.yaml")
	if err != nil {
		t.Fatalf("backlog_item: %v", err)
	}
	if _, ok := bli.ResolvedFields[FieldKeyRemainingOpenCount]; ok {
		t.Error("backlog_item must not gain remaining_open_count (does not compose remaining_open)")
	}
}

// TestSpecLoader_FieldOverride tests that child fields override parent fields
func TestSpecLoader_FieldOverride(t *testing.T) {
	t.Parallel()
	// If a child spec defines a field with the same name as parent,
	// child field should override parent field

	loader := NewSpecLoader("")
	spec, err := loader.LoadSpecWithInheritance("account.yaml")

	if err != nil {
		t.Fatalf("LoadSpecWithInheritance() error = %v", err)
	}

	// If account.yaml redefines a field from base_object, it should override
	// For now, just verify the field exists (override behavior will be tested when we have a case)
	if spec.ResolvedFields == nil {
		t.Fatal("Spec should have resolved fields")
	}
}

// TestSpecLoader_TraitInheritance tests that traits are inherited and merged
func TestSpecLoader_StorageProfileInheritance(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("LoadSpecWithInheritance() error = %v", err)
	}
	// auditable.yaml sets storage_profile: cas_entity; account extends base_object extends auditable.
	if spec.StorageProfile != "cas_entity" {
		t.Fatalf("StorageProfile = %q, want cas_entity", spec.StorageProfile)
	}
}

// TestSpecLoader_StreamKindOverridesAuditableProfile ensures high-volume kinds declare
// storage_profile: stream so they do not inherit cas_entity from auditable.
func TestSpecLoader_StreamKindOverridesAuditableProfile(t *testing.T) {
	t.Parallel()
	root := testModuleRoot(t)
	loader := NewSpecLoader(root)
	spec, err := loader.LoadSpecWithInheritance("audit_event.yaml")
	if err != nil {
		t.Fatalf("LoadSpecWithInheritance: %v", err)
	}
	if spec.StorageProfile != "stream" {
		t.Fatalf("StorageProfile = %q, want stream (auditable is cas_entity; audit_event must override)", spec.StorageProfile)
	}
}

func TestSpecLoader_TraitInheritance(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	spec, err := loader.LoadSpecWithInheritance("account.yaml")

	if err != nil {
		t.Fatalf("LoadSpecWithInheritance() error = %v", err)
	}

	// Account should inherit traits from base_object
	// account.yaml has: base_object_traits
	// base_object.yaml has: base_object_traits (which includes base_auditable_traits)
	// So account should have: base_object_traits, base_auditable_traits (inherited from base_object)
	expectedTraits := []string{
		"base_object_traits",    // From account.yaml
		"base_auditable_traits", // Inherited from base_object.yaml
	}

	traitMap := make(map[string]bool)
	for _, trait := range spec.ResolvedTraits {
		traitMap[trait] = true
	}

	for _, expected := range expectedTraits {
		if !traitMap[expected] {
			t.Errorf("Account spec should have trait '%s' (from own spec or inherited from base_object)", expected)
		}
	}
}

func TestSpecLoader_ExcludeTraits(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	spec, err := loader.LoadSpecWithInheritance("scheduler_job.yaml")
	if err != nil {
		t.Fatalf("LoadSpecWithInheritance() error = %v", err)
	}
	found := false
	for _, trait := range spec.ExcludeTraits {
		if trait == "auto_status_transitionable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected scheduler_job to declare exclude_traits including auto_status_transitionable")
	}
}

// TestSpecLoader_InheritanceChain tests multi-level inheritance chains
func TestSpecLoader_InheritanceChain(t *testing.T) {
	t.Parallel()
	// Test: component.yaml extends extensible_object extends base_object extends auditable
	loader := NewSpecLoader("")
	spec, err := loader.LoadSpecWithInheritance("component.yaml")

	if err != nil {
		t.Fatalf("LoadSpecWithInheritance() error = %v", err)
	}

	// Component should have fields from all levels
	// From component itself
	if _, ok := spec.ResolvedFields[FieldKeyDomain]; !ok {
		t.Error("Component spec should have 'domain' field")
	}

	// From extensible_object
	if _, ok := spec.ResolvedFields[FieldKeySpecInterpreter]; !ok {
		t.Error("Component spec should inherit 'spec_interpreter' from extensible_object")
	}

	// From base_object
	if _, ok := spec.ResolvedFields[FieldKeyID]; !ok {
		t.Error("Component spec should inherit 'id' from base_object")
	}

	// From auditable
	if _, ok := spec.ResolvedFields[FieldKeyCreatedAt]; !ok {
		t.Error("Component spec should inherit 'created_at' from auditable")
	}
}

// TestSpecLoader_CircularInheritance tests that circular extends are detected
func TestSpecLoader_CircularInheritance(t *testing.T) {
	t.Parallel()
	// This test will need a mock spec with circular inheritance
	// For now, just verify the loader handles it gracefully
	loader := NewSpecLoader("")

	// If we had a circular spec, it should error
	// For now, test that normal specs don't error on circular check
	_, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		// If error mentions circular, that's good
		if err.Error() == "circular inheritance detected" {
			return // Expected error
		}
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestSpecLoader_MissingParent tests that missing parent specs are handled
func TestSpecLoader_MissingParent(t *testing.T) {
	t.Parallel()
	// If a spec extends a non-existent parent, should error gracefully
	loader := NewSpecLoader("")

	// Create a temporary spec file with invalid extends
	tempDir := t.TempDir()
	invalidSpec := `schema_version: "` + DefaultSchemaVersion + `"
ontology: test
extends: non_existent_parent
`

	specPath := filepath.Join(tempDir, "test.yaml")
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = fileutil.WriteFile(specPath, []byte(invalidSpec), paths.FilePerm644)

	// Should error when parent doesn't exist
	_, err := loader.LoadSpecWithInheritance(specPath)
	if err == nil {
		t.Error("Expected error when parent spec doesn't exist")
	}
}

func testModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found (expected module root)")
		}
		dir = parent
	}
}

func TestNormalizeSpecsDirIfProjectRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specs := filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.EnsureDir(specs); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(specs, baseObjectSpecName)
	if err := fileutil.WriteSecureFile(base, []byte("x")); err != nil {
		t.Fatal(err)
	}
	got := normalizeSpecsDirIfProjectRoot(dir)
	if filepath.Clean(got) != filepath.Clean(specs) {
		t.Fatalf("got %q want %q", got, specs)
	}
	if got := normalizeSpecsDirIfProjectRoot(specs); filepath.Clean(got) != filepath.Clean(specs) {
		t.Fatalf("object_specs dir should pass through unchanged: %q", got)
	}
}

func TestNewSpecLoader_projectRootUsesFileSpecStorage(t *testing.T) {
	t.Parallel()
	root := testModuleRoot(t)
	loader := NewSpecLoader(root)
	if loader.specStorage == nil {
		t.Fatal("expected non-nil specStorage when NewSpecLoader is given project root")
	}
	wantSpecs := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if filepath.Clean(loader.specsDir) != filepath.Clean(wantSpecs) {
		t.Fatalf("specsDir=%q want %q", loader.specsDir, wantSpecs)
	}
	if _, err := loader.LoadSpecWithInheritance("backlog_item.yaml"); err != nil {
		t.Fatalf("LoadSpecWithInheritance(backlog_item.yaml): %v", err)
	}
}

// TestSpecLoader_ValidateTraitContracts verifies BLI-SPEC-TRAIT-VALIDATOR-002
// and CRIT-1788679768850001000-aaaa: compile-time trait validation membrane
// verifies spec contracts before registering kinds into the type registry.
func TestSpecLoader_ValidateTraitContracts(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")

	specs, err := loader.GetLoadOrder()
	if err != nil {
		t.Fatalf("GetLoadOrder: %v", err)
	}

	// Validate trait contracts across all loaded specs
	errors := loader.ValidateTraitContracts(specs)
	for _, vErr := range errors {
		if vErr.MissingItem == "traits" || strings.Contains(vErr.Message, "Trait validation error") {
			t.Errorf("unexpected invalid trait: %s", vErr.Message)
		}
	}

	// Verify that a synthetic invalid spec triggers validation error
	invalidSpec := &Spec{
		Ontology:       "synthetic_invalid",
		ResolvedTraits: []string{"nonexistent_trait_xyz_123"},
	}
	errs := loader.ValidateTraitContracts([]*Spec{invalidSpec})
	if len(errs) == 0 {
		t.Error("expected trait validation error for nonexistent trait, got 0")
	}
}
