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

// FormatMatrixResultMarkdown renders a multi-scenario, multi-model MatrixResult as a
// deterministic GitHub-Flavored Markdown convergence report.
func FormatMatrixResultMarkdown(res *MatrixResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	title := "ZQK Evaluation Matrix Convergence Report"
	if strings.TrimSpace(res.SessionID) != "" {
		fmt.Fprintf(&b, "## %s (`%s`)\n\n", title, res.SessionID)
	} else {
		fmt.Fprintf(&b, "## %s\n\n", title)
	}
	fmt.Fprintf(&b, "- **Scenarios Passed:** `%d / %d`\n", res.PassedScenarios, res.TotalScenarios)
	fmt.Fprintf(&b, "- **Average Composite Score:** `%.2f`\n", res.AverageCompositeScore)
	if !res.AssessedAt.IsZero() {
		fmt.Fprintf(&b, "- **Assessed At:** `%s`\n", res.AssessedAt.UTC().Format(time.RFC3339))
	}

	if len(res.ModelConvergence) > 0 {
		models := make([]string, 0, len(res.ModelConvergence))
		for m := range res.ModelConvergence {
			models = append(models, m)
		}
		sort.Strings(models)

		b.WriteString("\n### Model Convergence\n\n")
		b.WriteString("| Model | Runs | Passed | Pass Rate | Mean Score | Variance |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|\n")
		for _, m := range models {
			s := res.ModelConvergence[m]
			if s == nil {
				continue
			}
			fmt.Fprintf(&b, "| `%s` | %d | %d | %.1f%% | %.2f | %.3f |\n",
				m, s.RunsCount, s.PassedCount, s.PassRate*100.0, s.MeanScore, s.ScoreVariance)
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
