package components

import (
	"fmt"
	"strings"
)

// DAGViewComponent dynamically renders the Interactive Knowledge Graph DAG panel.
type DAGViewComponent struct {
	Active           bool
	PanelTitle       string
	ShowZoomControls bool
	FocusBannerText  string
	KindChips        []KindChip
}

// NewDefaultDAGView constructs a standard DAGViewComponent.
func NewDefaultDAGView() *DAGViewComponent {
	return &DAGViewComponent{
		Active:           true,
		PanelTitle:       "Interactive Knowledge Graph DAG",
		ShowZoomControls: true,
		FocusBannerText:  "🎯 Focused Subgraph: ",
		KindChips: []KindChip{
			{Kind: "all", Label: "All", Active: true},
			{Kind: "workstream", Label: "Workstreams", Active: false},
			{Kind: "goal", Label: "Goals", Active: false},
			{Kind: "priority_plan", Label: "Plans", Active: false},
			{Kind: "milestone", Label: "Milestones", Active: false},
			{Kind: "backlog_item", Label: "BLIs", Active: false},
		},
	}
}

// RenderHTML generates the HTML string for the DAG view.
func (d *DAGViewComponent) RenderHTML() string {
	var sb strings.Builder

	activeClass := ""
	if d.Active {
		activeClass = " active"
	}

	fmt.Fprintf(&sb, "    <!-- View 1: Interactive DAG Graph -->\n")
	fmt.Fprintf(&sb, "    <div class=\"content-view%s\" id=\"view-dag\">\n", activeClass)
	sb.WriteString("      <div class=\"panel-graph\" id=\"graph-panel\">\n")
	fmt.Fprintf(&sb, "        <div class=\"panel-header\" style=\"display:none;\" id=\"dag-panel-title\">%s</div>\n\n", d.PanelTitle)

	if d.ShowZoomControls {
		sb.WriteString("        <!-- Zoom Controls -->\n")
		sb.WriteString("        <div class=\"graph-toolbar\">\n")
		sb.WriteString("          <button class=\"btn\" onclick=\"zoomIn()\" title=\"Zoom In\">+</button>\n")
		sb.WriteString("          <button class=\"btn\" onclick=\"zoomOut()\" title=\"Zoom Out\">-</button>\n")
		sb.WriteString("          <button class=\"btn\" onclick=\"resetZoom()\" title=\"Reset Zoom\">⟲</button>\n")
		sb.WriteString("          <button class=\"btn\" onclick=\"fitGraph()\" title=\"Fit to Screen\">⛶</button>\n")
		sb.WriteString("        </div>\n\n")
	}

	sb.WriteString("        <!-- Focus Subgraph Banner -->\n")
	sb.WriteString("        <div id=\"focus-banner\" class=\"focus-banner\" style=\"display:none;\">\n")
	fmt.Fprintf(&sb, "          <span id=\"focus-banner-text\">%s</span>\n", d.FocusBannerText)
	sb.WriteString("          <button class=\"btn btn-sm btn-accent\" onclick=\"clearFocus()\">✕ Show All</button>\n")
	sb.WriteString("        </div>\n\n")

	sb.WriteString("        <!-- Kind Filter Chips -->\n")
	sb.WriteString("        <div class=\"filter-bar\" id=\"kind-filters\">\n")
	for _, chip := range d.KindChips {
		chipActive := ""
		if chip.Active {
			chipActive = " active"
		}
		fmt.Fprintf(&sb, "          <span class=\"filter-chip%s\" data-kind=\"%s\" onclick=\"filterKind('%s')\">%s</span>\n",
			chipActive, chip.Kind, chip.Kind, chip.Label)
	}
	sb.WriteString("        </div>\n\n")

	sb.WriteString("        <svg id=\"dag-svg\" class=\"graph-canvas\">\n")
	sb.WriteString("          <defs>\n")
	sb.WriteString("            <marker id=\"arrow\" viewBox=\"0 0 10 10\" refX=\"10\" refY=\"5\" markerWidth=\"6\" markerHeight=\"6\" orient=\"auto-start-reverse\">\n")
	sb.WriteString("              <path d=\"M 0 1 L 10 5 L 0 9 z\" fill=\"#484f58\" />\n")
	sb.WriteString("            </marker>\n")
	sb.WriteString("            <marker id=\"arrow-highlight\" viewBox=\"0 0 10 10\" refX=\"10\" refY=\"5\" markerWidth=\"6\" markerHeight=\"6\" orient=\"auto-start-reverse\">\n")
	sb.WriteString("              <path d=\"M 0 1 L 10 5 L 0 9 z\" fill=\"#58a6ff\" />\n")
	sb.WriteString("            </marker>\n")
	sb.WriteString("          </defs>\n")
	sb.WriteString("          <g id=\"viewport\">\n")
	sb.WriteString("            <g id=\"edges-layer\"></g>\n")
	sb.WriteString("            <g id=\"nodes-layer\"></g>\n")
	sb.WriteString("          </g>\n")
	sb.WriteString("        </svg>\n")
	sb.WriteString("      </div>\n")
	sb.WriteString("    </div>\n")

	return sb.String()
}
