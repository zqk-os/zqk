package community

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/bundle"
)

// PackGateStatus defines the outcome of a process pack verification gate.
type PackGateStatus string

const (
	PackGatePass    PackGateStatus = "pass"
	PackGateFail    PackGateStatus = "fail"
	PackGateWarning PackGateStatus = "warning"
)

// GateFinding captures a specific rule or policy violation detected during verification.
type GateFinding struct {
	RuleID      string `json:"rule_id"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	TargetFile  string `json:"target_file,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// EvaluationResult encapsulates the results of evaluating a bundle manifest and its rules.
type EvaluationResult struct {
	BundleName string         `json:"bundle_name"`
	Language   string         `json:"language"`
	Version    string         `json:"version"`
	Status     PackGateStatus `json:"status"`
	Findings   []GateFinding  `json:"findings"`
}

// PackVerificationReport aggregates evaluations across all installed or active packs.
type PackVerificationReport struct {
	TotalPacks EvaluatedCount     `json:"evaluated"`
	Passed     int                `json:"passed"`
	Failed     int                `json:"failed"`
	Results    []EvaluationResult `json:"results"`
}

// EvaluatedCount tracks the count of distinct evaluated bundles.
type EvaluatedCount struct {
	Total int `json:"total"`
}

// ProcessPackGateEvaluator audits language bundles and process packs against workspace policies.
type ProcessPackGateEvaluator struct {
	WorkspaceRoot   string
	EnforceCIGates  bool
	MaxDependencies int
}

// NewProcessPackGateEvaluator constructs a fail-closed pack gate evaluator.
func NewProcessPackGateEvaluator(workspaceRoot string) *ProcessPackGateEvaluator {
	return &ProcessPackGateEvaluator{
		WorkspaceRoot:   filepath.Clean(workspaceRoot),
		EnforceCIGates:  true,
		MaxDependencies: 16,
	}
}

// EvaluateBundle verifies a single language bundle manifest against static invariants and boundary rules.
func (e *ProcessPackGateEvaluator) EvaluateBundle(manifest *bundle.LanguageBundleManifest) (*EvaluationResult, error) {
	if manifest == nil {
		return nil, errors.New("cannot evaluate nil manifest")
	}

	result := &EvaluationResult{
		BundleName: manifest.Name,
		Language:   manifest.Language,
		Version:    manifest.Version,
		Status:     PackGatePass,
		Findings:   make([]GateFinding, 0),
	}

	if err := manifest.Validate(); err != nil {
		result.Status = PackGateFail
		result.Findings = append(result.Findings, GateFinding{
			RuleID:      "BUNDLE-STATIC-001",
			Severity:    "error",
			Message:     fmt.Sprintf("manifest schema validation failed: %v", err),
			Remediation: "Fix manifest schema fields and remove path traversal patterns",
		})
		return result, nil
	}

	// Boundary 1: Dependency depth limit
	if len(manifest.Dependencies) > e.MaxDependencies {
		result.Status = PackGateFail
		result.Findings = append(result.Findings, GateFinding{
			RuleID:      "BUNDLE-LIMIT-002",
			Severity:    "error",
			Message:     fmt.Sprintf("bundle exceeds max dependency limit of %d (got %d)", e.MaxDependencies, len(manifest.Dependencies)),
			Remediation: "Flatten bundle dependencies or prune circular chains",
		})
	}

	// Boundary 2: Agent directive destination safety
	for _, directive := range manifest.AgentDirectives {
		if directive.Path != "" {
			cleanPath := filepath.Clean(directive.Path)
			if filepath.IsAbs(cleanPath) || strings.HasPrefix(cleanPath, "..") {
				result.Status = PackGateFail
				result.Findings = append(result.Findings, GateFinding{
					RuleID:      "BUNDLE-SEC-003",
					Severity:    "error",
					Message:     fmt.Sprintf("unauthorized directive path escape: %q", directive.Path),
					TargetFile:  directive.Path,
					Remediation: "Ensure directive path is relative and within workspace boundary",
				})
			}
		}
	}

	// Boundary 3: Verify execution gates if enforced
	if e.EnforceCIGates {
		for _, gate := range manifest.Gates {
			if gate.EnforceCI && strings.TrimSpace(gate.Command) == "" {
				result.Status = PackGateFail
				result.Findings = append(result.Findings, GateFinding{
					RuleID:      "BUNDLE-GATE-004",
					Severity:    "error",
					Message:     fmt.Sprintf("CI-enforced gate %q missing command", gate.Name),
					Remediation: "Specify executable command for CI-enforced gate",
				})
			}
		}
	}

	return result, nil
}

// EvaluateManifests processes a slice of bundle manifests and returns an aggregated report.
func (e *ProcessPackGateEvaluator) EvaluateManifests(manifests []*bundle.LanguageBundleManifest) (*PackVerificationReport, error) {
	if manifests == nil {
		return nil, errors.New("cannot evaluate nil manifests slice")
	}

	report := &PackVerificationReport{
		TotalPacks: EvaluatedCount{Total: len(manifests)},
		Results:    make([]EvaluationResult, 0, len(manifests)),
	}

	for _, m := range manifests {
		eval, err := e.EvaluateBundle(m)
		if err != nil {
			return nil, err
		}
		if eval.Status == PackGatePass {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, *eval)
	}

	return report, nil
}
