package objects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpecLoader_SubdirectoryAndFieldRef(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Setup layout:
	// tmp/base_object.yaml
	// tmp/built_ins/fields/timestamp.yaml
	// tmp/pm/sample_item.yaml

	baseContent := `schema_version: 2.0.0
ontology: base_object
visibility: internal
traits: []
fields:
  id:
    type: string
`
	if err := os.WriteFile(filepath.Join(tmp, "base_object.yaml"), []byte(baseContent), 0644); err != nil {
		t.Fatal(err)
	}

	fieldsDir := filepath.Join(tmp, "built_ins", "fields")
	if err := os.MkdirAll(fieldsDir, 0755); err != nil {
		t.Fatal(err)
	}
	tsContent := `type: string
format: date-time
description: "Universal UTC timestamp"
traits:
  - readable
`
	if err := os.WriteFile(filepath.Join(fieldsDir, "timestamp.yaml"), []byte(tsContent), 0644); err != nil {
		t.Fatal(err)
	}

	pmDir := filepath.Join(tmp, "pm")
	if err := os.MkdirAll(pmDir, 0755); err != nil {
		t.Fatal(err)
	}
	sampleContent := `schema_version: 2.0.0
ontology: sample_item
extends: base_object
visibility: internal
traits: []
fields:
  created_at:
    $ref: "timestamp"
    description: "Creation time override"
  title:
    type: string
`
	if err := os.WriteFile(filepath.Join(pmDir, "sample_item.yaml"), []byte(sampleContent), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewSpecLoader(tmp)

	// Test 1: DiscoverOntologies should find both base_object and sample_item
	ontologies, err := loader.DiscoverOntologies()
	if err != nil {
		t.Fatalf("DiscoverOntologies error: %v", err)
	}
	hasSample := false
	for _, ont := range ontologies {
		if ont == "sample_item" {
			hasSample = true
			break
		}
	}
	if !hasSample {
		t.Fatalf("expected DiscoverOntologies to find sample_item in subfolder, got: %v", ontologies)
	}

	// Test 2: LoadSpec without inheritance finds sample_item in subfolder
	specNoInherit, err := loader.LoadSpec("sample_item")
	if err != nil {
		t.Fatalf("LoadSpec(sample_item) error: %v", err)
	}
	if specNoInherit.Ontology != "sample_item" {
		t.Fatalf("unexpected ontology: %s", specNoInherit.Ontology)
	}

	// Test 3: LoadSpecWithInheritance resolves parent and $ref field
	spec, err := loader.LoadSpecWithInheritance("sample_item.yaml")
	if err != nil {
		t.Fatalf("LoadSpecWithInheritance(sample_item.yaml) error: %v", err)
	}
	if spec.ResolvedFields == nil {
		t.Fatal("spec.ResolvedFields is nil")
	}
	// Verify inherited field from base_object
	if _, ok := spec.ResolvedFields["id"]; !ok {
		t.Fatalf("missing inherited field 'id'")
	}
	// Verify resolved $ref field
	ca, ok := spec.ResolvedFields["created_at"]
	if !ok {
		t.Fatalf("missing field 'created_at'")
	}
	caMap, ok := ca.(map[string]any)
	if !ok {
		t.Fatalf("expected created_at to be map[string]any, got: %T", ca)
	}
	// Format should come from referenced timestamp definition
	if caMap["format"] != "date-time" {
		t.Fatalf("expected format 'date-time' from $ref, got: %v", caMap["format"])
	}
	// Description should be locally overridden
	if caMap["description"] != "Creation time override" {
		t.Fatalf("expected overridden description, got: %v", caMap["description"])
	}
	// $ref key should be purged from materialized map
	if _, hasRef := caMap["$ref"]; hasRef {
		t.Fatalf("$ref key was not purged from resolved field")
	}
}

func TestLifecycleLoader_SubdirectoryDiscovery(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	baseContent := `object_type: base_object
statuses:
  - value: draft
`
	if err := os.WriteFile(filepath.Join(tmp, "base_object_lifecycle.yaml"), []byte(baseContent), 0644); err != nil {
		t.Fatal(err)
	}

	pmDir := filepath.Join(tmp, "pm")
	if err := os.MkdirAll(pmDir, 0755); err != nil {
		t.Fatal(err)
	}
	bliContent := `object_type: backlog_item
statuses:
  - value: open
`
	if err := os.WriteFile(filepath.Join(pmDir, "backlog_item_lifecycle.yaml"), []byte(bliContent), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLifecycleLoader(tmp)
	lc, err := loader.LoadLifecycle("backlog_item")
	if err != nil {
		t.Fatalf("LoadLifecycle(backlog_item) failed from subfolder: %v", err)
	}
	if lc.ObjectType != "backlog_item" {
		t.Fatalf("unexpected object_type: %s", lc.ObjectType)
	}
}
