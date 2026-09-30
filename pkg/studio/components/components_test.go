package components_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/studio/components"
)

func TestHeaderComponent_RenderHTML(t *testing.T) {
	header := components.NewDefaultHeader()
	html := header.RenderHTML()

	assert.Contains(t, html, "<header>")
	assert.Contains(t, html, "ZQK Knowledge Kernel Visual Studio")
	assert.Contains(t, html, "id=\"tab-nav-dag\"")
	assert.Contains(t, html, "id=\"tab-nav-gantt\"")
	assert.Contains(t, html, "id=\"workstream-filter\"")
	assert.Contains(t, html, "id=\"density-filter\"")
	assert.Contains(t, html, "id=\"global-search\"")
	assert.Contains(t, html, "id=\"live-indicator\"")
	assert.Contains(t, html, "</header>")

	// Verify dynamic custom options
	header.Title = "Custom Studio Title"
	header.ActiveView = "gantt"
	header.WorkstreamOptions = []components.SelectOption{
		{Value: "ws-1", Label: "Workstream 1", Selected: true},
	}
	customHTML := header.RenderHTML()
	assert.Contains(t, customHTML, "Custom Studio Title")
	assert.Contains(t, customHTML, "view-tab active\" id=\"tab-nav-gantt\"")
	assert.Contains(t, customHTML, "value=\"ws-1\" selected>Workstream 1</option>")
}

func TestDAGViewComponent_RenderHTML(t *testing.T) {
	dag := components.NewDefaultDAGView()
	html := dag.RenderHTML()

	assert.Contains(t, html, "id=\"view-dag\"")
	assert.Contains(t, html, "Interactive Knowledge Graph DAG")
	assert.Contains(t, html, "id=\"dag-svg\"")
	assert.Contains(t, html, "id=\"viewport\"")
	assert.Contains(t, html, "id=\"edges-layer\"")
	assert.Contains(t, html, "id=\"nodes-layer\"")
	assert.Contains(t, html, "id=\"kind-filters\"")
	assert.Contains(t, html, "data-kind=\"workstream\"")
	assert.Contains(t, html, "data-kind=\"milestone\"")
}

func TestGanttViewComponent_RenderHTML(t *testing.T) {
	gantt := components.NewDefaultGanttView()
	html := gantt.RenderHTML()

	assert.Contains(t, html, "id=\"view-gantt\"")
	assert.Contains(t, html, "gantt-toolbar")
	assert.Contains(t, html, "gantt-legend-bar")
	assert.Contains(t, html, "Milestone")
	assert.Contains(t, html, "Today Line")
	assert.Contains(t, html, "id=\"gantt-grouping\"")
	assert.Contains(t, html, "id=\"gantt-timeline-ticks\"")
	assert.Contains(t, html, "id=\"gantt-body\"")
}

func TestSidebarComponent_RenderHTML(t *testing.T) {
	sidebar := components.NewDefaultSidebar()
	html := sidebar.RenderHTML()

	assert.Contains(t, html, "class=\"sidebar-panel\"")
	assert.Contains(t, html, "id=\"tab-btn-inspector\"")
	assert.Contains(t, html, "id=\"tab-btn-objects\"")
	assert.Contains(t, html, "id=\"tab-btn-inbox\"")
	assert.Contains(t, html, "id=\"inspector-content\"")
	assert.Contains(t, html, "No Object Selected")
	assert.Contains(t, html, "id=\"objects-list-container\"")
	assert.Contains(t, html, "id=\"inbox-list-container\"")
}

func TestDocumentComponent_RenderHTML(t *testing.T) {
	doc := components.NewDefaultDocument("Test Studio Document")
	doc.StyleSheet = "body { margin: 0; }"
	doc.Header = components.NewDefaultHeader()
	doc.Views = []components.Component{
		components.NewDefaultDAGView(),
		components.NewDefaultGanttView(),
	}
	doc.Sidebar = components.NewDefaultSidebar()
	doc.Scripts = "console.log('ready');"

	html := doc.RenderHTML()
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Contains(t, html, "<title>Test Studio Document</title>")
	assert.Contains(t, html, "<style>\nbody { margin: 0; }\n  </style>")
	assert.Contains(t, html, "<header>")
	assert.Contains(t, html, "id=\"view-dag\"")
	assert.Contains(t, html, "id=\"view-gantt\"")
	assert.Contains(t, html, "class=\"sidebar-panel\"")
	assert.Contains(t, html, "<script>\nconsole.log('ready');\n  </script>")
	assert.Contains(t, html, "</html>")
}
