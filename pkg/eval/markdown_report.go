package eval

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FormatEvalResultMarkdown renders an EvalResult as a deterministic GitHub-Flavored Markdown
// scorecard suitable for PR comments, CI job summaries ($GITHUB_STEP_SUMMARY), and executive reviews.
func FormatEvalResultMarkdown(res *EvalResult) string {
	if res == nil {
		return ""
	}
	verdict := "PASS"
	badge := "✅"
	if !res.Passed {
		verdict = "FAIL"
		badge = "❌"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## %s ZQK Evaluation Scorecard: `%s` (%s)\n\n", badge, res.TemplateID, verdict)
	fmt.Fprintf(&b, "- **Target:** `%s`\n", res.Target)
	fmt.Fprintf(&b, "- **Composite Score:** `%.2f`\n", res.CompositeScore)
	if !res.AssessedAt.IsZero() {
		fmt.Fprintf(&b, "- **Assessed At:** `%s`\n", res.AssessedAt.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "- **Violations:** `%d`\n", len(res.Violations))

	if len(res.DimensionScores) > 0 {
		keys := make([]string, 0, len(res.DimensionScores))
		for k := range res.DimensionScores {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		b.WriteString("\n### Dimension Scores\n\n")
		b.WriteString("| Dimension | Score |\n|---|---:|\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "| `%s` | %.2f |\n", k, res.DimensionScores[k])
		}
	}

	if len(res.Violations) > 0 {
		b.WriteString("\n### Violations\n\n")
		b.WriteString("| Severity | File | Rule | Message |\n|---|---|---|---|\n")
		for _, v := range res.Violations {
			fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %s |\n", v.Severity, v.File, v.Rule, v.Message)
		}
	}

	if len(res.Remediations) > 0 {
		b.WriteString("\n### Remediations\n\n")
		for _, r := range res.Remediations {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}

	return b.String()
}
