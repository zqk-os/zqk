package pack

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
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
	Goal                 *GoalConfig             `yaml:"goal,omitempty" json:"goal,omitempty"`
	TemplateMapping      map[string]string       `yaml:"template_mapping,omitempty" json:"template_mapping,omitempty"`
	Agents               []AgentConfig           `yaml:"agents,omitempty" json:"agents,omitempty"`
	Tasks                []TaskConfig            `yaml:"tasks,omitempty" json:"tasks,omitempty"`
}

// GoalConfig defines explicit goal attributes for the swarm package.
type GoalConfig struct {
	Metric      string `yaml:"metric,omitempty" json:"metric,omitempty"`
	Target      string `yaml:"target,omitempty" json:"target,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
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
	ID             string   `yaml:"id" json:"id"`
	Title          string   `yaml:"title" json:"title"`
	Role           string   `yaml:"role,omitempty" json:"role,omitempty"`
	DependsOn      []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Template       string   `yaml:"template,omitempty" json:"template,omitempty"`
	TemplateRef    string   `yaml:"template_ref,omitempty" json:"template_ref,omitempty"`
	PromptTemplate string   `yaml:"prompt_template,omitempty" json:"prompt_template,omitempty"`
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

// SwarmDraftOptions configures generation of a draft swarm package manifest.
type SwarmDraftOptions struct {
	Name            string
	Version         string
	Description     string
	Author          string
	License         string
	CellType        string
	TeamRef         string
	GoalMetric      string
	GoalTarget      string
	GoalDescription string
}

// DraftSwarmManifestYAML returns a valid swarm.yaml document for editing before sealing or running.
func DraftSwarmManifestYAML(opts SwarmDraftOptions) ([]byte, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "custom-swarm"
	}
	ver := strings.TrimSpace(opts.Version)
	if ver == "" {
		ver = "1.0.0"
	}
	if _, err := ParseSemVer(ver); err != nil {
		return nil, fmt.Errorf("invalid swarm version: %w", err)
	}
	desc := strings.TrimSpace(opts.Description)
	if desc == "" {
		desc = "Autonomous multi-agent swarm pipeline."
	}
	author := strings.TrimSpace(opts.Author)
	if author == "" {
		author = "zqk-community"
	}
	license := strings.TrimSpace(opts.License)
	if license == "" {
		license = "Apache-2.0"
	}

	pkg := &SwarmPackage{
		Schema:      "https://zqk.dev/schemas/swarm_package_spec.schema.json",
		Name:        name,
		Version:     ver,
		Description: desc,
		Author:      author,
		License:     license,
		Entrypoint:  "task-execute",
		Membranes: []MembraneRule{
			{Path: paths.ProcessDir + "/", Mode: "read_only"},
		},
		Parameters: map[string]ParameterDef{
			"baseline": {
				Type:        "float",
				Default:     4.5,
				Description: "Target quality baseline",
				Required:    false,
			},
		},
		Tasks: []TaskConfig{
			{
				ID:    "task-execute",
				Title: "Execute pipeline implementation and code changes",
				Role:  "lead_integrator",
			},
			{
				ID:        "task-verify",
				Title:     "Run verification test suite and validate acceptance criteria",
				Role:      "qa_auditor",
				DependsOn: []string{"task-execute"},
			},
		},
	}

	if strings.TrimSpace(opts.CellType) != "" {
		ct := strings.ToLower(strings.TrimSpace(opts.CellType))
		pkg.TeamConfiguration = &TeamConfiguration{
			ID:        name + "-team",
			CellType:  ct,
			FocusArea: desc,
			PersonaAllocations: []PersonaAllocation{
				{
					PersonaRef: "PER-DEFAULT-OPERATOR",
					Role:       "cell_lead",
					Count:      1,
				},
			},
		}
		pkg.Tasks[0].Role = "cell_lead"
	} else if strings.TrimSpace(opts.TeamRef) != "" {
		pkg.TeamConfigurationRef = strings.TrimSpace(opts.TeamRef)
	} else {
		pkg.Agents = []AgentConfig{
			{
				Name:         "Lead Integrator",
				Role:         "lead_integrator",
				Skills:       []string{"ASK-COMMUNITY-CODE-CRAFTSMAN"},
				SystemPrompt: "Coordinate task decomposition, code crafting, and structural refactoring.\n",
			},
			{
				Name:         "QA Verifier",
				Role:         "qa_auditor",
				Skills:       []string{"ASK-COMMUNITY-QA-VERIFICATION"},
				SystemPrompt: "Execute verification test suite and validate acceptance criteria satisfaction.\n",
			},
		}
	}

	if strings.TrimSpace(opts.GoalMetric) != "" || strings.TrimSpace(opts.GoalTarget) != "" || strings.TrimSpace(opts.GoalDescription) != "" {
		pkg.Goal = &GoalConfig{
			Metric:      strings.TrimSpace(opts.GoalMetric),
			Target:      strings.TrimSpace(opts.GoalTarget),
			Description: strings.TrimSpace(opts.GoalDescription),
		}
	}

	if err := ValidateManifest(pkg); err != nil {
		return nil, fmt.Errorf("generated swarm manifest is invalid: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(paths.RewriteCanonicalCLIInvocations("# Canonical ZQK Portable Swarm Manifest (zqk new swarm)\n" +
		"# Next steps:\n" +
		"#   1. Edit agent personas, tasks, dependencies, and membranes as needed.\n" +
		"#   2. Cryptographically seal: zqk pack seal <dir>\n" +
		"#   3. Execute swarm: zqk run <dir>\n\n"))

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(pkg); err != nil {
		return nil, fmt.Errorf("encode swarm manifest: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	// Verify round-trip parse
	if _, err := ParseManifest(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("generated manifest failed roundtrip validation: %w", err)
	}

	return buf.Bytes(), nil
}
