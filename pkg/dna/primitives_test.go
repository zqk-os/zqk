package dna

import (
	"fmt"
	"testing"
	"time"
)


// MockEntity implements Auditable and Lifecycle for testing.
type MockEntity struct {
	BaseObject
	Provenance Provenance
	Current    LifecyclePhase
}

func (m *MockEntity) GetProvenance() Provenance { return m.Provenance }
func (m *MockEntity) ComputeDigest() (string, error) {
	return m.Provenance.ComputeDigest(), nil
}
func (m *MockEntity) Phase() LifecyclePhase { return m.Current }
func (m *MockEntity) CanTransition(target LifecyclePhase) bool {
	switch m.Current {
	case PhaseDraft:
		return target == PhaseClaimed || target == PhaseApoptotic
	case PhaseClaimed:
		return target == PhaseEvaluating || target == PhaseDraft
	case PhaseEvaluating:
		return target == PhasePromoted || target == PhaseDraft
	case PhasePromoted:
		return false
	default:
		return false
	}
}
func (m *MockEntity) ApoptosisPolicy() string {
	return "quarantine_on_stale_claim_300s"
}

func TestURNFormattingAndParsing(t *testing.T) {
	cellID := "core"
	kind := "backlog_item"
	id := "BLI-12345"

	urn := FormatURN(cellID, kind, id)
	expected := "urn:zqk:core:backlog_item:BLI-12345"
	if urn != expected {
		t.Fatalf("expected URN %q, got %q", expected, urn)
	}

	u, err := ParseURN(urn)
	if err != nil {
		t.Fatalf("unexpected error parsing URN: %v", err)
	}
	if u.Cell != cellID || u.Kind != kind || u.ID != id {
		t.Fatalf("parsed values mismatch: got (%s, %s, %s)", u.Cell, u.Kind, u.ID)
	}

	invalidURNs := []string{
		"urn:onlyone",
		"urn:zqk:onlytwo",
		"noturn:brand:cell:kind:id",
		"",
		"invalid",
	}
	for _, inv := range invalidURNs {
		if _, err := ParseURN(inv); err == nil {
			t.Errorf("expected error parsing invalid URN %q, got nil", inv)
		}
	}

	// Verify brand swapping
	SetDefaultBrand("custom_brand")
	defer SetDefaultBrand(DefaultBrand)

	customURN := FormatURN("core", "task", "TSK-1")
	if customURN != "urn:custom_brand:core:task:TSK-1" {
		t.Fatalf("expected custom brand URN, got %q", customURN)
	}

	parsedCustom, err := ParseURN(customURN)
	if err != nil {
		t.Fatalf("failed to parse custom brand URN: %v", err)
	}
	if parsedCustom.Brand != "custom_brand" || parsedCustom.Cell != "core" || parsedCustom.ID != "TSK-1" {
		t.Fatalf("parsed custom brand mismatch: %+v", parsedCustom)
	}
}

func TestBaseObjectValidation(t *testing.T) {
	urn, _ := NewURN("cell1", "backlog_item", "BLI-1")
	valid := BaseObject{
		URN:       urn,
		Kind:      "backlog_item",
		Plane:     PlaneDraft,
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid BaseObject, got error: %v", err)
	}

	epicURN, _ := NewURN("cell1", "epic", "EPI-1")
	mismatch := BaseObject{
		URN:  epicURN,
		Kind: "backlog_item",
	}
	if err := mismatch.Validate(); err == nil {
		t.Errorf("expected error on kind mismatch, got nil")
	}

	missingURN := BaseObject{Kind: "backlog_item"}
	if err := missingURN.Validate(); err == nil {
		t.Errorf("expected error on missing URN, got nil")
	}
}

func TestPlaneTransitionsAndParsing(t *testing.T) {
	planes := []struct {
		str  string
		want Plane
	}{
		{"draft", PlaneDraft},
		{"staged", PlaneStaged},
		{"promoted", PlanePromoted},
		{"apoptotic", PlaneApoptotic},
	}

	for _, tc := range planes {
		p, err := ParsePlane(tc.str)
		if err != nil {
			t.Fatalf("ParsePlane(%q) failed: %v", tc.str, err)
		}
		if p != tc.want {
			t.Errorf("ParsePlane(%q) = %v, want %v", tc.str, p, tc.want)
		}
	}

	if _, err := ParsePlane("invalid"); err == nil {
		t.Errorf("expected error parsing invalid plane, got nil")
	}
}

func TestProvenanceAndAuditable(t *testing.T) {
	prov := Provenance{
		ParentHash: "0000000000000000000000000000000000000000000000000000000000000000",
		AgentID:    "agent-007",
		Signature:  "sig-test",
		Metadata:   map[string]string{"model": "qwen3"},
	}

	digest := prov.ComputeDigest()
	if digest == "" {
		t.Fatalf("expected non-empty digest")
	}

	// Deterministic
	digest2 := prov.ComputeDigest()
	if digest != digest2 {
		t.Fatalf("expected deterministic digest, got %q != %q", digest, digest2)
	}
}

func TestLifecycleStateTransitions(t *testing.T) {
	entity := &MockEntity{
		Current: PhaseDraft,
	}

	// Valid transition: Draft -> Claimed
	if !entity.CanTransition(PhaseClaimed) {
		t.Errorf("expected Draft -> Claimed to be permissible")
	}

	// Invalid transition: Draft -> Promoted (must not skip Evaluating)
	if entity.CanTransition(PhasePromoted) {
		t.Errorf("expected Draft -> Promoted to be forbidden")
	}

	entity.Current = PhaseClaimed
	if !entity.CanTransition(PhaseEvaluating) {
		t.Errorf("expected Claimed -> Evaluating to be permissible")
	}

	entity.Current = PhaseEvaluating
	if !entity.CanTransition(PhasePromoted) {
		t.Errorf("expected Evaluating -> Promoted to be permissible")
	}

	entity.Current = PhasePromoted
	if entity.CanTransition(PhaseDraft) {
		t.Errorf("expected Promoted -> Draft to be immutable/forbidden")
	}
}

func TestMetaSchemaVetting(t *testing.T) {
	schema := &MetaSchema{
		TargetKind: "backlog_item",
		Fields: map[string]FieldMetaSpec{
			"title": {
				Name:      "title",
				Type:      TypeString,
				Required:  true,
				MinLength: 5,
			},
			"epic_urn": {
				Name:     "epic_urn",
				Type:     TypeURN,
				Required: true,
			},
		},
	}

	validData := map[string]any{
		"title":    "Implement DNA sectors",
		"epic_urn": "urn:zqk:cell1:epic:EPI-1",
	}
	if err := schema.VetObject(validData); err != nil {
		t.Fatalf("expected valid object vetting, got: %v", err)
	}

	shortTitle := map[string]any{
		"title":    "Fix",
		"epic_urn": "urn:zqk:cell1:epic:EPI-1",
	}
	if err := schema.VetObject(shortTitle); err == nil {
		t.Errorf("expected error on title min_length violation, got nil")
	}

	missingField := map[string]any{
		"title": "Valid title without epic_urn",
	}
	if err := schema.VetObject(missingField); err == nil {
		t.Errorf("expected error on missing required epic_urn, got nil")
	}

	invalidURN := map[string]any{
		"title":    "Valid title",
		"epic_urn": "not-a-urn",
	}
	if err := schema.VetObject(invalidURN); err == nil {
		t.Errorf("expected error on invalid URN, got nil")
	}
}

func TestMetaSchemaEpistemicInvariants(t *testing.T) {
	schema := &MetaSchema{TargetKind: "backlog_item"}

	// Hook for custom business gate
	schema.RegisterHook(func(target any, targetPlane Plane) error {
		if targetPlane == PlanePromoted {
			if e, ok := target.(*MockEntity); ok {
				if e.Current != PhasePromoted && e.Current != PhaseEvaluating {
					return fmt.Errorf("gate failure: entity must be evaluating before promotion")
				}
			}
		}
		return nil
	})

	entity := &MockEntity{
		Current: PhaseDraft,
		Provenance: Provenance{
			AgentID: "", // Missing ActorAttestation
		},
	}

	// Must fail: missing AgentID for Staged/Promoted
	if err := schema.VetTransition(entity, PlaneStaged); err == nil {
		t.Errorf("expected failure when AgentID is empty, got nil")
	}

	entity.Provenance.AgentID = "agent-alpha"
	entity.Provenance.ParentHash = "parent-sha256"

	// Custom hook should fail because Current is PhaseDraft, not PhaseEvaluating
	if err := schema.VetTransition(entity, PlanePromoted); err == nil {
		t.Errorf("expected custom gate failure, got nil")
	}

	// Now set evaluating
	entity.Current = PhaseEvaluating
	if err := schema.VetTransition(entity, PlanePromoted); err != nil {
		t.Fatalf("expected promotion transition to succeed, got: %v", err)
	}
}
