package dna

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

type mockEntity struct {
	BaseObject `yaml:",inline"`
	Title      string `yaml:"title"`
	Content    string `yaml:"content"`
}

func TestMetaSchemaValidation(t *testing.T) {
	ctx := context.Background()

	schema := NewMetaSchema(
		"mock_entity",
		"2.0.0",
		[]Plane{PlaneDraft, PlaneStaged, PlanePromoted},
		[]string{"title", "content"},
	)

	// Register a dynamic invariant requiring Title to be at least 3 chars
	schema.RegisterDynamicInvariant(func(ctx context.Context, obj any) error {
		if entity, ok := obj.(mockEntity); ok {
			if len(entity.Title) < 3 {
				return errfmt.Errorf("title must be at least 3 characters")
			}
		}
		return nil
	})

	urn, _ := NewURN("kernel", "mock_entity", "MCK-001")
	base := NewBaseObject(urn, "2.0.0")

	// Valid entity
	valid := mockEntity{
		BaseObject: base,
		Title:      "Proper Title",
		Content:    "Meaningful content",
	}
	if err := schema.Validate(ctx, valid); err != nil {
		t.Fatalf("expected valid entity to pass schema validation: %v", err)
	}

	// Missing required field (content)
	invalidMissing := mockEntity{
		BaseObject: base,
		Title:      "Proper Title",
		Content:    "",
	}
	if err := schema.Validate(ctx, invalidMissing); err == nil {
		t.Errorf("expected missing required field 'content' to fail validation")
	}

	// Disallowed Plane
	invalidPlane := valid
	invalidPlane.Plane = PlaneApoptotic
	if err := schema.Validate(ctx, invalidPlane); err == nil {
		t.Errorf("expected disallowed plane %q to fail validation", PlaneApoptotic)
	}

	// Dynamic invariant failure
	invalidShortTitle := valid
	invalidShortTitle.Title = "AB"
	if err := schema.Validate(ctx, invalidShortTitle); err == nil {
		t.Errorf("expected dynamic invariant to reject short title")
	}

	// Map validation
	mapEntity := map[string]any{
		"title":   "Valid Map Title",
		"content": "Valid map content",
	}
	if err := schema.Validate(ctx, mapEntity); err != nil {
		t.Errorf("expected valid map entity to pass validation: %v", err)
	}

	delete(mapEntity, "content")
	if err := schema.Validate(ctx, mapEntity); err == nil {
		t.Errorf("expected map missing 'content' to fail validation")
	}
}

func TestSchemaRegistry(t *testing.T) {
	schema := NewMetaSchema("registered_kind", "1.0.0", []Plane{PlaneDraft}, []string{"name"})
	if err := RegisterSchema(schema); err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}

	retrieved, ok := GetSchema("registered_kind")
	if !ok || retrieved != schema {
		t.Errorf("failed to retrieve registered schema")
	}

	// Case-insensitive retrieval
	retrievedUpper, ok := GetSchema("REGISTERED_KIND")
	if !ok || retrievedUpper != schema {
		t.Errorf("expected case-insensitive retrieval to succeed")
	}

	// Non-existent kind
	if _, ok := GetSchema("non_existent_kind"); ok {
		t.Errorf("expected non-existent schema to return false")
	}
}

func TestBaseEntityMetaSchemaRegistration(t *testing.T) {
	baseSchema, ok := GetSchema(KindBaseEntity)
	if !ok || baseSchema == nil {
		t.Fatalf("expected BaseEntityMetaSchema to be registered by default")
	}
	if baseSchema.TargetKind != KindBaseEntity {
		t.Errorf("expected TargetKind %q, got %q", KindBaseEntity, baseSchema.TargetKind)
	}
	reqFields := baseSchema.RequiredFields()
	expectedReq := []string{"urn", "kind", "plane", "version", "created_at", "updated_at"}
	for _, ef := range expectedReq {
		found := false
		for _, rf := range reqFields {
			if rf == ef {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected required field %q in BaseEntityMetaSchema", ef)
		}
	}
}

func TestResolveMacroPhase(t *testing.T) {
	schema := &MetaSchema{
		TargetKind: "test_case",
		MacroPhases: map[MacroPhase]MacroPhaseSpec{
			MacroPhaseGestation: {
				Phase:       MacroPhaseGestation,
				Plane:       PlaneDraft,
				SubStatuses: []string{"conceptual", "draft", "proposed"},
			},
			MacroPhaseMetabolism: {
				Phase:       MacroPhaseMetabolism,
				Plane:       PlaneStaged,
				SubStatuses: []string{"active", "in_progress"},
			},
			MacroPhaseEquilibrium: {
				Phase:       MacroPhaseEquilibrium,
				Plane:       PlanePromoted,
				SubStatuses: []string{"complete", "validated"},
			},
			MacroPhaseApoptosis: {
				Phase:       MacroPhaseApoptosis,
				Plane:       PlaneApoptotic,
				SubStatuses: []string{"archived"},
			},
		},
	}

	tests := []struct {
		subStatus     string
		expectedPhase MacroPhase
		expectedPlane Plane
		expectedFound bool
	}{
		{"conceptual", MacroPhaseGestation, PlaneDraft, true},
		{"draft", MacroPhaseGestation, PlaneDraft, true},
		{"active", MacroPhaseMetabolism, PlaneStaged, true},
		{"complete", MacroPhaseEquilibrium, PlanePromoted, true},
		{"archived", MacroPhaseApoptosis, PlaneApoptotic, true},
		{"unknown_status", MacroPhaseGestation, PlaneDraft, false},
	}

	for _, tt := range tests {
		phase, plane, found := schema.ResolvePhase(tt.subStatus)
		if found != tt.expectedFound {
			t.Errorf("ResolvePhase(%s) expected found=%v, got %v", tt.subStatus, tt.expectedFound, found)
		}
		if found && phase != tt.expectedPhase {
			t.Errorf("ResolvePhase(%s) expected phase %s, got %s", tt.subStatus, tt.expectedPhase, phase)
		}
		if found && plane != tt.expectedPlane {
			t.Errorf("ResolvePhase(%s) expected plane %s, got %s", tt.subStatus, tt.expectedPlane, plane)
		}
	}
}

func TestMetaSchemaStructuralConstraints(t *testing.T) {
	schema := &MetaSchema{
		TargetKind: "account",
		Fields: map[string]FieldMetaSpec{
			"code": {
				Name:      "code",
				Type:      TypeString,
				Required:  true,
				Pattern:   `^[A-Z]{3}-\d{2}$`,
				MaxLength: 10,
			},
			"tier": {
				Name:     "tier",
				Type:     TypeEnum,
				Required: true,
				Enum:     []string{"P0", "P1", "P2", "P3"},
			},
		},
	}

	valid := map[string]any{
		"code": "ACC-01",
		"tier": "P1",
	}
	if err := schema.VetObject(valid); err != nil {
		t.Fatalf("expected valid object to pass vetting, got: %v", err)
	}

	// Pattern mismatch
	invalidPattern := map[string]any{
		"code": "invalid-code",
		"tier": "P1",
	}
	if err := schema.VetObject(invalidPattern); err == nil {
		t.Errorf("expected pattern violation error, got nil")
	}

	// Enum mismatch
	invalidEnum := map[string]any{
		"code": "ACC-01",
		"tier": "P99",
	}
	if err := schema.VetObject(invalidEnum); err == nil {
		t.Errorf("expected enum violation error, got nil")
	}
}

func TestStorageProfileValidation(t *testing.T) {
	// Test parsing and IsKnown
	for _, p := range []StorageProfile{StorageProfileCASEntity, StorageProfileStream, StorageProfileLightFile} {
		if !p.IsKnown() {
			t.Errorf("expected %s to be known", p)
		}
		parsed, err := ParseStorageProfile(string(p))
		if err != nil || parsed != p {
			t.Errorf("failed to parse known profile %s: %v", p, err)
		}
	}

	// Unknown profile
	if StorageProfile("arbitrary_db").IsKnown() {
		t.Errorf("expected arbitrary_db to not be known")
	}
	_, err := ParseStorageProfile("arbitrary_db")
	if err == nil {
		t.Errorf("expected error parsing unknown storage profile")
	}

	// Register schema with valid profile
	validSchema := &MetaSchema{
		TargetKind:     "test_stream_kind",
		StorageProfile: StorageProfileStream,
	}
	if err := RegisterSchema(validSchema); err != nil {
		t.Fatalf("expected valid schema registration to succeed: %v", err)
	}

	// Register schema with invalid profile
	invalidSchema := &MetaSchema{
		TargetKind:     "test_bad_kind",
		StorageProfile: StorageProfile("kafka_cluster"),
	}
	if err := RegisterSchema(invalidSchema); err == nil {
		t.Errorf("expected error registering schema with invalid storage profile")
	}
}

func TestObjectAndFieldMetaSpecVetting(t *testing.T) {
	spec := &ObjectMetaSpec{
		TargetKind: "test_entity",
		Fields: map[string]FieldMetaSpec{
			"code": {
				Name: "code",
				Type: TypeString,
				Validation: FieldValidation{
					Required:  true,
					Pattern:   `^[A-Z]{3}-\d{3}$`,
					MinLength: 7,
					MaxLength: 7,
				},
			},
			"tags": {
				Name: "tags",
				Type: TypeList,
			},
		},
	}

	// 1. Verify RequiredFields
	reqs := spec.RequiredFields()
	if len(reqs) != 1 || reqs[0] != "code" {
		t.Fatalf("expected required field ['code'], got %v", reqs)
	}

	// 2. Verify GetField
	f, ok := spec.GetField("code")
	if !ok || !f.IsRequired() {
		t.Fatalf("failed to retrieve FieldMetaSpec")
	}

	// 3. Vet valid object
	validObj := map[string]any{
		"code": "EPK-001",
		"tags": []string{"core", "pm"},
	}
	if err := spec.VetObject(validObj); err != nil {
		t.Fatalf("expected valid object to pass vetting: %v", err)
	}

	// 4. Vet missing required
	missingObj := map[string]any{
		"tags": []string{"core"},
	}
	if err := spec.VetObject(missingObj); err == nil {
		t.Errorf("expected error for missing required field 'code'")
	}

	// 5. Vet cardinality violation (cardinality: one receiving a list)
	cardinalityViolatingObj := map[string]any{
		"code": []any{"EPK-001", "EPK-002"},
	}
	if err := spec.VetObject(cardinalityViolatingObj); err == nil {
		t.Errorf("expected error for cardinality violation on 'code'")
	}

	// 6. Vet pattern violation
	patternViolatingObj := map[string]any{
		"code": "invalid-code",
	}
	if err := spec.VetObject(patternViolatingObj); err == nil {
		t.Errorf("expected error for regex pattern violation on 'code'")
	}
}

func TestDNACoreInterfaceAndMetaSpecs(t *testing.T) {
	urn, err := NewURN("kernel", "account", "ACC-test")
	if err != nil {
		t.Fatalf("failed to create URN: %v", err)
	}

	base := NewBaseObject(urn, "urn:zqk:spec:kernel:account")

	// 1. Verify BaseObject satisfies Entity / SystemObject interface
	var entity Entity = &base
	if entity.GetURN() != urn {
		t.Errorf("expected URN %v, got %v", urn, entity.GetURN())
	}
	if entity.GetKind() != "account" {
		t.Errorf("expected kind 'account', got %q", entity.GetKind())
	}
	if entity.GetPlane() != PlaneDraft {
		t.Errorf("expected PlaneDraft, got %v", entity.GetPlane())
	}

	// 2. Verify TraitMetaSpec
	traitURN, _ := NewURN("spec", "trait_meta_spec", "occupiable")
	trait := TraitMetaSpec{
		BaseObject:  NewBaseObject(traitURN, "urn:zqk:spec:kernel:trait_meta_spec"),
		Name:        "occupiable",
		Description: "Enforces occupancy slot claim semantics",
		Category:    "behavior",
		ObjectLevel: true,
	}
	var traitEntity Entity = &trait
	if traitEntity.GetKind() != "trait_meta_spec" {
		t.Errorf("expected trait kind 'trait_meta_spec', got %q", traitEntity.GetKind())
	}

	// 3. Verify Enum
	e := Enum{
		Name:   "PriorityTier",
		Values: []string{"P0", "P1", "P2", "P3"},
	}
	if !e.IsValid("p1") {
		t.Errorf("expected 'p1' to be valid in Enum")
	}
	if e.IsValid("P99") {
		t.Errorf("expected 'P99' to be invalid in Enum")
	}
}





