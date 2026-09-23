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

	// Missing agents
	bad2 := `
name: test-swarm
version: 1.0.0
description: Test
`
	if _, err := ParseManifest([]byte(bad2)); err == nil {
		t.Error("expected error for missing agents")
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
