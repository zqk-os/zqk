package components

import (
	"fmt"
	"strings"
)

// SidebarComponent dynamically renders the shared multi-tab sidebar panel.
type SidebarComponent struct {
	Tabs               []SidebarTab
	ActiveTab          string
	EmptyStateTitle    string
	EmptyStateSubtitle string
}

// NewDefaultSidebar constructs a standard SidebarComponent.
func NewDefaultSidebar() *SidebarComponent {
	return &SidebarComponent{
		Tabs: []SidebarTab{
			{ID: "inspector", Label: "Inspector", CountID: "", Active: true},
			{ID: "objects", Label: "Kernel Objects", CountID: "objects-count", Active: false},
			{ID: "inbox", Label: "Inbox", CountID: "inbox-count", Active: false},
		},
		ActiveTab:          "inspector",
		EmptyStateTitle:    "No Object Selected",
		EmptyStateSubtitle: "Click any item in the DAG or Gantt timeline to inspect its causal dependencies.",
	}
}

// RenderHTML generates the HTML string for the sidebar panel.
func (s *SidebarComponent) RenderHTML() string {
	var sb strings.Builder

	sb.WriteString("    <!-- Right: Multi-Tab Sidebar (Shared) -->\n")
	sb.WriteString("    <div class=\"sidebar-panel\">\n")
	sb.WriteString("      <div class=\"tab-bar\">\n")
	for _, tab := range s.Tabs {
		active := ""
		if (s.ActiveTab == tab.ID) || (s.ActiveTab == "" && tab.Active) {
			active = " active"
		}
		if tab.CountID != "" {
			fmt.Fprintf(&sb, "        <div class=\"tab-btn%s\" id=\"tab-btn-%s\" onclick=\"switchTab('%s')\">%s (<span id=\"%s\">0</span>)</div>\n",
				active, tab.ID, tab.ID, tab.Label, tab.CountID)
		} else {
			fmt.Fprintf(&sb, "        <div class=\"tab-btn%s\" id=\"tab-btn-%s\" onclick=\"switchTab('%s')\">%s</div>\n",
				active, tab.ID, tab.ID, tab.Label)
		}
	}
	sb.WriteString("      </div>\n\n")

	// Tab 1: Inspector
	inspActive := ""
	if s.ActiveTab == "inspector" || s.ActiveTab == "" {
		inspActive = " active"
	}
	fmt.Fprintf(&sb, "      <!-- Tab 1: Selected Object Inspector -->\n")
	fmt.Fprintf(&sb, "      <div class=\"tab-content%s\" id=\"tab-inspector\">\n", inspActive)
	sb.WriteString("        <div id=\"inspector-content\">\n")
	sb.WriteString("          <div class=\"empty-state\">\n")
	sb.WriteString("            <div class=\"empty-state-icon\">🎯</div>\n")
	fmt.Fprintf(&sb, "            <div class=\"empty-state-title\">%s</div>\n", s.EmptyStateTitle)
	fmt.Fprintf(&sb, "            <div style=\"font-size: 12px; margin-top: 4px;\">%s</div>\n", s.EmptyStateSubtitle)
	sb.WriteString("          </div>\n")
	sb.WriteString("        </div>\n")
	sb.WriteString("      </div>\n\n")

	// Tab 2: Objects List
	objActive := ""
	if s.ActiveTab == "objects" {
		objActive = " active"
	}
	fmt.Fprintf(&sb, "      <!-- Tab 2: Kernel Objects List -->\n")
	fmt.Fprintf(&sb, "      <div class=\"tab-content%s\" id=\"tab-objects\">\n", objActive)
	sb.WriteString("        <div id=\"objects-list-container\">\n")
	sb.WriteString("          <div style=\"padding: 16px; color: var(--text-muted);\">Loading objects...</div>\n")
	sb.WriteString("        </div>\n")
	sb.WriteString("      </div>\n\n")

	// Tab 3: Operator Inbox
	inboxActive := ""
	if s.ActiveTab == "inbox" {
		inboxActive = " active"
	}
	fmt.Fprintf(&sb, "      <!-- Tab 3: Swarm & Operator Inbox -->\n")
	fmt.Fprintf(&sb, "      <div class=\"tab-content%s\" id=\"tab-inbox\">\n", inboxActive)
	sb.WriteString("        <div style=\"padding: 10px 14px; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border);\">\n")
	sb.WriteString("          <span style=\"font-weight: 600; font-size: 13px;\">Agent & Operator Inbox</span>\n")
	sb.WriteString("          <button class=\"btn btn-sm\" onclick=\"fetchInbox()\">↻ Refresh</button>\n")
	sb.WriteString("        </div>\n")
	sb.WriteString("        <div id=\"inbox-list-container\" style=\"overflow-y: auto; max-height: calc(100vh - 180px); padding: 8px;\">\n")
	sb.WriteString("          <div style=\"padding: 16px; color: var(--text-muted);\">Loading inbox...</div>\n")
	sb.WriteString("        </div>\n")
	sb.WriteString("      </div>\n")

	sb.WriteString("    </div>\n")

	return sb.String()
}
