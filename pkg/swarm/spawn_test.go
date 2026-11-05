package swarm

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSpawnCellSwarm_Neuron(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	teamObj := map[string]any{
		objects.FieldKeyID:   "TCFG-NEURON-ARCH",
		objects.FieldKeyKind: "team_configuration",
		"cell_type":          "neuron",
		"focus_area":         "strategic",
	}
	_ = store.Create(ctx, sec, teamObj)

	opts := SpawnOptions{
		ProjectRoot:     tempDir,
		TeamConfigID:    "TCFG-NEURON-ARCH",
		Storage:         store,
		SecurityContext: sec,
	}

	manifest, err := SpawnCellSwarm(ctx, opts)
	if err != nil {
		t.Fatalf("SpawnCellSwarm failed: %v", err)
	}

	if manifest.CellArchetype != CellArchetypeNeuron {
		t.Errorf("expected cell archetype neuron, got %s", manifest.CellArchetype)
	}
	if manifest.FocusArea != "strategic" {
		t.Errorf("expected focus area strategic, got %s", manifest.FocusArea)
	}
	if manifest.TotalPods != 1 {
		t.Errorf("expected 1 pod provisioned, got %d", manifest.TotalPods)
	}
	if len(manifest.Pods[0].Capabilities) == 0 {
		t.Errorf("expected capabilities for neuron cell")
	}
}

func TestSpawnCellSwarm_MuscleWithPlanAttachment(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	teamObj := map[string]any{
		objects.FieldKeyID:   "TCFG-MUSCLE-EXEC",
		objects.FieldKeyKind: "team_configuration",
		"cell_type":          "muscle",
		"focus_area":         "tactical",
		"persona_allocations": []any{
			map[string]any{
				"persona_ref": "PER-CRAFTSMAN-LEAD",
				"role":        "lead_craftsman",
				"count":       2,
			},
		},
	}
	_ = store.Create(ctx, sec, teamObj)

	planObj := map[string]any{
		objects.FieldKeyID:     "PRI-001",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Q4 Refactor Sprint",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	_ = store.Create(ctx, sec, planObj)

	opts := SpawnOptions{
		ProjectRoot:     tempDir,
		TeamConfigID:    "TCFG-MUSCLE-EXEC",
		PriorityPlanID:  "PRI-001",
		Storage:         store,
		SecurityContext: sec,
	}

	manifest, err := SpawnCellSwarm(ctx, opts)
	if err != nil {
		t.Fatalf("SpawnCellSwarm failed: %v", err)
	}

	if manifest.CellArchetype != CellArchetypeMuscle {
		t.Errorf("expected muscle cell, got %s", manifest.CellArchetype)
	}
	if manifest.TotalPods != 2 {
		t.Errorf("expected 2 pods allocated, got %d", manifest.TotalPods)
	}
	if manifest.PriorityPlanRef != "PRI-001" {
		t.Errorf("expected attached plan PRI-001, got %s", manifest.PriorityPlanRef)
	}
	for _, p := range manifest.Pods {
		if p.PriorityPlanRef != "PRI-001" {
			t.Errorf("pod %s missing priority_plan_ref", p.PodID)
		}
		if p.Role != "lead_craftsman" {
			t.Errorf("expected role lead_craftsman, got %s", p.Role)
		}
	}

	// Verify plan was updated in storage with team_configuration_ref
	updatedPlan, err := store.Read(ctx, sec, "PRI-001")
	if err != nil {
		t.Fatalf("failed to read updated plan: %v", err)
	}
	ref, _ := updatedPlan[objects.FieldKeyTeamConfigurationRef].(string)
	if ref != "TCFG-MUSCLE-EXEC" {
		t.Errorf("expected plan team_configuration_ref TCFG-MUSCLE-EXEC, got %q", ref)
	}

	// Verify manifest was persisted to state
	manifestPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.StateDir, paths.MeshStateSubdir, "spawn-manifest-latest.json")
	if !fileutil.Exists(manifestPath) {
		t.Errorf("expected manifest to be persisted at %s", manifestPath)
	}
	data, _ := fileutil.ReadFile(manifestPath)
	var loaded SpawnManifest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal persisted manifest: %v", err)
	}
	if loaded.TotalPods != 2 || loaded.TeamConfigID != "TCFG-MUSCLE-EXEC" {
		t.Errorf("persisted manifest mismatch: %+v", loaded)
	}
}

func TestSpawnCellSwarm_HeartAndLungs(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	// Heart cell
	heartObj := map[string]any{
		objects.FieldKeyID:   "TCFG-HEART-CAP",
		objects.FieldKeyKind: "team_configuration",
		"cell_type":          "heart",
	}
	_ = store.Create(ctx, sec, heartObj)

	heartManifest, err := SpawnCellSwarm(ctx, SpawnOptions{
		ProjectRoot:     tempDir,
		TeamConfigID:    "TCFG-HEART-CAP",
		Storage:         store,
		SecurityContext: sec,
	})
	if err != nil {
		t.Fatalf("Heart spawn failed: %v", err)
	}
	if heartManifest.FocusArea != "operations" {
		t.Errorf("expected operations focus area, got %s", heartManifest.FocusArea)
	}

	// Lungs cell
	lungsObj := map[string]any{
		objects.FieldKeyID:   "TCFG-LUNGS-INGEST",
		objects.FieldKeyKind: "team_configuration",
		"cell_type":          "lungs",
	}
	_ = store.Create(ctx, sec, lungsObj)

	lungsManifest, err := SpawnCellSwarm(ctx, SpawnOptions{
		ProjectRoot:     tempDir,
		TeamConfigID:    "TCFG-LUNGS-INGEST",
		Storage:         store,
		SecurityContext: sec,
	})
	if err != nil {
		t.Fatalf("Lungs spawn failed: %v", err)
	}
	if lungsManifest.FocusArea != "intake" {
		t.Errorf("expected intake focus area, got %s", lungsManifest.FocusArea)
	}
}

func TestSpawnCellSwarm_InvalidArchetypeAndMissingConfig(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockStorage()

	// Missing config
	_, err := SpawnCellSwarm(ctx, SpawnOptions{
		ProjectRoot:     tempDir,
		TeamConfigID:    "TCFG-NONEXISTENT",
		Storage:         store,
		SecurityContext: sec,
	})
	if err == nil {
		t.Errorf("expected error for nonexistent team config")
	}

	// Invalid cell_type
	badObj := map[string]any{
		objects.FieldKeyID:   "TCFG-BAD",
		objects.FieldKeyKind: "team_configuration",
		"cell_type":          "plasma",
	}
	_ = store.Create(ctx, sec, badObj)

	_, err = SpawnCellSwarm(ctx, SpawnOptions{
		ProjectRoot:     tempDir,
		TeamConfigID:    "TCFG-BAD",
		Storage:         store,
		SecurityContext: sec,
	})
	if err == nil {
		t.Errorf("expected error for unsupported cell archetype")
	}
}
