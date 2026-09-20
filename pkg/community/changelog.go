package community

import (
	"fmt"
	"strings"
	"time"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// ReleaseNoteEntry represents a categorized changelog item.
type ReleaseNoteEntry struct {
	Type    string
	Scope   string
	Message string
	Commit  string
}

// GenerateChangelog synthesizes a markdown changelog from a slice of commits.
func GenerateChangelog(version string, date time.Time, entries []ReleaseNoteEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## [%s] - %s\n\n", version, zqktime.FormatLayoutUTC(date, zqktime.LayoutDate)))

	features := []ReleaseNoteEntry{}
	fixes := []ReleaseNoteEntry{}
	chores := []ReleaseNoteEntry{}

	for _, e := range entries {
		switch strings.ToLower(e.Type) {
		case "feat":
			features = append(features, e)
		case "fix":
			fixes = append(fixes, e)
		default:
			chores = append(chores, e)
		}
	}

	if len(features) > 0 {
		sb.WriteString("### Features\n\n")
		for _, f := range features {
			if f.Scope != "" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s (%s)\n", f.Scope, f.Message, f.Commit))
			} else {
				sb.WriteString(fmt.Sprintf("- %s (%s)\n", f.Message, f.Commit))
			}
		}
		sb.WriteString("\n")
	}

	if len(fixes) > 0 {
		sb.WriteString("### Bug Fixes\n\n")
		for _, f := range fixes {
			if f.Scope != "" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s (%s)\n", f.Scope, f.Message, f.Commit))
			} else {
				sb.WriteString(fmt.Sprintf("- %s (%s)\n", f.Message, f.Commit))
			}
		}
		sb.WriteString("\n")
	}

	if len(chores) > 0 {
		sb.WriteString("### Maintenance & Chores\n\n")
		for _, c := range chores {
			if c.Scope != "" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s (%s)\n", c.Scope, c.Message, c.Commit))
			} else {
				sb.WriteString(fmt.Sprintf("- %s (%s)\n", c.Message, c.Commit))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
