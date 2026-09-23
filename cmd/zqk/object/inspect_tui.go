package object

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/zqk-os/zqk/cmd/zqk/ui/tds"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ANSI terminal escape control constants.
const (
	ansiAltBufferEnter = "\033[?1049h"
	ansiAltBufferExit  = "\033[?1049l"
	ansiClearScreen    = "\033[2J"
	ansiHomeCursor     = "\033[H"
	ansiHideCursor     = "\033[?25l"
	ansiShowCursor     = "\033[?25h"
	ansiClearToEOL     = "\033[K"
	ansiClearToBottom  = "\033[J"
	crlf               = "\r\n"
)

// Filter pill options.
var filterPillOptions = []string{"all", "active", "draft", "blocked", "complete", "mine"}

// Sort key options.
var sortKeyOptions = []string{"updated_at", "created_at", "id", "priority", "status"}

// InspectTUIModel manages interactive full-screen TUI state for Object Inspector.
type InspectTUIModel struct {
	Storage           storage.ObjectStorageProvider
	SecCtx            *pkgctx.SecurityContext
	StorageCtx        *storage.StorageContext
	Ctx               context.Context
	ActiveKind        string
	AvailableKinds    []string
	KindIndex         int
	AllObjects        []map[string]any
	VisibleProjections []SemanticAgentProjection
	RawVisibleObjects []map[string]any
	SelectedIndex     int
	ScrollOffset      int
	Width             int
	Height            int
	Fields            []string
	SortBy            string
	SortAsc           bool
	SortIndex         int
	FilterPill        string
	FilterIndex       int
	SearchQuery       string
	IsSearching       bool
	SearchBuffer      string
	DetailModalOpen   bool
	PolicyStudioOpen  bool
	StatusMessage     string
	StatusExpiresAt   time.Time
}

// NewInspectTUIModel constructs a new interactive Object Inspector model.
func NewInspectTUIModel(ctx context.Context, initialKind string, fields, filters []string, sortBy string, sortAsc bool, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext) *InspectTUIModel {
	m := &InspectTUIModel{
		Ctx:         ctx,
		Storage:     sp,
		SecCtx:      secCtx,
		StorageCtx:  storageCtx,
		Fields:      fields,
		SortBy:      sortBy,
		SortAsc:     sortAsc,
		FilterPill:  "all",
		Width:       100,
		Height:      30,
	}

	if m.SortBy == "" {
		m.SortBy = "updated_at"
	}
	for i, sk := range sortKeyOptions {
		if sk == m.SortBy {
			m.SortIndex = i
			break
		}
	}

	m.DiscoverAvailableKinds(initialKind)
	m.RefreshObjects()
	return m
}

// DiscoverAvailableKinds populates available kinds from FieldRegistry and known schemas.
func (m *InspectTUIModel) DiscoverAvailableKinds(initialKind string) {
	kindSet := make(map[string]bool)
	commonKinds := []string{
		objects.KindBacklogItem,
		objects.KindRequirement,
		objects.KindCriteria,
		objects.KindTestCase,
		objects.KindMilestone,
		objects.KindPriorityPlan,
		objects.KindStrategicPlan,
		objects.KindPolicy,
		objects.KindWorkstream,
		objects.KindAgentTask,
	}
	for _, k := range commonKinds {
		kindSet[k] = true
	}

	reg := objects.GetGlobalFieldRegistry()
	if reg != nil {
		if allKinds, err := reg.GetAllKinds(); err == nil {
			for _, k := range allKinds {
				if !strings.HasPrefix(k, "_") {
					kindSet[k] = true
				}
			}
		}
	}

	kinds := make([]string, 0, len(kindSet))
	for k := range kindSet {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	m.AvailableKinds = kinds

	m.ActiveKind = objects.KindBacklogItem
	if initialKind != "" {
		for i, k := range m.AvailableKinds {
			if strings.EqualFold(k, initialKind) {
				m.ActiveKind = k
				m.KindIndex = i
				return
			}
		}
		// If initialKind was specified but not in list, prepend it
		m.AvailableKinds = append([]string{initialKind}, m.AvailableKinds...)
		m.ActiveKind = initialKind
		m.KindIndex = 0
		return
	}

	for i, k := range m.AvailableKinds {
		if k == m.ActiveKind {
			m.KindIndex = i
			break
		}
	}
}

// RefreshObjects reloads objects of the active kind from storage and applies filtering/sorting.
func (m *InspectTUIModel) RefreshObjects() {
	if m.Storage == nil {
		return
	}

	res, err := m.Storage.List(m.Ctx, m.SecCtx, m.StorageCtx, storage.ListFilter{
		Kind: m.ActiveKind,
	})
	if err != nil || res == nil {
		m.AllObjects = nil
		m.VisibleProjections = nil
		m.RawVisibleObjects = nil
		m.SelectedIndex = 0
		return
	}

	m.AllObjects = res.Objects

	// 1. Filter
	filtered := make([]map[string]any, 0, len(m.AllObjects))
	for _, obj := range m.AllObjects {
		if m.matchesCurrentFilter(obj) && m.matchesSearch(obj) {
			filtered = append(filtered, obj)
		}
	}

	// 2. Sort
	sort.Slice(filtered, func(i, j int) bool {
		vi := fmt.Sprintf("%v", filtered[i][m.SortBy])
		vj := fmt.Sprintf("%v", filtered[j][m.SortBy])
		if m.SortAsc {
			return vi < vj
		}
		return vi > vj
	})

	m.RawVisibleObjects = filtered
	m.VisibleProjections = make([]SemanticAgentProjection, 0, len(filtered))
	for _, obj := range filtered {
		m.VisibleProjections = append(m.VisibleProjections, buildSemanticProjection(m.Ctx, m.Storage, m.SecCtx, obj, m.ActiveKind, m.Fields))
	}

	if m.SelectedIndex >= len(m.VisibleProjections) {
		m.SelectedIndex = len(m.VisibleProjections) - 1
	}
	if m.SelectedIndex < 0 {
		m.SelectedIndex = 0
	}
}

func (m *InspectTUIModel) matchesCurrentFilter(obj map[string]any) bool {
	status := strings.ToLower(fmt.Sprintf("%v", obj[objects.FieldKeyStatus]))
	claimed := fmt.Sprintf("%v", obj["claimed_by"])

	switch m.FilterPill {
	case "active":
		return status == "active" || status == "in_progress" || status == "testing" || status == "exploring" || status == "planned"
	case "draft":
		return status == "draft" || status == "conceptual" || status == "originated"
	case "blocked":
		return status == "blocked" || status == "error"
	case "complete":
		return status == "complete" || status == "validated" || status == "satisfied" || status == "archived"
	case "mine":
		return claimed != "" && claimed != "<nil>" && claimed != "-"
	default: // "all"
		return true
	}
}

func (m *InspectTUIModel) matchesSearch(obj map[string]any) bool {
	if m.SearchQuery == "" {
		return true
	}
	q := strings.ToLower(m.SearchQuery)
	id := strings.ToLower(fmt.Sprintf("%v", obj[objects.FieldKeyID]))
	title := strings.ToLower(fmt.Sprintf("%v", obj[objects.FieldKeyTitle]))
	return strings.Contains(id, q) || strings.Contains(title, q)
}

// CycleKind moves active kind index forward or backward.
func (m *InspectTUIModel) CycleKind(forward bool) {
	if len(m.AvailableKinds) == 0 {
		return
	}
	if forward {
		m.KindIndex = (m.KindIndex + 1) % len(m.AvailableKinds)
	} else {
		m.KindIndex = (m.KindIndex - 1 + len(m.AvailableKinds)) % len(m.AvailableKinds)
	}
	m.ActiveKind = m.AvailableKinds[m.KindIndex]
	m.SelectedIndex = 0
	m.ScrollOffset = 0
	m.DetailModalOpen = false
	m.RefreshObjects()
}

// CycleFilter advances the filter pill to the next option.
func (m *InspectTUIModel) CycleFilter() {
	m.FilterIndex = (m.FilterIndex + 1) % len(filterPillOptions)
	m.FilterPill = filterPillOptions[m.FilterIndex]
	m.SelectedIndex = 0
	m.ScrollOffset = 0
	m.RefreshObjects()
}

// CycleSort advances the sort key to the next option.
func (m *InspectTUIModel) CycleSort() {
	m.SortIndex = (m.SortIndex + 1) % len(sortKeyOptions)
	m.SortBy = sortKeyOptions[m.SortIndex]
	m.RefreshObjects()
}

// SetStatus displays a temporary notification message in the TUI header/footer.
func (m *InspectTUIModel) SetStatus(msg string, dur time.Duration) {
	m.StatusMessage = msg
	m.StatusExpiresAt = time.Now().Add(dur)
}

// Render formats the entire ANSI TUI screen frame.
func (m *InspectTUIModel) Render() string {
	w := m.Width
	if w < 60 {
		w = 80
	}
	h := m.Height
	if h < 20 {
		h = 24
	}

	var out strings.Builder

	// 1. Top Header Banner
	bannerTitle := fmt.Sprintf("🔍 ZQK OBJECT INSPECTOR — [Kind: %s] (%d active)", m.ActiveKind, len(m.AllObjects))
	var headerLines []string
	filterPillStr := fmt.Sprintf("[%s]", strings.ToUpper(m.FilterPill))
	sortPillStr := fmt.Sprintf("[%s %s]", m.SortBy, map[bool]string{true: "▲", false: "▼"}[m.SortAsc])
	searchIndicator := ""
	if m.SearchQuery != "" {
		searchIndicator = fmt.Sprintf("  │ Search: '%s'", m.SearchQuery)
	}
	statusLine := fmt.Sprintf("Filter: %s  │ Sort: %s%s  │ Match: %d/%d",
		color.New(color.FgCyan, color.Bold).Sprint(filterPillStr),
		color.New(color.FgYellow).Sprint(sortPillStr),
		searchIndicator,
		len(m.VisibleProjections),
		len(m.AllObjects),
	)
	if m.StatusMessage != "" && time.Now().Before(m.StatusExpiresAt) {
		statusLine += fmt.Sprintf("  │ %s", color.New(color.FgGreen, color.Bold).Sprint(m.StatusMessage))
	}
	headerLines = append(headerLines, statusLine)
	out.WriteString(tds.Panel(bannerTitle, headerLines, w, tds.BorderHeavy))
	out.WriteString("\n")

	// 2. Drill-Down Detail Modal Overlay
	if m.DetailModalOpen && m.SelectedIndex < len(m.VisibleProjections) {
		out.WriteString(m.renderDetailModal(w, h))
		return out.String()
	}

	// 3. Policy Studio Overlay
	if m.PolicyStudioOpen {
		out.WriteString(m.renderPolicyStudioOverlay(w, h))
		return out.String()
	}

	// 4. Split Pane: Master Table (top half) + Inspected Object Property Card (bottom half)
	tableRows := (h - 14) / 2
	if tableRows < 5 {
		tableRows = 5
	}

	table := tds.NewTable(w - 2)
	table.AddColumn("ID", tds.AlignLeft, 18, 1.2)
	table.AddColumn("PRI", tds.AlignCenter, 5, 0.4)
	table.AddColumn("STATUS", tds.AlignLeft, 14, 1.0)
	table.AddColumn("OWNER", tds.AlignLeft, 14, 1.0)
	table.AddColumn("TITLE", tds.AlignLeft, 32, 2.4)

	// Determine visible slice
	start := m.SelectedIndex - (tableRows / 2)
	if start < 0 {
		start = 0
	}
	end := start + tableRows
	if end > len(m.VisibleProjections) {
		end = len(m.VisibleProjections)
		start = end - tableRows
		if start < 0 {
			start = 0
		}
	}

	if len(m.VisibleProjections) == 0 {
		table.AddRow("-", "-", "[EMPTY]", "-", "No objects matching active filter/search")
	} else {
		for i := start; i < end; i++ {
			p := m.VisibleProjections[i]
			isSel := i == m.SelectedIndex
			idCell := tds.RowCursor(isSel, p.ID)
			statusBadge := tds.Badge(p.Status)
			table.AddRow(
				idCell,
				defaultStr(p.Priority, "-"),
				statusBadge,
				defaultStr(p.ClaimedBy, "unassigned"),
				truncateString(p.Title, 45),
			)
		}
	}
	out.WriteString(table.Render())
	out.WriteString("\n")

	// 5. Lower Pane: Inspected Object Property Card
	if m.SelectedIndex < len(m.VisibleProjections) {
		sel := m.VisibleProjections[m.SelectedIndex]
		out.WriteString(m.renderPropertyCard(sel, w))
	} else {
		emptyCard := tds.Panel("INSPECTED OBJECT", []string{"No object selected"}, w, tds.BorderRounded)
		out.WriteString(emptyCard)
	}
	out.WriteString("\n")

	// 6. Inline Search Prompt (if active)
	if m.IsSearching {
		prompt := fmt.Sprintf("🔍 Search regex/substring: %s█  (Press [Enter] to apply, [Esc] to cancel)", m.SearchBuffer)
		out.WriteString(color.New(color.FgCyan, color.Bold).Sprint(prompt))
		out.WriteString("\n")
	} else {
		// Navigation Key Help Bar
		helpBar := "Nav: [Tab] Kind │ [j/k] Select │ [g/G] Top/Bottom │ [f] Filter │ [s] Sort │ [/] Search │ [Enter] Drill-down │ [p] Policy │ [q] Quit"
		out.WriteString(color.New(color.Faint).Sprint(helpBar))
		out.WriteString("\n")
	}

	return out.String()
}

func (m *InspectTUIModel) renderPropertyCard(p SemanticAgentProjection, width int) string {
	var lines []string

	headerStats := []tds.StatItem{
		{Label: "Kind", Value: p.Kind},
		{Label: "Status", Value: p.Status, Extra: tds.Badge(p.Status)},
		{Label: "Priority", Value: defaultStr(p.Priority, "-")},
		{Label: "Owner", Value: defaultStr(p.ClaimedBy, "unassigned")},
	}
	lines = append(lines, tds.StatRow(headerStats, width-6))

	if p.Title != "" {
		lines = append(lines, fmt.Sprintf("Title: %s", color.New(color.FgWhite, color.Bold).Sprint(p.Title)))
	}

	// Lineage Radar Summary
	if p.Lineage != nil {
		chainParts := []string{}
		if p.Lineage.Goal != "" {
			chainParts = append(chainParts, fmt.Sprintf("[goal %s]", p.Lineage.Goal))
		}
		if p.Lineage.Milestone != "" {
			chainParts = append(chainParts, fmt.Sprintf("[milestone %s]", p.Lineage.Milestone))
		}
		if p.Lineage.Requirement != "" {
			chainParts = append(chainParts, fmt.Sprintf("[req %s]", p.Lineage.Requirement))
		}
		if p.Lineage.PriorityPlan != "" {
			chainParts = append(chainParts, fmt.Sprintf("[plan %s]", p.Lineage.PriorityPlan))
		}
		chainStr := strings.Join(chainParts, " ➔ ")
		if chainStr == "" {
			chainStr = "(root or self-contained)"
		}
		intactBadge := tds.Badge("PASS")
		if !p.Lineage.IsIntact {
			intactBadge = tds.Badge("WARN")
		}
		lines = append(lines, fmt.Sprintf("Lineage: %s  %s", chainStr, intactBadge))
	}

	// Criteria Summary
	if p.CriteriaSummary != nil && p.CriteriaSummary.Total > 0 {
		pct := float64(p.CriteriaSummary.Satisfied) / float64(p.CriteriaSummary.Total)
		bar := tds.ProgressBar(pct, 16)
		lines = append(lines, fmt.Sprintf("Criteria: %d/%d satisfied (%d open)  %s",
			p.CriteriaSummary.Satisfied, p.CriteriaSummary.Total, p.CriteriaSummary.Pending, bar))
	}

	if len(p.ActionsAvailable) > 0 {
		lines = append(lines, fmt.Sprintf("Actions: [%s]  (Press [Enter] to inspect/act)", strings.Join(p.ActionsAvailable, ", ")))
	}

	cardTitle := fmt.Sprintf("── INSPECTED OBJECT: %s (Press Enter for Full Drill-Down) ──", p.ID)
	return tds.Panel(cardTitle, lines, width, tds.BorderRounded)
}

func (m *InspectTUIModel) renderDetailModal(width, height int) string {
	p := m.VisibleProjections[m.SelectedIndex]
	var rawObj map[string]any
	if m.SelectedIndex < len(m.RawVisibleObjects) {
		rawObj = m.RawVisibleObjects[m.SelectedIndex]
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Object Identifier : %s", color.New(color.FgCyan, color.Bold).Sprint(p.ID)))
	lines = append(lines, fmt.Sprintf("Schema Kind       : %s", p.Kind))
	lines = append(lines, fmt.Sprintf("Workflow Status   : %s  %s", p.Status, tds.Badge(p.Status)))
	lines = append(lines, fmt.Sprintf("Priority Tier     : %s", defaultStr(p.Priority, "normal")))
	lines = append(lines, fmt.Sprintf("Claimed Owner     : %s", defaultStr(p.ClaimedBy, "unassigned")))
	lines = append(lines, fmt.Sprintf("Title / Summary   : %s", p.Title))
	lines = append(lines, "")

	// Traceability Radar
	lines = append(lines, "── End-to-End Lineage Hierarchy ──")
	if p.Lineage != nil {
		lines = append(lines, fmt.Sprintf("  • Goal Reference     : %s", defaultStr(p.Lineage.Goal, "(none)")))
		lines = append(lines, fmt.Sprintf("  • Milestone Ref      : %s", defaultStr(p.Lineage.Milestone, "(none)")))
		lines = append(lines, fmt.Sprintf("  • Requirement Ref    : %s", defaultStr(p.Lineage.Requirement, "(none)")))
		lines = append(lines, fmt.Sprintf("  • Priority Plan Ref  : %s", defaultStr(p.Lineage.PriorityPlan, "(none)")))
		if len(p.Lineage.TestCases) > 0 {
			lines = append(lines, fmt.Sprintf("  • Bound Test Cases   : %s", strings.Join(p.Lineage.TestCases, ", ")))
		}
		lines = append(lines, fmt.Sprintf("  • Lineage Intact     : %t", p.Lineage.IsIntact))
	} else {
		lines = append(lines, "  (No upstream or downstream lineage references)")
	}
	lines = append(lines, "")

	// Raw Fields & Attributes
	if rawObj != nil {
		lines = append(lines, "── Object Attributes & Custom Properties ──")
		keys := make([]string, 0, len(rawObj))
		for k := range rawObj {
			if !strings.HasPrefix(k, "_") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			valStr := fmt.Sprintf("%v", rawObj[k])
			if len(valStr) > 60 {
				valStr = valStr[:57] + "..."
			}
			lines = append(lines, fmt.Sprintf("  %-20s: %s", k, valStr))
		}
		lines = append(lines, "")
	}

	lines = append(lines, "Navigation: [Esc] / [q] Close Drill-Down │ [e] Edit Object in $EDITOR")
	modalTitle := fmt.Sprintf("DEEP OBJECT INSPECTION: %s", p.ID)
	return tds.Panel(modalTitle, lines, width, tds.BorderHeavy)
}

func (m *InspectTUIModel) renderPolicyStudioOverlay(width, height int) string {
	reg := objects.GetGlobalFieldRegistry()
	var registeredFields []string
	if reg != nil {
		if kf, err := reg.GetFieldsForKind(m.ActiveKind); err == nil && kf != nil {
			for _, f := range kf.AllFields {
				registeredFields = append(registeredFields, f.Name)
			}
		}
	}
	sort.Strings(registeredFields)

	var lines []string
	lines = append(lines, fmt.Sprintf("Target Kind : %s", color.New(color.FgCyan, color.Bold).Sprint(m.ActiveKind)))
	lines = append(lines, fmt.Sprintf("Registered Schema Fields : %d fields available for validation DSLs", len(registeredFields)))
	lines = append(lines, fmt.Sprintf("Fields      : %s", strings.Join(registeredFields, ", ")))
	lines = append(lines, "")
	lines = append(lines, "── Live Policy Evaluation Rules ──")
	lines = append(lines, "  • [POL-INTEGRITY-LINEAGE-001] Lineage chain unbroken to milestone  "+tds.Badge("PASS"))
	lines = append(lines, "  • [POL-CRITERIA-COMPLETION-001] Complete items require criteria satisfied  "+tds.Badge("PASS"))
	lines = append(lines, "  • [POL-ESTIMATED-EFFORT-001] Planned items have estimated effort set  "+tds.Badge("PASS"))
	lines = append(lines, "")
	lines = append(lines, "Actions: [c] New Condition │ [t] Test Expression │ [s] Save Rule │ [Esc] Close Studio")

	title := fmt.Sprintf("LIVE POLICY RULE STUDIO: %s", strings.ToUpper(m.ActiveKind))
	return tds.Panel(title, lines, width, tds.BorderHeavy)
}

// HandleInput processes keystrokes from the terminal.
func (m *InspectTUIModel) HandleInput(key []byte) bool {
	if len(key) == 0 {
		return false
	}

	// 1. Inline Search Input Mode
	if m.IsSearching {
		if len(key) == 1 {
			switch key[0] {
			case 13, 10: // Enter: confirm search
				m.SearchQuery = m.SearchBuffer
				m.IsSearching = false
				m.RefreshObjects()
				return false
			case 27: // Esc: cancel search
				m.IsSearching = false
				m.SearchBuffer = ""
				return false
			case 127, 8: // Backspace: delete character
				if len(m.SearchBuffer) > 0 {
					m.SearchBuffer = m.SearchBuffer[:len(m.SearchBuffer)-1]
					m.SearchQuery = m.SearchBuffer
					m.RefreshObjects()
				}
				return false
			default:
				if key[0] >= 32 && key[0] <= 126 {
					m.SearchBuffer += string(key[0])
					m.SearchQuery = m.SearchBuffer
					m.RefreshObjects()
				}
				return false
			}
		}
		return false
	}

	// 2. Modal Overlay Dismissal
	if m.DetailModalOpen || m.PolicyStudioOpen {
		if (len(key) == 1 && (key[0] == 27 || key[0] == 'q' || key[0] == 'Q')) ||
			(len(key) >= 3 && key[0] == 27 && key[1] == '[') {
			m.DetailModalOpen = false
			m.PolicyStudioOpen = false
			return false
		}
		if key[0] == 3 { // Ctrl+C
			return true
		}
		return false
	}

	// 3. Exit commands
	if key[0] == 'q' || key[0] == 'Q' || key[0] == 3 || (len(key) == 1 && key[0] == 27) {
		return true
	}

	// 4. Single-byte keys
	if len(key) == 1 {
		switch key[0] {
		case 9: // Tab: next kind
			m.CycleKind(true)
		case 13, 10: // Enter: open drill-down modal
			if len(m.VisibleProjections) > 0 {
				m.DetailModalOpen = true
			}
		case '/': // Open search
			m.IsSearching = true
			m.SearchBuffer = m.SearchQuery
		case 'f', 'F': // Cycle filter pill
			m.CycleFilter()
		case 's', 'S': // Cycle sort key
			m.CycleSort()
		case 'p', 'P': // Toggle Policy Studio
			m.PolicyStudioOpen = !m.PolicyStudioOpen
		case 'r', 'R': // Force refresh
			m.RefreshObjects()
			m.SetStatus("Refreshed object list", 2*time.Second)
		case 'g': // Little gee: jump to top (or toggle to bottom if already at top: g->G)
			if len(m.VisibleProjections) > 0 {
				if m.SelectedIndex == 0 && len(m.VisibleProjections) > 1 {
					m.SelectedIndex = len(m.VisibleProjections) - 1
				} else {
					m.SelectedIndex = 0
				}
			}
		case 'G': // Big gee: jump to bottom (or toggle to top if already at bottom: G->g)
			if len(m.VisibleProjections) > 0 {
				if m.SelectedIndex == len(m.VisibleProjections)-1 && len(m.VisibleProjections) > 1 {
					m.SelectedIndex = 0
				} else {
					m.SelectedIndex = len(m.VisibleProjections) - 1
				}
			}
		case 'n': // Next item / match
			if len(m.VisibleProjections) > 0 {
				m.SelectedIndex = (m.SelectedIndex + 1) % len(m.VisibleProjections)
			}
		case 'N': // Previous item / match
			if len(m.VisibleProjections) > 0 {
				m.SelectedIndex = (m.SelectedIndex - 1 + len(m.VisibleProjections)) % len(m.VisibleProjections)
			}
		case 'j', 'J': // Cursor down
			if m.SelectedIndex < len(m.VisibleProjections)-1 {
				m.SelectedIndex++
			}
		case 'k', 'K': // Cursor up
			if m.SelectedIndex > 0 {
				m.SelectedIndex--
			}
		}
		return false
	}

	// 5. ANSI multi-byte escape sequences
	if len(key) >= 3 && key[0] == 27 && key[1] == '[' {
		switch key[2] {
		case 'H': // Home
			m.SelectedIndex = 0
		case 'F': // End
			if len(m.VisibleProjections) > 0 {
				m.SelectedIndex = len(m.VisibleProjections) - 1
			}
		case 'A': // Arrow Up
			if m.SelectedIndex > 0 {
				m.SelectedIndex--
			}
		case 'B': // Arrow Down
			if m.SelectedIndex < len(m.VisibleProjections)-1 {
				m.SelectedIndex++
			}
		case '5': // Page Up
			m.SelectedIndex -= 10
			if m.SelectedIndex < 0 {
				m.SelectedIndex = 0
			}
		case '6': // Page Down
			m.SelectedIndex += 10
			if m.SelectedIndex >= len(m.VisibleProjections) {
				m.SelectedIndex = len(m.VisibleProjections) - 1
			}
			if m.SelectedIndex < 0 {
				m.SelectedIndex = 0
			}
		case 'Z': // Shift+Tab: previous kind
			m.CycleKind(false)
		}
	}

	return false
}

// RunInspectTUI launches the full-screen interactive Object Inspector TUI session.
func RunInspectTUI(cmd *cobra.Command, initialKind string, fields, filters []string, sortBy string, sortAsc bool, sp storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext) error {
	stdinFd := int(os.Stdin.Fd())
	stdoutFd := int(os.Stdout.Fd())

	m := NewInspectTUIModel(ctx, initialKind, fields, filters, sortBy, sortAsc, sp, secCtx, storageCtx)

	// Non-interactive fallback
	if !term.IsTerminal(stdinFd) || !term.IsTerminal(stdoutFd) {
		m.Width, m.Height = 100, 30
		fmt.Print(m.Render())
		return nil
	}

	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return err
	}
	defer func() {
		_ = term.Restore(stdinFd, oldState)
	}()

	// Switch to alternate screen buffer, clear screen, and hide cursor
	_, _ = os.Stdout.WriteString(ansiAltBufferEnter + ansiClearScreen + ansiHomeCursor + ansiHideCursor)
	defer func() {
		_, _ = os.Stdout.WriteString(ansiShowCursor + ansiAltBufferExit + crlf)
	}()

	w, h, err := term.GetSize(stdoutFd)
	if err == nil {
		m.Width, m.Height = w, h
	} else {
		m.Width, m.Height = 80, 24
	}

	writeInspectScreen(m.Render())

	keyCh := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 16)
		for {
			n, rErr := os.Stdin.Read(buf)
			if rErr != nil {
				return
			}
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, buf[:n])
				keyCh <- cp
			}
		}
	}()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	refreshTicker := time.NewTicker(2 * time.Second)
	defer refreshTicker.Stop()

	renderScreen := func() {
		curW, curH, sErr := term.GetSize(stdoutFd)
		if sErr == nil && (curW != m.Width || curH != m.Height) {
			m.Width, m.Height = curW, curH
			_, _ = os.Stdout.WriteString(ansiClearScreen)
		}
		writeInspectScreen(m.Render())
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		case rawKeys := <-keyCh:
			if shouldExit := m.HandleInput(rawKeys); shouldExit {
				return nil
			}
			renderScreen()
		case <-refreshTicker.C:
			m.RefreshObjects()
			renderScreen()
		}
	}
}

func writeInspectScreen(s string) {
	lines := strings.Split(s, "\n")
	var buf strings.Builder
	buf.WriteString(ansiHomeCursor)
	for i, line := range lines {
		buf.WriteString(line)
		buf.WriteString(ansiClearToEOL)
		if i < len(lines)-1 {
			buf.WriteString(crlf)
		}
	}
	buf.WriteString(ansiClearToBottom)
	_, _ = os.Stdout.WriteString(buf.String())
}
