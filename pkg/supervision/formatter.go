package supervision

import (
	"fmt"
	"strings"
)

// FormatContrastiveRetryPrompt constructs an illuminated retry prompt that explicitly presents
// the structured failure delta, negative hypotheses (invalidated paths), canonical exemplars,
// and progressive scope narrowing.
func FormatContrastiveRetryPrompt(basePrompt string, env SupervisionEnvelope) string {
	var sb strings.Builder

	sb.WriteString(basePrompt)
	sb.WriteString("\n\n---\n")
	sb.WriteString(fmt.Sprintf("## Adaptive Task Supervision: Attempt %d of %d (Failure Delta Illumination)\n", env.Attempt, env.MaxAttempts))
	sb.WriteString("> [!IMPORTANT]\n")
	sb.WriteString("> A previous attempt for this task did not pass verification. Review the focused diagnostic delta below.\n")
	sb.WriteString("> Do **NOT** repeat invalidated hypotheses or cyclical blind attempts. Narrow the scope and address the localized error directly.\n\n")

	// 1. Diagnostic Error Delta
	if env.Diagnostic != nil {
		diag := env.Diagnostic
		sb.WriteString("### 1. Focused Diagnostic Delta\n")
		sb.WriteString(fmt.Sprintf("- **Failing Phase:** `%s`\n", diag.Phase))
		sb.WriteString(fmt.Sprintf("- **Exit Code:** `%d`\n", diag.ExitCode))
		if diag.FailureAnchor != "" {
			sb.WriteString(fmt.Sprintf("- **Failure Anchor / Symbol:** `%s`\n", diag.FailureAnchor))
		}
		if diag.ObservedAnomaly != "" {
			sb.WriteString(fmt.Sprintf("- **Observed Anomaly:** %s\n", diag.ObservedAnomaly))
		}
		sb.WriteString("\n**Diagnostic Error Excerpt:**\n```text\n")
		sb.WriteString(strings.TrimSpace(diag.FocusedDiagnostic))
		sb.WriteString("\n```\n\n")
	}

	// 2. Negative Hypotheses (Pruning Invalidated Paths)
	if len(env.NegativeBoundaries) > 0 {
		sb.WriteString("### 2. Negative Hypotheses (Invalidated Paths to Avoid)\n")
		for _, nh := range env.NegativeBoundaries {
			sb.WriteString(fmt.Sprintf("- **Attempt %d Invalidation:** %s\n", nh.Attempt, nh.InvalidatedApproach))
			for _, forbidden := range nh.ForbiddenActions {
				sb.WriteString(fmt.Sprintf("  - *Constraint:* %s\n", forbidden))
			}
		}
		sb.WriteString("\n")
	}

	// 3. Canonical Exemplars
	if len(env.Exemplars) > 0 {
		sb.WriteString("### 3. Canonical Reference Exemplars\n")
		for _, ex := range env.Exemplars {
			sb.WriteString(fmt.Sprintf("#### Pattern: %s (%s)\n", ex.Title, ex.Category))
			if ex.Description != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", ex.Description))
			}
			if ex.PatternCode != "" {
				sb.WriteString("```\n")
				sb.WriteString(strings.TrimSpace(ex.PatternCode))
				sb.WriteString("\n```\n\n")
			}
			if ex.AntiPatternCode != "" {
				sb.WriteString("**Avoid (Anti-Pattern):**\n```\n")
				sb.WriteString(strings.TrimSpace(ex.AntiPatternCode))
				sb.WriteString("\n```\n\n")
			}
		}
	}

	// 4. Progressive Scope Narrowing
	if env.ProgressiveScope != nil {
		ps := env.ProgressiveScope
		sb.WriteString("### 4. Progressive Scope Narrowing & Tight Verification Loop\n")
		if ps.FocusedTarget != "" {
			sb.WriteString(fmt.Sprintf("- **Target Scope:** `%s`\n", ps.FocusedTarget))
		}
		if ps.IterationRationale != "" {
			sb.WriteString(fmt.Sprintf("- **Rationale:** %s\n", ps.IterationRationale))
		}
		if ps.SuggestedSubGateCommand != "" {
			sb.WriteString(fmt.Sprintf("- **Suggested Sub-Gate Verification Command:**\n  ```bash\n  %s\n  ```\n", ps.SuggestedSubGateCommand))
		}
		sb.WriteString("- Iterate against the targeted sub-gate first before running the entire verification suite.\n\n")
	}

	return sb.String()
}
