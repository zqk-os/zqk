package dna

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestBaseObject_Getters(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	urn := FormatURN("cell-1", "task", "task-1")
	parsedURN, err := ParseURN(urn)
	if err != nil {
		t.Fatalf("ParseURN: %v", err)
	}

	bo := NewBaseObject(parsedURN, "schema-ref-1")
	bo.Version = 3
	bo.CreatedAt = now
	bo.UpdatedAt = now

	if bo.GetVersion() != 3 {
		t.Errorf("expected version 3, got %d", bo.GetVersion())
	}
	if !bo.GetCreatedAt().Equal(now) {
		t.Errorf("expected created at %v, got %v", now, bo.GetCreatedAt())
	}
	if !bo.GetUpdatedAt().Equal(now) {
		t.Errorf("expected updated at %v, got %v", now, bo.GetUpdatedAt())
	}
}

func TestLifecycle_Methods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// NewLifecycleFacet
	facet := NewLifecycleFacet(MacroPhaseMetabolism, "running", PlaneStaged, map[string][]string{"running": {"done"}})
	if facet.MacroPhase != MacroPhaseMetabolism || facet.SubStatus != "running" {
		t.Errorf("unexpected facet values: %+v", facet)
	}

	// NewLifecycle
	transitions := map[string][]string{
		"originated":  {"in_progress", "rejected"},
		"in_progress": {"completed", "failed"},
	}
	lc := NewLifecycle("originated", transitions)

	if lc.Phase() != "originated" {
		t.Errorf("expected phase originated, got %s", lc.Phase())
	}
	if !lc.CanTransition("in_progress") {
		t.Errorf("expected CanTransition to in_progress true")
	}
	if lc.CanTransition("completed") {
		t.Errorf("expected CanTransition to completed false")
	}
	if lc.ApoptosisPolicy() != "quarantine_on_stale_claim_300s" {
		t.Errorf("unexpected apoptosis policy: %s", lc.ApoptosisPolicy())
	}

	// RegisterInvariantGate and Transition success
	gateCalled := false
	lc.RegisterInvariantGate("in_progress", func(ctx context.Context, obj any) error {
		gateCalled = true
		return nil
	})

	if err := lc.Transition(ctx, nil, "in_progress"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}
	if !gateCalled {
		t.Errorf("expected gate to be called")
	}
	if lc.Phase() != "in_progress" {
		t.Errorf("expected phase in_progress, got %s", lc.Phase())
	}

	// Invariant gate failure
	lc.RegisterInvariantGate("failed", func(ctx context.Context, obj any) error {
		return errors.New("gate blocked transition")
	})
	if err := lc.Transition(ctx, nil, "failed"); err == nil {
		t.Errorf("expected gate error, got nil")
	}

	// Illegal transition error
	if err := lc.Transition(ctx, nil, "originated"); err == nil {
		t.Errorf("expected illegal transition error, got nil")
	}
}

func TestURN_Serialization(t *testing.T) {
	t.Parallel()

	u := URN{
		Brand: "zqk",
		Cell:  "cell1",
		Kind:  "task",
		ID:    "123",
	}

	// MarshalJSON & UnmarshalJSON
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	var u2 URN
	if err := json.Unmarshal(data, &u2); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if u2.String() != u.String() {
		t.Errorf("expected %s, got %s", u.String(), u2.String())
	}

	// UnmarshalJSON empty string
	var emptyU URN
	if err := json.Unmarshal([]byte(`""`), &emptyU); err != nil {
		t.Fatalf("UnmarshalJSON empty error: %v", err)
	}
	if !emptyU.IsZero() {
		t.Errorf("expected empty URN to be zero")
	}

	// MarshalYAML & UnmarshalYAML
	yamlData, err := yaml.Marshal(u)
	if err != nil {
		t.Fatalf("MarshalYAML error: %v", err)
	}
	var u3 URN
	if err := yaml.Unmarshal(yamlData, &u3); err != nil {
		t.Fatalf("UnmarshalYAML error: %v", err)
	}
	if u3.String() != u.String() {
		t.Errorf("expected %s, got %s", u.String(), u3.String())
	}

	// UnmarshalYAML empty string
	var emptyYAML URN
	if err := yaml.Unmarshal([]byte(`""`), &emptyYAML); err != nil {
		t.Fatalf("UnmarshalYAML empty error: %v", err)
	}
	if !emptyYAML.IsZero() {
		t.Errorf("expected empty YAML URN to be zero")
	}

	// FormatSchemaRef
	schemaRef := FormatSchemaRef("zqk", "core", "test_schema")
	if schemaRef != "urn:zqk:spec:core:test_schema" {
		t.Errorf("unexpected schemaRef: %s", schemaRef)
	}
}

func TestAuditableAndMetaSchema_Provenance(t *testing.T) {
	t.Parallel()

	prov := Provenance{
		AgentID:   "user1",
		Signature: "sig123",
	}
	aud := &Auditable{
		Provenance: prov,
	}

	if aud.GetProvenance().AgentID != "user1" {
		t.Errorf("GetProvenance failed")
	}
	digest, err := aud.ComputeDigest()
	if err != nil || digest == "" {
		t.Errorf("ComputeDigest failed: %v", err)
	}

	schema := &MetaSchema{
		Provenance: prov,
		Traits:     []string{"auditable", "immutable"},
	}
	if schema.GetProvenance().AgentID != "user1" {
		t.Errorf("MetaSchema GetProvenance failed")
	}
	schemaDigest, err := schema.ComputeDigest()
	if err != nil || schemaDigest == "" {
		t.Errorf("MetaSchema ComputeDigest failed: %v", err)
	}

	// HasTrait
	if !schema.HasTrait("auditable") {
		t.Errorf("expected HasTrait auditable true")
	}
	if !schema.HasTrait("IMMUTABLE") {
		t.Errorf("expected case-insensitive HasTrait true")
	}
	if schema.HasTrait("unknown") {
		t.Errorf("expected HasTrait unknown false")
	}
}
