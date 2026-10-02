package components

import (
	"fmt"
	"strings"
)

// GanttViewComponent dynamically renders the Timeline & Gantt View panel.
type GanttViewComponent struct {
	Active          bool
	StatusFilters   []StatusFilter
	GroupingOptions []SelectOption
	LegendItems     []LegendItem
}

// NewDefaultGanttView constructs a standard GanttViewComponent.
func NewDefaultGanttView() *GanttViewComponent {
	return &GanttViewComponent{
		Active: false,
		StatusFilters: []StatusFilter{
			{ID: "gantt-pill-all", Label: "All", Active: true},
			{ID: "gantt-pill-active", Label: "In Progress", Active: false},
			{ID: "gantt-pill-planned", Label: "Planned", Active: false},
			{ID: "gantt-pill-done", Label: "Completed", Active: false},
		},
		GroupingOptions: []SelectOption{
			{Value: "workstream", Label: "By Workstream", Selected: true},
			{Value: "plan", Label: "By Priority Plan", Selected: false},
			{Value: "flat", Label: "Flat (Chronological)", Selected: false},
		},
		LegendItems: []LegendItem{
			{Icon: "◆", IconColor: "#f0883e", Class: "gantt-bar-milestone", Label: "Milestone", IsLine: false},
			{Icon: "🌐", IconColor: "#58a6ff", Class: "gantt-bar-workstream", Label: "Workstream", IsLine: false},
			{Icon: "▶", IconColor: "#58a6ff", Class: "gantt-bar-inprogress", Label: "In Progress", IsLine: false},
			{Icon: "⏳", IconColor: "#8b949e", Class: "gantt-bar-planned", Label: "Planned", IsLine: false},
			{Icon: "✓", IconColor: "#3fb950", Class: "gantt-bar-complete", Label: "Completed", IsLine: false},
			{Icon: "⚙", IconColor: "#bc8cff", Class: "gantt-bar-testing", Label: "Testing", IsLine: false},
			{Icon: "", IconColor: "#f85149", Class: "gantt-legend-today-line", Label: "Today Line", IsLine: true},
		},
	}
}

// RenderHTML generates the HTML string for the Gantt view.
func (g *GanttViewComponent) RenderHTML() string {
	var sb strings.Builder

	activeClass := ""
	if g.Active {
		activeClass = " active"
	}

	fmt.Fprintf(&sb, "    <!-- View 2: Timeline & Gantt View -->\n")
	fmt.Fprintf(&sb, "    <div class=\"content-view%s\" id=\"view-gantt\">\n", activeClass)
	sb.WriteString("      <div class=\"gantt-container\">\n")
	sb.WriteString("        <!-- Gantt Sub-Toolbar -->\n")
	sb.WriteString("        <div class=\"gantt-toolbar\">\n")
	sb.WriteString("          <div class=\"gantt-filter-group\">\n")
	sb.WriteString("            <span class=\"gantt-toolbar-label\">Status:</span>\n")
	for _, f := range g.StatusFilters {
		pillActive := ""
		if f.Active {
			pillActive = " active"
		}
		statusKey := strings.TrimPrefix(f.ID, "gantt-pill-")
		fmt.Fprintf(&sb, "            <button class=\"gantt-filter-pill%s\" id=\"%s\" onclick=\"setGanttStatusFilter('%s')\">%s</button>\n",
			pillActive, f.ID, statusKey, f.Label)
	}
	sb.WriteString("          </div>\n")
	sb.WriteString("          <div class=\"gantt-filter-group\">\n")
	sb.WriteString("            <span class=\"gantt-toolbar-label\">Timescale:</span>\n")
	sb.WriteString("            <button class=\"gantt-filter-pill active\" id=\"gantt-zoom-fit\" onclick=\"setGanttZoom('fit')\" title=\"Auto-Fit Active Task Horizon\">Auto-Fit</button>\n")
	sb.WriteString("            <button class=\"gantt-filter-pill\" id=\"gantt-zoom-1w\" onclick=\"setGanttZoom('1w')\" title=\"1-Week High Resolution\">1 Week</button>\n")
	sb.WriteString("            <button class=\"gantt-filter-pill\" id=\"gantt-zoom-2w\" onclick=\"setGanttZoom('2w')\" title=\"2-Week Sprint Zoom\">2 Weeks</button>\n")
	sb.WriteString("            <button class=\"gantt-filter-pill\" id=\"gantt-zoom-1m\" onclick=\"setGanttZoom('1m')\" title=\"1-Month Horizon\">1 Month</button>\n")
	sb.WriteString("            <button class=\"gantt-filter-pill\" id=\"gantt-zoom-all\" onclick=\"setGanttZoom('all')\" title=\"Full Project Span\">All Roadmap</button>\n")
	sb.WriteString("          </div>\n")
	sb.WriteString("          <div class=\"gantt-filter-group\">\n")
	sb.WriteString("            <span class=\"gantt-toolbar-label\">Grouping:</span>\n")
	sb.WriteString("            <select id=\"gantt-grouping\" class=\"select-input\" onchange=\"renderGantt()\">\n")
	for _, opt := range g.GroupingOptions {
		sel := ""
		if opt.Selected {
			sel = " selected"
		}
		fmt.Fprintf(&sb, "              <option value=\"%s\"%s>%s</option>\n", opt.Value, sel, opt.Label)
	}
	sb.WriteString("            </select>\n")
	sb.WriteString("          </div>\n")
	sb.WriteString("          <div style=\"flex: 1;\"></div>\n")
	sb.WriteString("          <span id=\"gantt-task-count\" style=\"font-size: 11px; color: var(--text-muted); font-weight: 500;\">0 items</span>\n")
	sb.WriteString("        </div>\n\n")

	sb.WriteString("        <!-- Gantt Legend Bar -->\n")
	sb.WriteString("        <div class=\"gantt-legend-bar\">\n")
	sb.WriteString("          <span class=\"gantt-legend-title\">Legend:</span>\n")
	sb.WriteString("          <div class=\"gantt-legend-items\">\n")
	for _, item := range g.LegendItems {
		sb.WriteString("            <div class=\"gantt-legend-item\">\n")
		if item.IsLine {
			fmt.Fprintf(&sb, "              <span class=\"%s\"></span>\n", item.Class)
			fmt.Fprintf(&sb, "              <span class=\"gantt-legend-text\" style=\"color: %s; font-weight: 600;\">%s</span>\n", item.IconColor, item.Label)
		} else {
			fmt.Fprintf(&sb, "              <span class=\"gantt-legend-icon\" style=\"color: %s;\">%s</span>\n", item.IconColor, item.Icon)
			fmt.Fprintf(&sb, "              <span class=\"gantt-legend-swatch %s\"></span>\n", item.Class)
			fmt.Fprintf(&sb, "              <span class=\"gantt-legend-text\">%s</span>\n", item.Label)
		}
		sb.WriteString("            </div>\n")
	}
	sb.WriteString("          </div>\n")
	sb.WriteString("        </div>\n\n")

	sb.WriteString("        <div class=\"gantt-header-row\">\n")
	sb.WriteString("          <div class=\"gantt-header-title\">\n")
	sb.WriteString("            <span>Work Item Hierarchy</span>\n")
	sb.WriteString("          </div>\n")
	sb.WriteString("          <div class=\"gantt-timeline-ticks\" id=\"gantt-timeline-ticks\">\n")
	sb.WriteString("            <!-- Dynamic date ticks -->\n")
	sb.WriteString("          </div>\n")
	sb.WriteString("        </div>\n")
	sb.WriteString("        <div class=\"gantt-body\" id=\"gantt-body\">\n          <!-- Dynamic grouped rows -->\n        </div>\n      </div>\n    </div>\n")
	return sb.String()
}
