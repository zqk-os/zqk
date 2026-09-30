package qa

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/validation"
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
			Summary:   validation.ConstMagic14f2f5cc,
			Steps:     []string{validation.ConstMagicExtracted_30, validation.ConstMagicExtracted_31, validation.ConstMagicExtracted_32, validation.ConstMagicExtracted_33},
			Reference: "docs/best-practices/testing/TDD_STANDARDS.md",
		}
	case validation.ConstMagicExtracted_34:
		return Guidance{
			Summary: validation.ConstMagic25218325,
			Steps: []string{
				fmt.Sprintf(validation.ConstMagic88551cec, context), validation.ConstMagicExtracted_35, validation.ConstMagicExtracted_36,
			},
			Reference: "docs/architecture/concurrency/MANAGEMENT.md",
		}
	case "di_violation":
		return Guidance{
			Summary: validation.ConstMagicc82d0cca,
			Steps: []string{
				fmt.Sprintf(validation.ConstMagic88551cec, context), validation.ConstMagicExtracted_37, validation.ConstMagicExtracted_38,
			},
			Reference: "docs/architecture/patterns/DEPENDENCY_INJECTION.md",
		}
	case validation.ConstMagicExtracted_39:
		return Guidance{
			Summary: validation.ConstMagicb5962b4e,
			Steps: []string{
				fmt.Sprintf("Violation: %s", context), validation.ConstMagicExtracted_40, validation.ConstMagicExtracted_41,
			},
		}
	case "dry_violation":
		return Guidance{
			Summary: validation.ConstMagic887ac3c2,
			Steps: []string{
				fmt.Sprintf("Violation: %s", context), validation.ConstMagicExtracted_42, validation.ConstMagicExtracted_43,
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
	case validation.ConstMagicExtracted_44:
		if strings.Contains(context, validation.ConstMagiccd1ac283) {
			return Guidance{
				Summary: validation.ConstMagic8a57c7e1,
				Steps: []string{
					fmt.Sprintf(validation.ConstMagic88551cec, context),
					"Refactor the repetitive 'if/else if/else' chain using the 'pkg/when' or 'pkg/functional' fluent utility.", validation.ConstMagicExtracted_45, validation.ConstMagicExtracted_46,
				},
				Reference: "pkg/when/chain.go",
			}
		}
		return Guidance{
			Summary: "Abstraction Violation: Excessive Complexity/Length.",
			Steps: []string{
				fmt.Sprintf("Violation: %s", context), validation.ConstMagicExtracted_47, validation.ConstMagicExtracted_48, validation.ConstMagicExtracted_49,
			},
			Reference: "docs/architecture/patterns/ABSTRACTION.md",
		}
	case validation.ConstMagicExtracted_50:
		return Guidance{
			Summary:   validation.ConstMagic3b158fda,
			Steps:     []string{validation.ConstMagicExtracted_51, validation.ConstMagicExtracted_52, validation.ConstMagicExtracted_53},
			Reference: "docs/onboarding/AI_AGENT_ONBOARDING.md",
		}
	default:
		return Guidance{
			Summary: validation.ConstMagic23002f73,
			Steps:   []string{validation.ConstMagicExtracted_54, validation.ConstMagicExtracted_55},
		}
	}
}
