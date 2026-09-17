package system

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// GoalRefsFromObject extracts goal references from an object
func GoalRefsFromObject(obj map[string]any) map[string]bool {
	refs := make(map[string]bool)
	v, ok := obj[objects.FieldKeyGoalRefs]
	if !ok {
		return refs
	}
	v, ok = nildecode.DecodeNonNilPayload[any](v)
	if !ok {
		return refs
	}
	switch t := v.(type) {
	case []any:
		for _, r := range t {
			if s, ok := r.(string); ok && s != "" {
				refs[s] = true
			}
		}
	case []string:
		for _, s := range t {
			if s != "" {
				refs[s] = true
			}
		}
	}
	return refs
}

// RoundTwo rounds a float to two decimal places
func RoundTwo(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// TruncateGaps truncates a list of gaps to a maximum size
func TruncateGaps(items []map[string]any, max int) []map[string]any {
	if len(items) <= max {
		return items
	}
	return items[:max]
}

// OutputAlignDashboard writes a comprehensive alignment dashboard
func OutputAlignDashboard(cmd *cobra.Command, result map[string]any, alignmentScore float64, total, withCount, gapCount, goalsCount int) error {
	format := cli.GetFormat(cmd)
	coverage := 0.0
	if total > 0 {
		coverage = 100 * float64(withCount) / float64(total)
	}
	dashboardData := map[string]any{
		"dashboard":               true,
		"overall_alignment_score": alignmentScore,
		"goal_coverage_pct":       coverage,
		"goal_work": map[string]any{
			"total_work_items":    total,
			"items_with_goals":    withCount,
			"items_without_goals": gapCount,
		},
		"goals_count": goalsCount,
	}

	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, dashboardData)
	default:
		var buf strings.Builder
		headers := []string{"METRIC", "VALUE", "DETAILS"}
		var rows [][]string

		rows = append(rows, []string{"Overall Alignment", fmt.Sprintf("%.1f / 100", alignmentScore), "Aggregate system score"})
		rows = append(rows, []string{"Goal Coverage", fmt.Sprintf("%.1f%%", coverage), fmt.Sprintf("%d of %d items aligned", withCount, total)})
		rows = append(rows, []string{"Goals Defined", fmt.Sprintf("%d", goalsCount), "Total active strategic goals"})
		rows = append(rows, []string{"Alignment Gaps", fmt.Sprintf("%d", gapCount), "Items missing goal alignment"})

		buf.WriteString(clipkg.RenderTableWithTitle("SYSTEM ALIGNMENT DASHBOARD", headers, nil, rows))
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}
}

// OutputAlignTable writes a basic alignment table
func OutputAlignTable(cmd *cobra.Command, result map[string]any, gapsOnly bool, goalID string) error {
	var buf strings.Builder

	align, _ := result["alignment"].(map[string]any)
	gw, _ := align["goal_work"].(map[string]any)
	total, _ := gw["total_work_items"].(int)
	withCount, _ := gw["items_with_goals"].(int)
	gapCount, _ := gw["items_without_goals"].(int)
	coverage, _ := gw["goal_coverage_pct"].(float64)
	score, _ := result["alignment_score"].(float64)

	buf.WriteString("Strategic alignment (goal-work)\n")
	buf.WriteString("--------------------------------\n")
	fmt.Fprintf(&buf, "  Alignment score:       %.1f (0-100)\n", score)
	fmt.Fprintf(&buf, "  Total work items:     %d\n", total)
	fmt.Fprintf(&buf, "  Items with goals:     %d\n", withCount)
	fmt.Fprintf(&buf, "  Items without goals:  %d\n", gapCount)
	fmt.Fprintf(&buf, "  Goal coverage:         %.1f%%\n", coverage)
	if goalID != "" {
		fmt.Fprintf(&buf, "  Goal filter:           %s\n", goalID)
	}
	buf.WriteString("\n")

	if gapsOnly {
		gaps, _ := result["gaps"].([]map[string]any)
		if len(gaps) > 0 {
			buf.WriteString("Items without goal alignment (gaps):\n")
			for _, obj := range gaps {
				id, _ := obj[objects.FieldKeyID].(string)
				title, _ := obj[objects.FieldKeyTitle].(string)
				if title == "" {
					title = "(no title)"
				}
				fmt.Fprintf(&buf, "  - %s %s\n", id, title)
			}
		}
	} else if goalID != "" {
		items, _ := result["aligned_items"].([]map[string]any)
		if len(items) > 0 {
			fmt.Fprintf(&buf, "Items aligned to %s:\n", goalID)
			for _, obj := range items {
				id, _ := obj[objects.FieldKeyID].(string)
				title, _ := obj[objects.FieldKeyTitle].(string)
				if title == "" {
					title = "(no title)"
				}
				fmt.Fprintf(&buf, "  - %s %s\n", id, title)
			}
		}
	}

	return cli.WriteOutput(cmd, []byte(buf.String()))
}
