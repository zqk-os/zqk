package pm_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/dna"
	"github.com/lanceman/zqk/pkg/dna/pm"
)

func TestDNAForwardedPMEntities(t *testing.T) {
	ctx := context.Background()

	bli, err := pm.NewBacklogItem("BLI-CELLULAR-001", "DNA Primitives", "Implement DNA Primitives")
	if err != nil {
		t.Fatalf("failed to create BacklogItem: %v", err)
	}
	if bli.Plane != dna.PlaneDraft {
		t.Errorf("expected PlaneDraft, got %s", bli.Plane)
	}

	epic, err := pm.NewEpic("EPC-001", "Cellular OS", "Master epic")
	if err != nil {
		t.Fatalf("failed to create Epic: %v", err)
	}
	epic.AddMemberWorkUnit(bli)

	// Invariant promotion check: child is not complete
	if err := epic.CanPromote(ctx); err == nil {
		t.Errorf("expected promotion to fail when member is not complete")
	}

	// Advance child to complete
	if err := bli.TransitionTo(ctx, "originated"); err != nil {
		t.Fatalf("failed to transition: %v", err)
	}
	if err := bli.TransitionTo(ctx, "exploring"); err != nil {
		t.Fatalf("failed to transition: %v", err)
	}
	if err := bli.TransitionTo(ctx, "planned"); err != nil {
		t.Fatalf("failed to transition: %v", err)
	}
	if err := bli.TransitionTo(ctx, "in_progress"); err != nil {
		t.Fatalf("failed to transition: %v", err)
	}
	if err := bli.TransitionTo(ctx, "complete"); err != nil {
		t.Fatalf("failed to transition: %v", err)
	}

	// Now epic can promote
	if err := epic.CanPromote(ctx); err != nil {
		t.Fatalf("expected epic promotion to succeed: %v", err)
	}

	adr, err := pm.NewADR("ADR-001", "Architecture Decision", "Context", "Decision", "Consequences")
	if err != nil {
		t.Fatalf("failed to create ADR: %v", err)
	}
	if adr.Status != "conceptual" {
		t.Errorf("expected conceptual status, got %s", adr.Status)
	}
}

func TestDNAMigrateLegacyBacklogItem(t *testing.T) {
	legacy := `
id: BLI-LEGACY-001
title: Legacy Item
description: Legacy description
status: in_progress
namespace_id: zqk:kernel
`
	bli, err := pm.MigrateLegacyBacklogItem([]byte(legacy))
	if err != nil {
		t.Fatalf("MigrateLegacyBacklogItem failed: %v", err)
	}
	if bli.GetID() != "BLI-LEGACY-001" {
		t.Errorf("expected ID BLI-LEGACY-001, got %s", bli.GetID())
	}
	if bli.Title != "Legacy Item" {
		t.Errorf("expected title 'Legacy Item', got %s", bli.Title)
	}
	if bli.GetProvenance().ParentHash == "" {
		t.Errorf("expected non-empty Genesis ParentHash")
	}
}
