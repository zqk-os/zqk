package components

// Component represents an abstraction that can dynamically render HTML markup.
type Component interface {
	RenderHTML() string
}

// SelectOption represents an option in a dropdown select element.
type SelectOption struct {
	Value    string
	Label    string
	Selected bool
}

// KindChip represents a filter chip in the DAG view.
type KindChip struct {
	Kind   string
	Label  string
	Active bool
}

// StatusFilter represents a pill filter in the Gantt toolbar.
type StatusFilter struct {
	ID     string
	Label  string
	Active bool
}

// LegendItem represents an entry in the Gantt legend bar.
type LegendItem struct {
	Icon      string
	IconColor string
	Class     string
	Label     string
	IsLine    bool
}

// SidebarTab represents a tab in the right sidebar.
type SidebarTab struct {
	ID       string
	Label    string
	CountID  string
	Active   bool
}
