package studio

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/studio/components"
)

// Embedded default web assets for standalone or air-gapped execution.
// During development, if these files exist on disk, they are read fresh
// on each request without requiring a recompile.

//go:embed assets/style.css
var defaultEmbeddedCSS string

//go:embed assets/app.js
var defaultEmbeddedJS string

//go:embed assets/dashboard.html
var defaultEmbeddedBodyHTML string

// DashboardBuilder coordinates the dynamic generation and live asset loading
// of the Web Studio dashboard.
type DashboardBuilder struct {
	projectRoot     string
	customAssetsDir string
	document        *components.DocumentComponent
	header          *components.HeaderComponent
	dagView         *components.DAGViewComponent
	ganttView       *components.GanttViewComponent
	sidebar         *components.SidebarComponent
}

// NewDashboardBuilder constructs a new DashboardBuilder rooted at projectRoot.
func NewDashboardBuilder(projectRoot string) *DashboardBuilder {
	header := components.NewDefaultHeader()
	dagView := components.NewDefaultDAGView()
	ganttView := components.NewDefaultGanttView()
	sidebar := components.NewDefaultSidebar()

	doc := components.NewDefaultDocument(header.Title)
	doc.Header = header
	doc.Views = []components.Component{dagView, ganttView}
	doc.Sidebar = sidebar

	return &DashboardBuilder{
		projectRoot: projectRoot,
		document:    doc,
		header:      header,
		dagView:     dagView,
		ganttView:   ganttView,
		sidebar:     sidebar,
	}
}

// WithCustomAssetsDir overrides the disk asset lookup directory.
func (b *DashboardBuilder) WithCustomAssetsDir(dir string) *DashboardBuilder {
	b.customAssetsDir = dir
	return b
}

// Document returns the underlying DocumentComponent for programmatic customization.
func (b *DashboardBuilder) Document() *components.DocumentComponent {
	return b.document
}

// Header returns the HeaderComponent for programmatic customization.
func (b *DashboardBuilder) Header() *components.HeaderComponent {
	return b.header
}

// DAGView returns the DAGViewComponent for programmatic customization.
func (b *DashboardBuilder) DAGView() *components.DAGViewComponent {
	return b.dagView
}

// GanttView returns the GanttViewComponent for programmatic customization.
func (b *DashboardBuilder) GanttView() *components.GanttViewComponent {
	return b.ganttView
}

// Sidebar returns the SidebarComponent for programmatic customization.
func (b *DashboardBuilder) Sidebar() *components.SidebarComponent {
	return b.sidebar
}

// RenderHTML dynamically generates the complete HTML page, reading from disk if available
// or generating from utility components and embedded assets.
func (b *DashboardBuilder) RenderHTML() string {
	css := b.resolveCSS()
	js := b.resolveJS()

	// If a custom dashboard body HTML template exists on disk, assemble it with live CSS/JS.
	if bodyHTML := b.resolveBodyHTML(); bodyHTML != "" {
		return b.renderWithCustomBody(css, js, bodyHTML)
	}

	// Otherwise, generate dynamically from the component hierarchy.
	b.document.Title = b.header.Title
	b.document.StyleSheet = css
	b.document.Scripts = js
	b.document.Header = b.header
	b.document.Views = []components.Component{b.dagView, b.ganttView}
	b.document.Sidebar = b.sidebar

	return b.document.RenderHTML()
}

func (b *DashboardBuilder) resolveCSS() string {
	for _, dir := range b.candidateAssetDirs() {
		cssPath := filepath.Join(dir, "style.css")
		if data, err := os.ReadFile(cssPath); err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return defaultEmbeddedCSS
}

func (b *DashboardBuilder) resolveJS() string {
	for _, dir := range b.candidateAssetDirs() {
		jsPath := filepath.Join(dir, "app.js")
		if data, err := os.ReadFile(jsPath); err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return defaultEmbeddedJS
}

func (b *DashboardBuilder) resolveBodyHTML() string {
	for _, dir := range b.candidateAssetDirs() {
		bodyPath := filepath.Join(dir, "dashboard.html")
		if data, err := os.ReadFile(bodyPath); err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return ""
}

func (b *DashboardBuilder) candidateAssetDirs() []string {
	var dirs []string
	if b.customAssetsDir != "" {
		dirs = append(dirs, b.customAssetsDir)
	}
	if envDir := os.Getenv("ZQK_STUDIO_ASSETS_DIR"); envDir != "" {
		dirs = append(dirs, envDir)
	}
	if b.projectRoot != "" {
		dirs = append(dirs,
			filepath.Join(b.projectRoot, "pkg", "studio", "assets"),
			filepath.Join(b.projectRoot, "web", "studio"),
		)
	}
	return dirs
}

func (b *DashboardBuilder) renderWithCustomBody(css, js, bodyHTML string) string {
	var sb strings.Builder
	components.WriteHTMLHead(&sb, b.header.Title, css)
	sb.WriteString("</head>\n<body>\n")
	sb.WriteString(bodyHTML)
	if !strings.HasSuffix(bodyHTML, "\n") {
		sb.WriteString("\n")
	}

	if js != "" {
		sb.WriteString("  <script>\n")
		sb.WriteString(js)
		if !strings.HasSuffix(js, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("  </script>\n")
	}

	sb.WriteString("</body>\n</html>\n")
	return sb.String()
}
