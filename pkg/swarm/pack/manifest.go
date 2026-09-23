package pack

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SwarmPackage represents a portable, decentralized swarm manifest.
type SwarmPackage struct {
	Schema      string         `yaml:"$schema,omitempty" json:"$schema,omitempty"`
	Name        string         `yaml:"name" json:"name"`
	Version     string         `yaml:"version" json:"version"`
	Description string         `yaml:"description" json:"description"`
	Author      string         `yaml:"author,omitempty" json:"author,omitempty"`
	License     string         `yaml:"license,omitempty" json:"license,omitempty"`
	Entrypoint  string         `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Membranes   []MembraneRule `yaml:"membranes,omitempty" json:"membranes,omitempty"`
	Agents      []AgentConfig  `yaml:"agents" json:"agents"`
	Tasks       []TaskConfig   `yaml:"tasks,omitempty" json:"tasks,omitempty"`
}

// MembraneRule specifies path-level boundary restrictions for the swarm.
type MembraneRule struct {
	Path string `yaml:"path" json:"path"`
	Mode string `yaml:"mode" json:"mode"` // read_only, disallow, audit_log
}

// AgentConfig defines an agent persona and configuration within the swarm.
type AgentConfig struct {
	Name         string   `yaml:"name" json:"name"`
	Role         string   `yaml:"role" json:"role"`
	Skills       []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	SystemPrompt string   `yaml:"system_prompt,omitempty" json:"system_prompt,omitempty"`
}

// TaskConfig defines a unit of work in the swarm pipeline.
type TaskConfig struct {
	ID        string   `yaml:"id" json:"id"`
	Title     string   `yaml:"title" json:"title"`
	Role      string   `yaml:"role,omitempty" json:"role,omitempty"`
	DependsOn []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
}

// ParseManifest parses raw YAML or JSON data into a SwarmPackage.
func ParseManifest(data []byte) (*SwarmPackage, error) {
	var pkg SwarmPackage
	if err := yaml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse swarm package manifest: %w", err)
	}

	if err := ValidateManifest(&pkg); err != nil {
		return nil, err
	}

	return &pkg, nil
}

// LoadManifestFile reads and parses a swarm.yaml file from disk.
func LoadManifestFile(filePath string) (*SwarmPackage, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read swarm package file %s: %w", filePath, err)
	}

	return ParseManifest(data)
}

// ValidateManifest checks the semantic integrity of the swarm package.
func ValidateManifest(pkg *SwarmPackage) error {
	if strings.TrimSpace(pkg.Name) == "" {
		return fmt.Errorf("swarm package name is required")
	}
	if strings.TrimSpace(pkg.Version) == "" {
		return fmt.Errorf("swarm package version is required")
	}
	if strings.TrimSpace(pkg.Description) == "" {
		return fmt.Errorf("swarm package description is required")
	}
	if len(pkg.Agents) == 0 {
		return fmt.Errorf("swarm package must define at least one agent")
	}

	for i, a := range pkg.Agents {
		if strings.TrimSpace(a.Name) == "" {
			return fmt.Errorf("agent[%d] name is required", i)
		}
		if strings.TrimSpace(a.Role) == "" {
			return fmt.Errorf("agent[%d] role is required", i)
		}
	}

	for i, m := range pkg.Membranes {
		if strings.TrimSpace(m.Path) == "" {
			return fmt.Errorf("membrane[%d] path is required", i)
		}
		mode := strings.ToLower(strings.TrimSpace(m.Mode))
		if mode != "read_only" && mode != "disallow" && mode != "audit_log" {
			return fmt.Errorf("membrane[%d] invalid mode %q; must be read_only, disallow, or audit_log", i, m.Mode)
		}
	}

	return nil
}

// EnsureSampleSwarm writes a canonical sample swarm.yaml if not present.
func EnsureSampleSwarm(destPath string) error {
	if fileutil.Exists(destPath) {
		return nil
	}
	content := `# Canonical ZQK Portable Swarm Manifest
$schema: https://zqk.dev/schemas/swarm_package_spec.schema.json
name: sample-refactor-swarm
version: 1.0.0
description: Autonomous two-agent swarm for code refactoring and test verification.
author: zqk-community
license: Apache-2.0
entrypoint: task-refactor

membranes:
  - path: .zqk/process/
    mode: read_only

agents:
  - name: Refactor Specialist
    role: software_engineer
    skills:
      - ASK-COMMUNITY-CODE-CRAFTSMAN
    system_prompt: |
      Focus on structural refactoring, AST hygiene, and resource cleanup.
  - name: QA Verifier
    role: qa_auditor
    skills:
      - ASK-COMMUNITY-QA-VERIFICATION
    system_prompt: |
      Verify test execution and criteria satisfaction before terminal handoff.

tasks:
  - id: task-refactor
    title: Refactor target package and ensure leak-free concurrency
    role: software_engineer
  - id: task-verify
    title: Run verification test suite and validate criteria
    role: qa_auditor
    depends_on:
      - task-refactor
`
	return fileutil.WriteFile(destPath, []byte(content), paths.FilePerm644)
}
