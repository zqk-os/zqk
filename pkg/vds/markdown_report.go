package vds

import (
	"fmt"
	"strings"
)

// FormatReportMarkdown renders a deterministic GitHub-Flavored Markdown
// scorecard for a VDS evaluation Report suitable for CI job summaries
// ($GITHUB_STEP_SUMMARY) and pull request comments.
func FormatReportMarkdown(rep *Report) string {
	if rep == nil {
		return "_No VDS evaluation report available._\n"
	}

	var b strings.Builder
	verdict := rep.Verdict
	if strings.TrimSpace(verdict) == "" {
		verdict = "UNKNOWN"
	}

	fmt.Fprintf(&b, "## ZQK VDS Done-Gate Report — `%s`\n\n", verdict)
	if rep.PolicyID != "" {
		fmt.Fprintf(&b, "- **Policy:** `%s`\n", rep.PolicyID)
	}
	if rep.SpineProfileID != "" {
		fmt.Fprintf(&b, "- **Spine Profile:** `%s`\n", rep.SpineProfileID)
	}
	if rep.CustomizationID != "" {
		fmt.Fprintf(&b, "- **Customization:** `%s`\n", rep.CustomizationID)
	}
	fmt.Fprintf(&b, "- **Chunks Evaluated:** `%d` (Failed: `%d`)\n\n", len(rep.Chunks), len(rep.FailedChunkIDs))

	if len(rep.Chunks) > 0 {
		b.WriteString("### Chunks\n\n")
		b.WriteString("| Chunk ID | Stage | Verdict | Checklist Passed | Claim |\n")
		b.WriteString("|---|---|---|---:|---|\n")
		for _, ch := range rep.Chunks {
			passed := 0
			for _, item := range ch.Checklist {
				if item.OK {
					passed++
				}
			}
			claim := strings.ReplaceAll(strings.TrimSpace(ch.Claim), "|", "\\|")
			fmt.Fprintf(&b, "| `%s` | `%s` | **%s** | %d / %d | %s |\n",
				ch.ChunkID, ch.Stage, ch.Verdict, passed, len(ch.Checklist), claim)
		}
		b.WriteString("\n")
	}

	if len(rep.NextActions) > 0 {
		b.WriteString("### Next Actions\n\n")
		for _, action := range rep.NextActions {
			fmt.Fprintf(&b, "- %s\n", action)
		}
	}

	return b.String()
}
