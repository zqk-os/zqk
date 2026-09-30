package qa

import (
	"fmt"
	"strings"

)

// Guidance holds actionable remediation steps.
type Guidance struct {
	Summary   string   `json:"summary" yaml:"summary"`
	Steps     []string `json:"steps" yaml:"steps"`
	Reference string   `json:"reference,omitempty" yaml:"reference,omitempty"`
}

// GuidanceEngine maps audit failures to remediation guidance.
type GuidanceEngine struct{}

func NewGuidanceEngine() *GuidanceEngine {
	return &GuidanceEngine{}
}

// Recommend provides specific steps based on failure type and context.
func (e *GuidanceEngine) Recommend(failureType string, context string) Guidance {
	switch failureType {
	case "coverage":
		return Guidance{
			Summary:   "Critical Test Coverage Deficit.",
			Steps:     []string{"Run 'go test -coverprofile=coverage.out' to identify gaps.", "Target the specific module reported in the failure context.", "Implement adversarial test cases that verify both happy and failure paths.", "Threshold for 'Done' is strictly >= 80%."},
			Reference: "docs/best-practices/testing/TDD_STANDARDS.md",
		}
	case "concurrency_violation":
		return Guidance{
			Summary: "Structural Integrity Violation: Unmanaged Concurrency.",
			Steps: []string{
				fmt.Sprintf("Specific Violation: %s", context), "Ensure every 'go func()' propagates 'context.Context'.", "Wrap background goroutines in a 'concurrency.InterruptChecker' or manage them via the 'LifecycleManager'.",
			},
			Reference: "docs/architecture/concurrency/MANAGEMENT.md",
		}
	case "di_violation":
		return Guidance{
			Summary: "Structural Integrity Violation: Dependency Injection.",
			Steps: []string{
				fmt.Sprintf("Specific Violation: %s", context), "Refactor the component to use the 'OrchestratorRegistry' or a factory pattern.", "Avoid direct instantiation of core services (e.g., NewManager, NewFileObjectStorage).",
			},
			Reference: "docs/architecture/patterns/DEPENDENCY_INJECTION.md",
		}
	case "error_handling_debt":
		return Guidance{
			Summary: "Technical Debt: Swallowed Error.",
			Steps: []string{
				fmt.Sprintf("Violation: %s", context), "Do not use the blank identifier '_' for error returns in critical paths.", "Properly handle the error or wrap it using 'errfmt.Newf'.",
			},
		}
	case "dry_violation":
		return Guidance{
			Summary: "DRY Principle Violation: Hardcoded Literal.",
			Steps: []string{
				fmt.Sprintf("Violation: %s", context), "Extract the hardcoded string or numeric literal to a named constant.", "Place constants in a dedicated 'constants.go' or at the top of the package if shared.",
			},
			Reference: "docs/best-practices/coding/DRY_AND_CONSTANTS.md",
		}
	case "duplication_violation", "structural_duplication":
		return Guidance{
			Summary: "DRY Principle Violation: Structural Duplication Detected.",
			Steps: []string{
				fmt.Sprintf("Violation: %s", context),
				"Extract duplicate statement sequence or identical function body into a shared helper function.",
				"Consolidate common logic to eliminate drift and maintain single source of truth.",
			},
			Reference: "docs/best-practices/coding/DRY_AND_CONSTANTS.md",
		}
	case "abstraction_violation":
		if strings.Contains(context, "conditional chain") {
			return Guidance{
				Summary: "Abstraction Violation: Repetitive Conditional Logic.",
				Steps: []string{
					fmt.Sprintf("Specific Violation: %s", context),
					"Refactor the repetitive 'if/else if/else' chain using the 'pkg/when' or 'pkg/functional' fluent utility.", "This improves readability and aligns with our 'code as a story' architectural principle.", "Example: when.When(cond).Then(fn).OrElseWhen(cond).Then(fn).OrElse(fn).Run()",
				},
				Reference: "pkg/when/chain.go",
			}
		}
		return Guidance{
			Summary: "Abstraction Violation: Excessive Complexity/Length.",
			Steps: []string{
				fmt.Sprintf("Violation: %s", context), "The function is too long or complex, violating the single-responsibility principle.", "Refactor by extracting sub-logic into private helper functions.", "Consider if the logic belongs in a different package or should be abstracted behind an interface.",
			},
			Reference: "docs/architecture/patterns/ABSTRACTION.md",
		}
	case "smoke_and_mirrors":
		return Guidance{
			Summary:   "Disparity Detected: Incomplete Implementation.",
			Steps:     []string{"A stub or 'TODO' was found in an object definition that is marked as 'active' or 'complete'.", "Remove the stub and implement the missing functional logic.", "If implementation is deferred, update the object status to 'stalled' or 'planned'."},
			Reference: "docs/onboarding/AI_AGENT_ONBOARDING.md",
		}
	default:
		return Guidance{
			Summary: "General QA Audit Failure.",
			Steps:   []string{"Review the raw audit log in the Knowledge Kernel.", "Consult with the 'adversarial-auditor' for a deep-dive forensic report."},
		}
	}
}
