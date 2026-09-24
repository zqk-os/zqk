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
	Schema               string                  `yaml:"$schema,omitempty" json:"$schema,omitempty"`
	Name                 string                  `yaml:"name" json:"name"`
	Version              string                  `yaml:"version" json:"version"`
	Description          string                  `yaml:"description" json:"description"`
	Author               string                  `yaml:"author,omitempty" json:"author,omitempty"`
	License              string                  `yaml:"license,omitempty" json:"license,omitempty"`
	Entrypoint           string                  `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Parameters           map[string]ParameterDef `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Integrity            *Integrity              `yaml:"integrity,omitempty" json:"integrity,omitempty"`
	TeamConfigurationRef string                  `yaml:"team_configuration_ref,omitempty" json:"team_configuration_ref,omitempty"`
	TeamConfiguration    *TeamConfiguration      `yaml:"team_configuration,omitempty" json:"team_configuration,omitempty"`
	Membranes            []MembraneRule          `yaml:"membranes,omitempty" json:"membranes,omitempty"`
	Agents               []AgentConfig           `yaml:"agents,omitempty" json:"agents,omitempty"`
	Tasks                []TaskConfig            `yaml:"tasks,omitempty" json:"tasks,omitempty"`
}

// TeamConfiguration defines a cellular team archetype to prevent repetitive agent boilerplate.
type TeamConfiguration struct {
	ID                 string              `yaml:"id,omitempty" json:"id,omitempty"`
	CellType           string              `yaml:"cell_type,omitempty" json:"cell_type,omitempty"` // neuron, muscle, heart, lungs
	FocusArea          string              `yaml:"focus_area,omitempty" json:"focus_area,omitempty"`
	PersonaAllocations []PersonaAllocation `yaml:"persona_allocations,omitempty" json:"persona_allocations,omitempty"`
}

// PersonaAllocation specifies a persona and count within a team configuration.
type PersonaAllocation struct {
	PersonaRef string `yaml:"persona_ref" json:"persona_ref"`
	Role       string `yaml:"role,omitempty" json:"role,omitempty"`
	Count      int    `yaml:"count,omitempty" json:"count,omitempty"`
}

// ParameterDef defines a configurable parameter for a swarm package.
type ParameterDef struct {
	Type        string      `yaml:"type" json:"type"`
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool        `yaml:"required,omitempty" json:"required,omitempty"`
}

// Integrity records cryptographic attestation and content hashes.
type Integrity struct {
	Algorithm     string `yaml:"algorithm" json:"algorithm"`
	SignerID      string `yaml:"signer_id" json:"signer_id"`
	ContentDigest string `yaml:"content_digest" json:"content_digest"`
	Signature     string `yaml:"signature" json:"signature"`
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
	if _, err := ParseSemVer(pkg.Version); err != nil {
		return fmt.Errorf("swarm package version invalid: %w", err)
	}
	if strings.TrimSpace(pkg.Description) == "" {
		return fmt.Errorf("swarm package description is required")
	}

	hasAgents := len(pkg.Agents) > 0
	hasTeamRef := strings.TrimSpace(pkg.TeamConfigurationRef) != ""
	hasTeamConfig := pkg.TeamConfiguration != nil

	if !hasAgents && !hasTeamRef && !hasTeamConfig {
		return fmt.Errorf("swarm package must define either agents (ad-hoc personas), team_configuration_ref, or team_configuration")
	}

	for i, a := range pkg.Agents {
		if strings.TrimSpace(a.Name) == "" {
			return fmt.Errorf("agent[%d] name is required", i)
		}
		if strings.TrimSpace(a.Role) == "" {
			return fmt.Errorf("agent[%d] role is required", i)
		}
	}

	if hasTeamConfig && pkg.TeamConfiguration.CellType != "" {
		ct := strings.ToLower(strings.TrimSpace(pkg.TeamConfiguration.CellType))
		if ct != "neuron" && ct != "muscle" && ct != "heart" && ct != "lungs" {
			return fmt.Errorf("invalid team_configuration cell_type %q; must be neuron, muscle, heart, or lungs", pkg.TeamConfiguration.CellType)
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
