package components

import (
	"fmt"
	"strings"
)

// HeaderComponent dynamically renders the top visual studio header bar.
type HeaderComponent struct {
	BrandIcon         string
	Title             string
	ActiveView        string
	WorkstreamOptions []SelectOption
	DensityOptions    []SelectOption
	SearchPlaceholder string
	LiveStatusText    string
}

// NewDefaultHeader constructs a standard HeaderComponent.
func NewDefaultHeader() *HeaderComponent {
	return &HeaderComponent{
		BrandIcon:  "⚡",
		Title:      "ZQK Knowledge Kernel Visual Studio",
		ActiveView: "dag",
		WorkstreamOptions: []SelectOption{
			{Value: "all", Label: "🌐 All Workstreams", Selected: true},
		},
		DensityOptions: []SelectOption{
			{Value: "backbone", Label: "Backbone (Plans & Milestones)", Selected: false},
			{Value: "execution", Label: "Execution (+ Backlog Items)", Selected: true},
			{Value: "all", Label: "Full Mesh (All Objects)", Selected: false},
		},
		SearchPlaceholder: "Search (press /) ...",
		LiveStatusText:    "Connected",
	}
}

// RenderHTML generates the HTML string for the header bar.
func (h *HeaderComponent) RenderHTML() string {
	var sb strings.Builder

	dagActive := ""
	if h.ActiveView == "dag" || h.ActiveView == "" {
		dagActive = " active"
	}
	ganttActive := ""
	if h.ActiveView == "gantt" {
		ganttActive = " active"
	}

	sb.WriteString("  <header>\n")
	sb.WriteString("    <div class=\"brand\">\n")
	fmt.Fprintf(&sb, "      <span class=\"brand-icon\">%s</span>\n", h.BrandIcon)
	fmt.Fprintf(&sb, "      <h1>%s</h1>\n", h.Title)
	sb.WriteString("    </div>\n\n")

	sb.WriteString("    <!-- View Mode Switcher -->\n")
	sb.WriteString("    <div class=\"view-switcher\">\n")
	fmt.Fprintf(&sb, "      <div class=\"view-tab%s\" id=\"tab-nav-dag\" onclick=\"switchMainView('dag')\">☊ DAG Graph</div>\n", dagActive)
	fmt.Fprintf(&sb, "      <div class=\"view-tab%s\" id=\"tab-nav-gantt\" onclick=\"switchMainView('gantt')\">▤ Timeline & Gantt</div>\n", ganttActive)
	sb.WriteString("    </div>\n\n")

	sb.WriteString("    <div class=\"header-controls\">\n")
	sb.WriteString("      <!-- Workstream Filter Dropdown -->\n")
	sb.WriteString("      <select id=\"workstream-filter\" class=\"select-input\" onchange=\"onWorkstreamChange()\" title=\"Filter by Workstream\">\n")
	for _, opt := range h.WorkstreamOptions {
		sel := ""
		if opt.Selected {
			sel = " selected"
		}
		fmt.Fprintf(&sb, "        <option value=\"%s\"%s>%s</option>\n", opt.Value, sel, opt.Label)
	}
	sb.WriteString("      </select>\n\n")

	sb.WriteString("      <!-- Density Filter -->\n")
	sb.WriteString("      <select id=\"density-filter\" class=\"select-input\" onchange=\"onDensityChange()\" title=\"Control Graph Density\">\n")
	for _, opt := range h.DensityOptions {
		sel := ""
		if opt.Selected {
			sel = " selected"
		}
		fmt.Fprintf(&sb, "        <option value=\"%s\"%s>%s</option>\n", opt.Value, sel, opt.Label)
	}
	sb.WriteString("      </select>\n\n")

	sb.WriteString("      <div class=\"search-box\">\n")
	sb.WriteString("        <span class=\"search-icon\">🔍</span>\n")
	fmt.Fprintf(&sb, "        <input type=\"text\" id=\"global-search\" class=\"search-input\" placeholder=\"%s\">\n", h.SearchPlaceholder)
	sb.WriteString("      </div>\n\n")

	sb.WriteString("      <div class=\"status-pill\" id=\"live-indicator\">\n")
	sb.WriteString("        <span class=\"pulse-dot\"></span>\n")
	fmt.Fprintf(&sb, "        <span>%s</span>\n", h.LiveStatusText)
	sb.WriteString("      </div>\n\n")

	sb.WriteString("      <button class=\"btn\" id=\"btn-refresh\" onclick=\"loadDAG()\">⟳ Refresh</button>\n")
	sb.WriteString("    </div>\n")
	sb.WriteString("  </header>\n")

	return sb.String()
}
