package quality

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CEFPromptDefinition pairs the source markdown path with metadata for conversion.
type CEFPromptDefinition struct {
	ID        string
	Title     string
	Category  string
	Role      string // specialist, adversarial, coordinator
	Lens      string
	RelPath   string
}

// Canonical list of the 25 CEF prompt files
var CEFPromptRegistry = []CEFPromptDefinition{
	{"PROMPT-CEF-PREAMBLE", "CEF Shared Preamble", "framework", "coordinator", "FRAMEWORK", "prompts/_SHARED_PREAMBLE.md"},
	{"PROMPT-CEF-KICKOFF", "CEF Kickoff & Scope", "framework", "coordinator", "FRAMEWORK", "KICKOFF_PROMPT.md"},
	{"PROMPT-CEF-INTEGRATOR-SPEC", "Integrator Specialist Prompt", "synthesis", "coordinator", "L-INTEGRATOR", "prompts/L-INTEGRATOR/specialist.md"},
	{"PROMPT-CEF-CODE-QUALITY-SPEC", "Code Quality Specialist Prompt", "evaluation", "specialist_evaluator", "L-CODE-QUALITY", "prompts/L-CODE-QUALITY/specialist.md"},
	{"PROMPT-CEF-CODE-QUALITY-ADV", "Code Quality Adversarial Prompt", "evaluation", "adversarial_auditor", "L-CODE-QUALITY", "prompts/L-CODE-QUALITY/adversarial.md"},
	{"PROMPT-CEF-ARCHITECTURE-SPEC", "Architecture Specialist Prompt", "evaluation", "specialist_evaluator", "L-ARCHITECTURE", "prompts/L-ARCHITECTURE/specialist.md"},
	{"PROMPT-CEF-ARCHITECTURE-ADV", "Architecture Adversarial Prompt", "evaluation", "adversarial_auditor", "L-ARCHITECTURE", "prompts/L-ARCHITECTURE/adversarial.md"},
	{"PROMPT-CEF-CONCURRENCY-SPEC", "Concurrency Specialist Prompt", "evaluation", "specialist_evaluator", "L-CONCURRENCY", "prompts/L-CONCURRENCY/specialist.md"},
	{"PROMPT-CEF-CONCURRENCY-ADV", "Concurrency Adversarial Prompt", "evaluation", "adversarial_auditor", "L-CONCURRENCY", "prompts/L-CONCURRENCY/adversarial.md"},
	{"PROMPT-CEF-TESTING-SPEC", "Testing Specialist Prompt", "evaluation", "specialist_evaluator", "L-TESTING", "prompts/L-TESTING/specialist.md"},
	{"PROMPT-CEF-TESTING-ADV", "Testing Adversarial Prompt", "evaluation", "adversarial_auditor", "L-TESTING", "prompts/L-TESTING/adversarial.md"},
	{"PROMPT-CEF-SECURITY-SPEC", "Security Specialist Prompt", "evaluation", "specialist_evaluator", "L-SECURITY", "prompts/L-SECURITY/specialist.md"},
	{"PROMPT-CEF-SECURITY-ADV", "Security Adversarial Prompt", "evaluation", "adversarial_auditor", "L-SECURITY", "prompts/L-SECURITY/adversarial.md"},
	{"PROMPT-CEF-RELIABILITY-SPEC", "Reliability Specialist Prompt", "evaluation", "specialist_evaluator", "L-RELIABILITY", "prompts/L-RELIABILITY/specialist.md"},
	{"PROMPT-CEF-RELIABILITY-ADV", "Reliability Adversarial Prompt", "evaluation", "adversarial_auditor", "L-RELIABILITY", "prompts/L-RELIABILITY/adversarial.md"},
	{"PROMPT-CEF-PERFORMANCE-SPEC", "Performance Specialist Prompt", "evaluation", "specialist_evaluator", "L-PERFORMANCE", "prompts/L-PERFORMANCE/specialist.md"},
	{"PROMPT-CEF-PERFORMANCE-ADV", "Performance Adversarial Prompt", "evaluation", "adversarial_auditor", "L-PERFORMANCE", "prompts/L-PERFORMANCE/adversarial.md"},
	{"PROMPT-CEF-OBSERVABILITY-SPEC", "Observability Specialist Prompt", "evaluation", "specialist_evaluator", "L-OBSERVABILITY", "prompts/L-OBSERVABILITY/specialist.md"},
	{"PROMPT-CEF-OBSERVABILITY-ADV", "Observability Adversarial Prompt", "evaluation", "adversarial_auditor", "L-OBSERVABILITY", "prompts/L-OBSERVABILITY/adversarial.md"},
	{"PROMPT-CEF-SUPPLY-RELEASE-SPEC", "Supply & Release Specialist Prompt", "evaluation", "specialist_evaluator", "L-SUPPLY-RELEASE", "prompts/L-SUPPLY-RELEASE/specialist.md"},
	{"PROMPT-CEF-SUPPLY-RELEASE-ADV", "Supply & Release Adversarial Prompt", "evaluation", "adversarial_auditor", "L-SUPPLY-RELEASE", "prompts/L-SUPPLY-RELEASE/adversarial.md"},
	{"PROMPT-CEF-USABILITY-SPEC", "Usability Specialist Prompt", "evaluation", "specialist_evaluator", "L-USABILITY", "prompts/L-USABILITY/specialist.md"},
	{"PROMPT-CEF-USABILITY-ADV", "Usability Adversarial Prompt", "evaluation", "adversarial_auditor", "L-USABILITY", "prompts/L-USABILITY/adversarial.md"},
	{"PROMPT-CEF-DOCS-MODEL-SPEC", "Docs & Domain Model Specialist Prompt", "evaluation", "specialist_evaluator", "L-DOCS-MODEL", "prompts/L-DOCS-MODEL/specialist.md"},
	{"PROMPT-CEF-DOCS-MODEL-ADV", "Docs & Domain Model Adversarial Prompt", "evaluation", "adversarial_auditor", "L-DOCS-MODEL", "prompts/L-DOCS-MODEL/adversarial.md"},
}

// ConvertCEFPrompts reads markdown prompt files from cefSourceDir, validates against stubs,
// and creates structured CAS prompt_template objects in targetTemplatesDir.
func ConvertCEFPrompts(cefSourceDir string, targetTemplatesDir string) ([]metabolism.PromptTemplateObject, error) {
	if err := os.MkdirAll(targetTemplatesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create target templates dir: %w", err)
	}

	bannedStubs := []string{"UNGRADED", "F-STUB-000", "REPLACE_ME"}
	var templates []metabolism.PromptTemplateObject

	for _, def := range CEFPromptRegistry {
		srcFile := filepath.Join(cefSourceDir, def.RelPath)
		data, err := os.ReadFile(srcFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CEF source prompt %s: %w", srcFile, err)
		}

		rawContent := string(data)

		// Assert zero stubs
		for _, stub := range bannedStubs {
			if strings.Contains(rawContent, stub) {
				return nil, fmt.Errorf("prompt file %s contains banned stub %q", def.RelPath, stub)
			}
		}

		// Inject template placeholders for operational variables
		augmentedContent := rawContent
		if !strings.Contains(augmentedContent, "{{baseline}}") {
			augmentedContent += "\n\n<!-- Operational Parameters: baseline={{baseline}}, output_dir={{output_dir}}, compliance={{compliance}} -->\n"
		}

		tplObj := metabolism.PromptTemplateObject{
			ID:          def.ID,
			Kind:        "prompt_template",
			Title:       def.Title,
			Category:    def.Category,
			Description: fmt.Sprintf("CEF Evaluation Lens template for %s (%s)", def.Lens, def.Role),
			Template:    augmentedContent,
			Variables:   []string{"baseline", "output_dir", "compliance"},
			Metadata: map[string]string{
				"lens":        def.Lens,
				"role":        def.Role,
				"source_file": def.RelPath,
				"framework":   "CEF-v0.1.0",
			},
		}

		outFileName := fmt.Sprintf("%s.yaml", strings.ToLower(strings.ReplaceAll(def.ID, "PROMPT-CEF-", "")))
		destPath := filepath.Join(targetTemplatesDir, outFileName)
		marshaled, err := yaml.Marshal(&tplObj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal prompt template %s: %w", def.ID, err)
		}

		if err := fileutil.WriteFile(destPath, marshaled, paths.FilePerm644); err != nil {
			return nil, fmt.Errorf("failed to write prompt template %s: %w", destPath, err)
		}

		templates = append(templates, tplObj)
	}

	return templates, nil
}

// BuildCanonicalCEFPack assembles the complete canonical code-eval pack in destPackDir and cryptographically seals it.
func BuildCanonicalCEFPack(cefSourceDir string, destPackDir string, privKey ed25519.PrivateKey) (*pack.SwarmPackage, error) {
	if err := os.MkdirAll(destPackDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create pack dir: %w", err)
	}

	templatesDir := filepath.Join(destPackDir, "templates")
	if _, err := ConvertCEFPrompts(cefSourceDir, templatesDir); err != nil {
		return nil, fmt.Errorf("failed to convert CEF prompts: %w", err)
	}

	membranesDir := filepath.Join(destPackDir, "membranes")
	if err := os.MkdirAll(membranesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create membranes dir: %w", err)
	}

	// 4-Wave DAG Definition
	manifest := pack.SwarmPackage{
		Schema:      "https://zqk.dev/schemas/swarm_package_spec.schema.json",
		Name:        "code-eval",
		Version:     "1.0.0",
		Description: "Codebase Evaluation Framework (CEF) 8-Lens diamond quality and adversarial verification swarm",
		Author:      "zqk-community",
		License:     "Apache-2.0",
		Entrypoint:  "wave-1-preflight",
		Parameters: map[string]pack.ParameterDef{
			"baseline": {
				Type:        "float",
				Default:     4.5,
				Description: "Minimum acceptable diamond envelope quality score",
				Required:    false,
			},
			"output_dir": {
				Type:        "string",
				Default:     ".zqk/runs/code-eval-latest",
				Description: "Directory path for catabolic exhaust (findings, scorecards, traces)",
				Required:    false,
			},
			"compliance": {
				Type:        "string",
				Default:     "SOC2,OWASP-TOP10",
				Description: "Comma-separated list of regulatory and compliance overlay constraints",
				Required:    false,
			},
		},
		Membranes: []pack.MembraneRule{
			{Path: ".zqk/process/", Mode: "read_only"},
			{Path: ".zqk/audit/", Mode: "audit_log"},
			{Path: "docs/quality/codebase_evaluation/", Mode: "read_only"},
		},
		TeamConfigurationRef: "TCFG-CEF-DIAMOND-EVALUATION",
		Agents: []pack.AgentConfig{
			{
				Name:         "Specialist Evaluator",
				Role:         "specialist_evaluator",
				Skills:       []string{"ASK-COMMUNITY-CODE-CRAFTSMAN", "ASK-COMMUNITY-ARCH-DESIGN"},
				SystemPrompt: "Execute objective, evidence-driven codebase evaluation across designated lenses.",
			},
			{
				Name:         "Adversarial Auditor",
				Role:         "adversarial_auditor",
				Skills:       []string{"ASK-COMMUNITY-QA-VERIFICATION"},
				SystemPrompt: "Subject findings and code invariants to falsifiable critique, boundary tests, and fail-closed bounds.",
			},
			{
				Name:         "Lead Integrator",
				Role:         "lead_integrator",
				Skills:       []string{"ASK-COMMUNITY-TPM-ORCHESTRATION"},
				SystemPrompt: "Synthesize multi-agent findings, calculate Diamond Scale envelope, and certify convergence.",
			},
		},
		Tasks: []pack.TaskConfig{
			// Wave 1: Preflight
			{
				ID:    "wave-1-preflight",
				Title: "Inspect codebase inventory, tool availability, and define run scope",
				Role:  "lead_integrator",
			},
			// Wave 2: Structural & Correctness Lenses
			{
				ID:        "wave-2-code-quality",
				Title:     "Evaluate Code Quality & Maintainability (MNT)",
				Role:      "specialist_evaluator",
				DependsOn: []string{"wave-1-preflight"},
			},
			{
				ID:        "wave-2-code-quality-critique",
				Title:     "Adversarial critique of Code Quality findings",
				Role:      "adversarial_auditor",
				DependsOn: []string{"wave-2-code-quality"},
			},
			{
				ID:        "wave-2-architecture",
				Title:     "Evaluate Architecture, Package Boundaries & Graph Hygeine (RDB)",
				Role:      "specialist_evaluator",
				DependsOn: []string{"wave-1-preflight"},
			},
			{
				ID:        "wave-2-architecture-critique",
				Title:     "Adversarial critique of Architecture findings",
				Role:      "adversarial_auditor",
				DependsOn: []string{"wave-2-architecture"},
			},
			{
				ID:        "wave-2-security",
				Title:     "Evaluate Security & Threat Vectors (SEC)",
				Role:      "specialist_evaluator",
				DependsOn: []string{"wave-1-preflight"},
			},
			{
				ID:        "wave-2-security-critique",
				Title:     "Adversarial verification of Security findings",
				Role:      "adversarial_auditor",
				DependsOn: []string{"wave-2-security"},
			},
			{
				ID:        "wave-2-testing",
				Title:     "Evaluate Test Strategy & Invariant Proofs (TST)",
				Role:      "specialist_evaluator",
				DependsOn: []string{"wave-1-preflight"},
			},
			{
				ID:        "wave-2-testing-critique",
				Title:     "Adversarial stress-test of Testing assertions",
				Role:      "adversarial_auditor",
				DependsOn: []string{"wave-2-testing"},
			},
			// Wave 3: Operational & Systemic Lenses
			{
				ID:        "wave-3-reliability",
				Title:     "Evaluate Reliability & Error Recovery (REL/ROB)",
				Role:      "specialist_evaluator",
				DependsOn: []string{"wave-2-code-quality-critique", "wave-2-architecture-critique"},
			},
			{
				ID:        "wave-3-observability",
				Title:     "Evaluate Observability & Diagnostics (OBS)",
				Role:      "specialist_evaluator",
				DependsOn: []string{"wave-2-code-quality-critique"},
			},
			// Wave 4: Synthesis & Convergence Certification
			{
				ID:        "wave-4-synthesis",
				Title:     "Synthesize multi-agent findings, calculate Diamond Scale envelope, and check convergence",
				Role:      "lead_integrator",
				DependsOn: []string{"wave-3-reliability", "wave-3-observability"},
			},
		},
	}

	manifestBytes, err := yaml.Marshal(&manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal code-eval manifest: %w", err)
	}

	manifestPath := filepath.Join(destPackDir, "swarm.yaml")
	if err := fileutil.WriteFile(manifestPath, manifestBytes, paths.FilePerm644); err != nil {
		return nil, fmt.Errorf("failed to write code-eval swarm.yaml: %w", err)
	}

	// Seal pack
	if privKey == nil {
		_, privKey, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate key for pack sealing: %w", err)
		}
	}

	if _, err := pack.SealPack(destPackDir, privKey, "cef-curator@zqk.dev"); err != nil {
		return nil, fmt.Errorf("failed to seal code-eval pack: %w", err)
	}

	return pack.LoadManifestFile(manifestPath)
}
