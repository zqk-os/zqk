package pm

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/dna"
)

func TestMigrateLegacyBacklogItemYAML(t *testing.T) {
	legacyYAML := `
created_at: "2026-06-23T14:41:43Z"
created_by: ACC-1785920548450214012-68b850c0
description: Core migration testing backlog item.
estimated_effort: 2 days
goal_refs:
- GOAL-1782237534659420000-d8a7cdea
id: BLI-1782225703572753000-7bfab85b
kind: backlog_item
namespace_id: zqk:kernel
priority: high
priority_tier: tier_1
problem_statement: Ensure clean backwards compatibility with legacy CAS data.
schema_version: 2.0.0
status: in_progress
title: Migrate Backlog Items to DNA
updated_at: "2026-08-14T08:21:32Z"
updated_by: ACC-1785920548450214012-68b850c0
version_context: default
`
	bli, err := MigrateLegacyBacklogItem([]byte(legacyYAML))
	if err != nil {
		t.Fatalf("MigrateLegacyBacklogItem failed: %v", err)
	}

	// Verify URN synthesis
	expectedURN := "urn:zqk:kernel:backlog_item:BLI-1782225703572753000-7bfab85b"
	if bli.URN.String() != expectedURN {
		t.Errorf("URN mismatch: got %q, want %q", bli.URN.String(), expectedURN)
	}

	// Verify BaseObject invariants
	if bli.Kind != "backlog_item" {
		t.Errorf("Kind mismatch: got %q, want backlog_item", bli.Kind)
	}
	if bli.Plane != dna.PlanePromoted {
		t.Errorf("Plane mismatch: got %q, want %q", bli.Plane, dna.PlanePromoted)
	}
	if bli.SchemaRef != "2.0.0" {
		t.Errorf("SchemaRef mismatch: got %q, want 2.0.0", bli.SchemaRef)
	}

	// Verify Provenance
	prov := bli.GetProvenance()
	if prov.ParentHash != DefaultGenesisParentHash {
		t.Errorf("ParentHash mismatch: got %q, want %q", prov.ParentHash, DefaultGenesisParentHash)
	}
	if prov.AgentID != "ACC-1785920548450214012-68b850c0" {
		t.Errorf("AgentID mismatch: got %q", prov.AgentID)
	}
	if prov.Hash == "" {
		t.Errorf("expected non-empty Provenance Hash")
	}
	if prov.Signature != "migrated:"+prov.Hash {
		t.Errorf("Signature mismatch: got %q, want %q", prov.Signature, "migrated:"+prov.Hash)
	}
	if prov.Metadata["migration_source"] != "legacy_v1" {
		t.Errorf("Provenance metadata mismatch: got %v", prov.Metadata)
	}

	// Verify Lifecycle
	if bli.Status != "in_progress" {
		t.Errorf("Status mismatch: got %q, want in_progress", bli.Status)
	}

	// Verify Domain Fields
	if bli.Title != "Migrate Backlog Items to DNA" {
		t.Errorf("Title mismatch: got %q", bli.Title)
	}
	if bli.Description != "Core migration testing backlog item." {
		t.Errorf("Description mismatch: got %q", bli.Description)
	}
	if len(bli.GoalRefs) != 1 || bli.GoalRefs[0] != "GOAL-1782237534659420000-d8a7cdea" {
		t.Errorf("GoalRefs mismatch: got %v", bli.GoalRefs)
	}
}

func TestMigrateLegacyBacklogItemJSON(t *testing.T) {
	legacyMap := map[string]any{
		"id":           "BLI-JSON-TEST-001",
		"kind":         "backlog_item",
		"namespace_id": "cell_alpha",
		"title":        "JSON Migration Test",
		"description":  "Testing JSON ingestion path",
		"status":       "conceptual",
		"priority":     "medium",
		"created_by":   "agent-json",
	}
	jsonData, err := json.Marshal(legacyMap)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	bli, err := MigrateLegacyBacklogItem(jsonData)
	if err != nil {
		t.Fatalf("MigrateLegacyBacklogItem failed on JSON: %v", err)
	}

	if bli.URN.Cell != "cell_alpha" {
		t.Errorf("Cell ID mismatch: expected cell_alpha, got %q", bli.URN.Cell)
	}
	if bli.Plane != dna.PlaneDraft {
		t.Errorf("Plane mismatch: conceptual status should map to PlaneDraft, got %q", bli.Plane)
	}
	if bli.GetProvenance().AgentID != "agent-json" {
		t.Errorf("AgentID mismatch: got %q, want agent-json", bli.GetProvenance().AgentID)
	}
}

func TestMigrateLegacyBacklogItemErrorCases(t *testing.T) {
	// Empty data
	if _, err := MigrateLegacyBacklogItem(nil); err == nil {
		t.Errorf("expected error on nil rawData")
	}
	if _, err := MigrateLegacyBacklogItem([]byte("")); err == nil {
		t.Errorf("expected error on empty rawData")
	}

	// Missing ID
	noID := `title: No ID item
description: Missing ID`
	if _, err := MigrateLegacyBacklogItem([]byte(noID)); err == nil {
		t.Errorf("expected error when ID is missing")
	}

	// Missing Title
	noTitle := `id: BLI-123
description: Missing title`
	if _, err := MigrateLegacyBacklogItem([]byte(noTitle)); err == nil {
		t.Errorf("expected error when Title is missing")
	}
}

func TestMigrateActualCASBacklogFile(t *testing.T) {
	samplePath := "../../.zqk/process/backlog_items/0012e74fe0f16c0aadf6d0eae87c63f612ca8537b8374b21687aa861cf9adc0e.yaml"
	data, err := os.ReadFile(samplePath)
	if err != nil {
		t.Skipf("sample CAS file %s not found, skipping disk test", samplePath)
	}

	bli, err := MigrateLegacyBacklogItem(data)
	if err != nil {
		t.Fatalf("failed to migrate real CAS backlog file: %v", err)
	}

	if bli.URN.ID != "BLI-1782225703572753000-7bfab85b" {
		t.Errorf("unexpected ID: %s", bli.URN.ID)
	}
	if bli.GetProvenance().ParentHash != DefaultGenesisParentHash {
		t.Errorf("unexpected ParentHash: %s", bli.GetProvenance().ParentHash)
	}
}
