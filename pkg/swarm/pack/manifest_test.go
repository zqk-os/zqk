package pack

import (
	"path/filepath"
	"testing"
)

func TestParseManifest_Valid(t *testing.T) {
	yamlContent := `
name: test-swarm
version: 1.0.0
description: Test swarm for packaging
agents:
  - name: Tester
    role: software_engineer
    skills:
      - ASK-001
membranes:
  - path: .zqk/process/
    mode: read_only
tasks:
  - id: t1
    title: First Task
`
	pkg, err := ParseManifest([]byte(yamlContent))
	if err != nil {
		t.Fatalf("unexpected error parsing manifest: %v", err)
	}

	if pkg.Name != "test-swarm" {
		t.Errorf("expected name 'test-swarm', got %q", pkg.Name)
	}
	if len(pkg.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(pkg.Agents))
	}
	if len(pkg.Membranes) != 1 || pkg.Membranes[0].Mode != "read_only" {
		t.Errorf("expected 1 read_only membrane, got %v", pkg.Membranes)
	}
}

func TestParseManifest_Invalid(t *testing.T) {
	// Missing name
	bad1 := `
version: 1.0.0
description: Test
agents:
  - name: Agent1
    role: worker
`
	if _, err := ParseManifest([]byte(bad1)); err == nil {
		t.Error("expected error for missing name")
	}

	// Missing both agents and team_configuration
	bad2 := `
name: test-swarm
version: 1.0.0
description: Test
`
	if _, err := ParseManifest([]byte(bad2)); err == nil {
		t.Error("expected error for missing both agents and team_configuration")
	}

	// Invalid membrane mode
	bad3 := `
name: test-swarm
version: 1.0.0
description: Test
agents:
  - name: A
    role: R
membranes:
  - path: /tmp
    mode: invalid_mode
`
	if _, err := ParseManifest([]byte(bad3)); err == nil {
		t.Error("expected error for invalid membrane mode")
	}
}

func TestParseManifest_TeamConfiguration(t *testing.T) {
	// 1. Valid with team_configuration_ref (no agents declared inline)
	yamlWithRef := `
name: team-ref-swarm
version: 1.0.0
description: Swarm using team_configuration_ref
team_configuration_ref: TCFG-CEF-EVALUATION-POD
tasks:
  - id: t1
    title: Task 1
`
	pkg1, err := ParseManifest([]byte(yamlWithRef))
	if err != nil {
		t.Fatalf("unexpected error with team_configuration_ref: %v", err)
	}
	if pkg1.TeamConfigurationRef != "TCFG-CEF-EVALUATION-POD" {
		t.Errorf("expected team_configuration_ref TCFG-CEF-EVALUATION-POD, got %s", pkg1.TeamConfigurationRef)
	}
	if len(pkg1.Agents) != 0 {
		t.Errorf("expected 0 inline agents, got %d", len(pkg1.Agents))
	}

	// 2. Valid with inline team_configuration
	yamlWithInline := `
name: inline-team-swarm
version: 1.0.0
description: Swarm with inline team_configuration
team_configuration:
  cell_type: neuron
  focus_area: architecture
  persona_allocations:
    - persona_ref: PER-1790238938478986000
      count: 2
tasks:
  - id: t1
    title: Task 1
`
	pkg2, err := ParseManifest([]byte(yamlWithInline))
	if err != nil {
		t.Fatalf("unexpected error with inline team_configuration: %v", err)
	}
	if pkg2.TeamConfiguration == nil || pkg2.TeamConfiguration.CellType != "neuron" {
		t.Errorf("unexpected inline team configuration: %+v", pkg2.TeamConfiguration)
	}

	// 3. Valid with both team_configuration_ref and ad-hoc persona augmentation
	yamlHybrid := `
name: hybrid-swarm
version: 1.0.0
description: Swarm with both team_configuration_ref and ad-hoc agent
team_configuration_ref: TCFG-DEFAULT-ENGINEERING
agents:
  - name: Ad-hoc Specialist
    role: specialist_evaluator
tasks:
  - id: t1
    title: Task 1
`
	pkg3, err := ParseManifest([]byte(yamlHybrid))
	if err != nil {
		t.Fatalf("unexpected error with hybrid pack: %v", err)
	}
	if pkg3.TeamConfigurationRef != "TCFG-DEFAULT-ENGINEERING" || len(pkg3.Agents) != 1 {
		t.Errorf("unexpected hybrid pack state: %+v", pkg3)
	}
}

func TestEnsureSampleSwarm(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "swarm.yaml")

	if err := EnsureSampleSwarm(samplePath); err != nil {
		t.Fatalf("EnsureSampleSwarm failed: %v", err)
	}

	pkg, err := LoadManifestFile(samplePath)
	if err != nil {
		t.Fatalf("LoadManifestFile failed on generated sample: %v", err)
	}

	if pkg.Name != "sample-refactor-swarm" {
		t.Errorf("expected sample name 'sample-refactor-swarm', got %q", pkg.Name)
	}
	if len(pkg.Agents) != 2 {
		t.Errorf("expected 2 agents in sample, got %d", len(pkg.Agents))
	}
}

func TestDraftSwarmManifestYAML(t *testing.T) {
	// 1. Default options
	data, err := DraftSwarmManifestYAML(SwarmDraftOptions{})
	if err != nil {
		t.Fatalf("unexpected error drafting swarm: %v", err)
	}
	pkg, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("failed to parse generated manifest: %v", err)
	}
	if pkg.Name != "custom-swarm" {
		t.Errorf("expected custom-swarm, got %q", pkg.Name)
	}
	if pkg.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", pkg.Version)
	}
	if len(pkg.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(pkg.Agents))
	}
	if len(pkg.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(pkg.Tasks))
	}

	// 2. Cellular archetype
	cellData, err := DraftSwarmManifestYAML(SwarmDraftOptions{
		Name:     "cell-swarm",
		CellType: "neuron",
	})
	if err != nil {
		t.Fatalf("unexpected error drafting cellular swarm: %v", err)
	}
	cellPkg, err := ParseManifest(cellData)
	if err != nil {
		t.Fatalf("failed to parse cellular manifest: %v", err)
	}
	if cellPkg.TeamConfiguration == nil || cellPkg.TeamConfiguration.CellType != "neuron" {
		t.Errorf("expected neuron cell_type, got %+v", cellPkg.TeamConfiguration)
	}

	// 3. Goal configuration
	goalData, err := DraftSwarmManifestYAML(SwarmDraftOptions{
		Name:            "goal-swarm",
		GoalMetric:      "test_coverage",
		GoalTarget:      "95%",
		GoalDescription: "Ensure high coverage",
	})
	if err != nil {
		t.Fatalf("unexpected error drafting goal swarm: %v", err)
	}
	goalPkg, err := ParseManifest(goalData)
	if err != nil {
		t.Fatalf("failed to parse goal manifest: %v", err)
	}
	if goalPkg.Goal == nil || goalPkg.Goal.Metric != "test_coverage" || goalPkg.Goal.Target != "95%" {
		t.Errorf("expected test_coverage goal, got %+v", goalPkg.Goal)
	}

	// 4. Invalid SemVer
	_, err = DraftSwarmManifestYAML(SwarmDraftOptions{
		Version: "invalid-semver",
	})
	if err == nil {
		t.Fatal("expected error with invalid SemVer version")
	}
}

