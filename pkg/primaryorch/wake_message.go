package primaryorch

import (
	"fmt"
	"strings"
)

// BacklogBrief is a compact open backlog row for attentiveness wakes.
type BacklogBrief struct {
	ID    string
	Title string
	Tier  string
}

// AppendAttentivenessContext appends PRI id and up to three open BLI lines to a wake message.
// Empty planID / briefs are omitted (callers may still send a useful base message).
func AppendAttentivenessContext(base, planID string, top []BacklogBrief) string {
	base = strings.TrimSpace(base)
	var b strings.Builder
	if base != "" {
		b.WriteString(base)
	}
	if planID = strings.TrimSpace(planID); planID != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("PRI=")
		b.WriteString(planID)
	}
	n := 0
	for _, row := range top {
		id := strings.TrimSpace(row.ID)
		if id == "" {
			continue
		}
		n++
		if n > 3 {
			break
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		title := strings.TrimSpace(row.Title)
		if title == "" {
			title = "(untitled)"
		}
		tier := strings.TrimSpace(row.Tier)
		if tier == "" {
			tier = "?"
		}
		fmt.Fprintf(&b, "BLI[%d] %s %s — %s", n, tier, id, title)
	}
	if b.Len() == 0 {
		return defaultWakeMessage(WakeRequest{})
	}
	return b.String()
}

// ComposeWakeMessage builds the delivered wake text from WakeRequest fields.
func ComposeWakeMessage(req WakeRequest) string {
	base := strings.TrimSpace(req.Message)
	if base == "" {
		base = defaultWakeMessage(req)
	}
	return AppendAttentivenessContext(base, req.PlanID, req.TopBLIs)
}
