package testdiscovery

import (
	"fmt"
	"sort"
	"strings"
)

// FormatDiscoveredTargetsMarkdown renders discovered test targets as a deterministic
// GitHub-Flavored Markdown inventory table grouped by language.
func FormatDiscoveredTargetsMarkdown(targets []DiscoveredTarget) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## ZQK Discovered Test Inventory (%d targets)\n\n", len(targets))
	if len(targets) == 0 {
		b.WriteString("_No test targets discovered._\n")
		return b.String()
	}

	sorted := append([]DiscoveredTarget(nil), targets...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Language != sorted[j].Language {
			return sorted[i].Language < sorted[j].Language
		}
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line < sorted[j].Line
		}
		return sorted[i].Function < sorted[j].Function
	})

	b.WriteString("| Language | Location | Test Function | Criteria |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, t := range sorted {
		criteria := "-"
		if len(t.CriteriaRefs) > 0 {
			criteria = strings.Join(t.CriteriaRefs, ", ")
		}
		fmt.Fprintf(&b, "| `%s` | `%s:%d` | `%s` | %s |\n",
			t.Language, t.Path, t.Line, t.Function, criteria)
	}
	return b.String()
}
