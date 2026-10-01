package object

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/tui/tds"
	"github.com/zqk-os/zqk/pkg/zqkenv"
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
	Storage            storage.ObjectStorageProvider
	SecCtx             *pkgctx.SecurityContext
	StorageCtx         *storage.StorageContext
	Ctx                context.Context
	ActiveKind         string
	AvailableKinds     []string
	KindIndex          int
	AllObjects         []map[string]any
	VisibleProjections []SemanticAgentProjection
	RawVisibleObjects  []map[string]any
	SelectedIndex      int
	ScrollOffset       int
	Width              int
	Height             int
	Fields             []string
	SortBy             string
	SortAsc            bool
	SortIndex          int
	FilterPill         string
	FilterIndex        int
	SearchQuery        string
	IsSearching        bool
	SearchBuffer       string
	DetailModalOpen    bool
	PolicyStudioOpen   bool
	PolicyRules        []PolicyRule
	ActiveRuleIndex    int
	DSLEditMode        bool
	DSLInputBuffer     string
	DSLSuggestions     []DSLTokenSuggestion
	DSLSuggestionIndex int
	DryRunResults      []RuleEvaluationResult
	ActionPaletteOpen  bool
	ActionIndex        int
	EditorProfile      string // "newb", "pro", "jedi"
	StatusMessage      string
	StatusExpiresAt    time.Time
}

// NewInspectTUIModel constructs a new interactive Object Inspector model.
func NewInspectTUIModel(ctx context.Context, initialKind string, fields, filters []string, sortBy string, sortAsc bool, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext) *InspectTUIModel {
	profile := "newb"
	if envProfile := zqkenv.EditorProfile().Get(); envProfile != "" {
		switch strings.ToLower(envProfile) {
		case "pro":
			profile = "pro"
		case "jedi":
			profile = "jedi"
		}
	}

	m := &InspectTUIModel{
		Ctx:           ctx,
		Storage:       sp,
		SecCtx:        secCtx,
		StorageCtx:    storageCtx,
		Fields:        fields,
		SortBy:        sortBy,
		SortAsc:       sortAsc,
		FilterPill:    "all",
		EditorProfile: profile,
		Width:         100,
		Height:        30,
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

// CycleEditorProfile toggles between newb, pro, and jedi editor profiles.
func (m *InspectTUIModel) CycleEditorProfile() {
	switch m.EditorProfile {
	case "newb", "":
		m.EditorProfile = "pro"
		m.SetStatus("Profile: PRO (compact header & property card)", 2*time.Second)
	case "pro":
		m.EditorProfile = "jedi"
		m.SetStatus("Profile: JEDI (zen mode — maximum table view)", 2*time.Second)
	case "jedi":
		m.EditorProfile = "newb"
		m.SetStatus("Profile: NEWB (full help & property card)", 2*time.Second)
	default:
		m.EditorProfile = "newb"
	}
}

// SetEditorProfile safely sets the profile to a valid preset.
func (m *InspectTUIModel) SetEditorProfile(profile string) {
	switch strings.ToLower(profile) {
	case "pro":
		m.EditorProfile = "pro"
	case "jedi":
		m.EditorProfile = "jedi"
	default:
		m.EditorProfile = "newb"
	}
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

	filterPillStr := fmt.Sprintf("[%s]", strings.ToUpper(m.FilterPill))
	sortPillStr := fmt.Sprintf("[%s %s]", m.SortBy, map[bool]string{true: "▲", false: "▼"}[m.SortAsc])
	searchIndicator := ""
	if m.SearchQuery != "" {
		searchIndicator = fmt.Sprintf("  │ Search: '%s'", m.SearchQuery)
	}

	// 1. Top Header Banner based on EditorProfile
	switch m.EditorProfile {
	case "jedi":
		// Jedi mode: completely collapse top header banner unless searching
	case "pro":
		proHeader := fmt.Sprintf("🔍 ZQK │ Kind: %s (%d) │ %s %s%s │ %s",
			m.ActiveKind, len(m.AllObjects),
			color.New(color.FgCyan, color.Bold).Sprint(filterPillStr),
			color.New(color.FgYellow).Sprint(sortPillStr),
			searchIndicator,
			color.New(color.FgYellow, color.Bold).Sprint("[PRO]"),
		)
		if m.StatusMessage != "" && time.Now().Before(m.StatusExpiresAt) {
			proHeader += fmt.Sprintf(" │ %s", color.New(color.FgGreen, color.Bold).Sprint(m.StatusMessage))
		}
		if tds.VisibleWidth(proHeader) > w {
			proHeader = tds.TruncateVisible(proHeader, w, "")
		}
		out.WriteString(tds.PadRight(proHeader, w) + "\n")
		out.WriteString(dim(strings.Repeat("─", w)) + "\n")
	default: // "newb"
		bannerTitle := fmt.Sprintf("🔍 ZQK OBJECT INSPECTOR — [Kind: %s] (%d active)", m.ActiveKind, len(m.AllObjects))
		var headerLines []string
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
	}

	// 2. Action Palette Overlay
	if m.ActionPaletteOpen && m.SelectedIndex < len(m.VisibleProjections) {
		out.WriteString(m.renderActionPaletteOverlay(w, h))
		return out.String()
	}

	// 3. Drill-Down Detail Modal Overlay
	if m.DetailModalOpen && m.SelectedIndex < len(m.VisibleProjections) {
		out.WriteString(m.renderDetailModal(w, h))
		return out.String()
	}

	// 4. Policy Studio Overlay
	if m.PolicyStudioOpen {
		out.WriteString(m.renderPolicyStudioOverlay(w, h))
		return out.String()
	}

	// 4. Master Table Height Budgeting by Profile
	var tableRows int
	switch m.EditorProfile {
	case "jedi":
		tableRows = h - 4
		if m.IsSearching {
			tableRows -= 2
		}
		if tableRows < 5 {
			tableRows = 5
		}
	case "pro":
		tableRows = (h - 10) * 2 / 3
		if tableRows < 6 {
			tableRows = 6
		}
	default: // "newb"
		tableRows = (h - 14) / 2
		if tableRows < 5 {
			tableRows = 5
		}
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

	// 5. Lower Pane: Inspected Object Property Card (hidden in jedi mode)
	if m.EditorProfile != "jedi" {
		if m.SelectedIndex < len(m.VisibleProjections) {
			sel := m.VisibleProjections[m.SelectedIndex]
			out.WriteString(m.renderPropertyCard(sel, w))
		} else {
			emptyCard := tds.Panel("INSPECTED OBJECT", []string{"No object selected"}, w, tds.BorderRounded)
			out.WriteString(emptyCard)
		}
		out.WriteString("\n")
	}

	// 6. Navigation Key Help Bar / Search Prompt
	if m.IsSearching {
		prompt := fmt.Sprintf("🔍 Search regex/substring: %s█  (Press [Enter] to apply, [Esc] to cancel)", m.SearchBuffer)
		out.WriteString(color.New(color.FgCyan, color.Bold).Sprint(prompt))
		out.WriteString("\n")
	} else {
		switch m.EditorProfile {
		case "jedi":
			// Zen mode: no footer help bar
		case "pro":
			helpBar := "Nav: [Tab] Kind │ [j/k/g/G] Select │ [/] Search │ [Enter] Drill-down │ [z/?] jedi │ [q] Quit"
			out.WriteString(color.New(color.Faint).Sprint(helpBar))
			out.WriteString("\n")
		default: // "newb"
			helpBar := "Nav: [Tab] Kind │ [j/k] Select │ [g/G] Top/Bottom │ [f] Filter │ [s] Sort │ [/] Search │ [Enter] Drill-down │ [p] Policy │ [z/?] Profile (newb) │ [q] Quit"
			out.WriteString(color.New(color.Faint).Sprint(helpBar))
			out.WriteString("\n")
		}
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

	// CAS Storage Profile
	if p.StorageProfile != nil {
		hashSnippet := p.StorageProfile.CASHash
		if len(hashSnippet) > 18 {
			hashSnippet = hashSnippet[:8] + "…" + hashSnippet[len(hashSnippet)-6:]
		}
		lines = append(lines, fmt.Sprintf("Storage: [%s] │ Hash: %s │ Size: %d B │ Mode: %s │ Mod: %s",
			strings.ToUpper(p.StorageProfile.StoragePlane),
			hashSnippet,
			p.StorageProfile.ByteSize,
			p.StorageProfile.Permissions,
			p.StorageProfile.LastModified,
		))
	}

	// Ontology & Schema Profile
	if p.Ontology != nil {
		traitsList := "none"
		if len(p.Ontology.Traits) > 0 {
			traitsList = strings.Join(p.Ontology.Traits, ", ")
		}
		lines = append(lines, fmt.Sprintf("Ontology: ns:%s │ ctx:%s │ profile:%s │ fields:%d │ traits:[%s]",
			p.Ontology.Namespace,
			p.Ontology.VersionContext,
			p.Ontology.StorageProfile,
			p.Ontology.RegisteredFieldsCount,
			traitsList,
		))
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

	lines = append(lines, "Actions: [a] Action Palette │ [Enter] Full Drill-Down │ [p] Policy Studio │ [c] Claim │ [t] State")

	cardTitle := fmt.Sprintf("── INSPECTED OBJECT: %s (Press [a] for Actions / [Enter] for Drill-Down) ──", p.ID)
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

	// CAS Storage & Data-Cell Profile
	if p.StorageProfile != nil {
		lines = append(lines, "── CAS Storage & Data-Cell Profile ──")
		lines = append(lines, fmt.Sprintf("  • Storage Plane      : %s", p.StorageProfile.StoragePlane))
		lines = append(lines, fmt.Sprintf("  • CAS Content Hash   : %s", p.StorageProfile.CASHash))
		lines = append(lines, fmt.Sprintf("  • Content Size       : %d bytes", p.StorageProfile.ByteSize))
		lines = append(lines, fmt.Sprintf("  • Permissions / Mode : %s", p.StorageProfile.Permissions))
		lines = append(lines, fmt.Sprintf("  • Last Modified Time : %s", p.StorageProfile.LastModified))
		if p.StorageProfile.FilePath != "" {
			lines = append(lines, fmt.Sprintf("  • File Path          : %s", p.StorageProfile.FilePath))
		}
		lines = append(lines, "")
	}

	// Ontology Profile
	if p.Ontology != nil {
		lines = append(lines, "── Ontology & Schema Profile ──")
		lines = append(lines, fmt.Sprintf("  • Namespace          : %s", p.Ontology.Namespace))
		lines = append(lines, fmt.Sprintf("  • Version Context    : %s", p.Ontology.VersionContext))
		lines = append(lines, fmt.Sprintf("  • Storage Profile    : %s", p.Ontology.StorageProfile))
		lines = append(lines, fmt.Sprintf("  • Registered Fields  : %d fields", p.Ontology.RegisteredFieldsCount))
		if len(p.Ontology.Traits) > 0 {
			lines = append(lines, fmt.Sprintf("  • Declared Traits    : %s", strings.Join(p.Ontology.Traits, ", ")))
		}
		lines = append(lines, "")
	}

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

	lines = append(lines, "Navigation: [Esc]/[q] Close Drill-Down │ [a] Action Palette │ [t] Status Transition │ [e] Edit Object │ [p] Policy Studio")
	modalTitle := fmt.Sprintf("DEEP OBJECT INSPECTION: %s", p.ID)
	return tds.Panel(modalTitle, lines, width, tds.BorderHeavy)
}

// ActionPaletteItem represents an executable action within the Role-Gated Action Palette.
type ActionPaletteItem struct {
	Key         rune
	Label       string
	Description string
	Gated       bool
	GateReason  string
}

func (m *InspectTUIModel) prepareActionItems() []ActionPaletteItem {
	if m.SelectedIndex >= len(m.VisibleProjections) {
		return nil
	}
	p := m.VisibleProjections[m.SelectedIndex]

	canDel, delReason := canDelete(m.SecCtx, p.Kind)
	canEd, edReason := canEdit(m.SecCtx, p.Kind)

	claimLabel := "Claim Work"
	claimDesc := "Assign current operator to object"
	if p.ClaimedBy != "" {
		claimLabel = "Unclaim Work"
		claimDesc = "Release assignment back to pool"
	}

	nextState := getNextLifecycleState(p.Kind, p.Status)

	return []ActionPaletteItem{
		{
			Key:         'c',
			Label:       claimLabel,
			Description: claimDesc,
			Gated:       false,
		},
		{
			Key:         't',
			Label:       "Transition Status",
			Description: fmt.Sprintf("Progress lifecycle state to: %s", nextState),
			Gated:       false,
		},
		{
			Key:         'e',
			Label:       "Edit in $EDITOR",
			Description: "Edit YAML properties with schema validation",
			Gated:       !canEd,
			GateReason:  edReason,
		},
		{
			Key:         'p',
			Label:       "Open Policy Studio",
			Description: "Inspect and test DSL validation rules",
			Gated:       false,
		},
		{
			Key:         'd',
			Label:       "Delete Object",
			Description: "Delete object permanently from CAS store",
			Gated:       !canDel,
			GateReason:  delReason,
		},
	}
}

func (m *InspectTUIModel) renderActionPaletteOverlay(width, height int) string {
	if m.SelectedIndex >= len(m.VisibleProjections) {
		return ""
	}
	p := m.VisibleProjections[m.SelectedIndex]
	items := m.prepareActionItems()

	actor := "anonymous"
	roles := "[]"
	perms := "[]"
	if m.SecCtx != nil {
		if m.SecCtx.AccountID != "" {
			actor = m.SecCtx.AccountID
		}
		if len(m.SecCtx.Roles) > 0 {
			roles = fmt.Sprintf("[%s]", strings.Join(m.SecCtx.Roles, ", "))
		}
		if len(m.SecCtx.Permissions) > 0 {
			perms = fmt.Sprintf("[%s]", strings.Join(m.SecCtx.Permissions, ", "))
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Target Object   : %s  │ Kind: %s  │ Status: %s %s",
		color.New(color.FgCyan, color.Bold).Sprint(p.ID),
		p.Kind,
		p.Status,
		tds.Badge(p.Status),
	))
	lines = append(lines, fmt.Sprintf("Caller Security : Actor: %s │ Roles: %s │ Permissions: %s",
		color.New(color.FgYellow).Sprint(actor),
		roles,
		perms,
	))
	lines = append(lines, "")
	lines = append(lines, "── Available Actions (Role-Gated) ──")

	for i, item := range items {
		cursor := "  "
		if i == m.ActionIndex {
			cursor = color.New(color.FgCyan, color.Bold).Sprint("➔ ")
		}

		keyBadge := color.New(color.FgWhite, color.Bold).Sprintf("[%c]", item.Key)
		labelStr := color.New(color.Bold).Sprint(item.Label)
		if item.Gated {
			labelStr = color.New(color.Faint).Sprint(item.Label)
			gateNotice := color.New(color.FgRed).Sprintf(" [LOCKED: %s]", item.GateReason)
			lines = append(lines, fmt.Sprintf("%s%s %-20s - %s%s", cursor, keyBadge, labelStr, color.New(color.Faint).Sprint(item.Description), gateNotice))
		} else {
			lines = append(lines, fmt.Sprintf("%s%s %-20s - %s", cursor, keyBadge, labelStr, color.New(color.FgHiBlack).Sprint(item.Description)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "Hotkeys: [↑/↓/j/k] Navigate │ [Enter] Execute Selected │ [c/t/e/p/d] Trigger │ [Esc] Close")

	title := fmt.Sprintf("⚡ ROLE-GATED ACTION PALETTE: %s", p.ID)
	return tds.Panel(title, lines, width, tds.BorderHeavy)
}

func (m *InspectTUIModel) executeCurrentAction() {
	items := m.prepareActionItems()
	if m.ActionIndex < 0 || m.ActionIndex >= len(items) {
		return
	}
	item := items[m.ActionIndex]
	if item.Gated {
		m.SetStatus(fmt.Sprintf("Action '%s' is locked: %s", item.Label, item.GateReason), 3*time.Second)
		return
	}
	switch item.Key {
	case 'c':
		m.executeClaimToggle()
	case 't':
		m.executeStatusTransition()
	case 'e':
		m.executeEditObject()
	case 'p':
		m.ActionPaletteOpen = false
		m.PolicyStudioOpen = true
	case 'd':
		m.executeDeleteObject()
	}
}

func (m *InspectTUIModel) executeClaimToggle() {
	if m.SelectedIndex >= len(m.VisibleProjections) {
		return
	}
	p := m.VisibleProjections[m.SelectedIndex]
	actor := "ACC-OPERATOR"
	if m.SecCtx != nil && m.SecCtx.AccountID != "" {
		actor = m.SecCtx.AccountID
	}

	var updates map[string]any
	var statusMsg string
	if p.ClaimedBy != "" {
		updates = map[string]any{"claimed_by": ""}
		statusMsg = "Work unclaimed: " + p.ID
	} else {
		updates = map[string]any{"claimed_by": actor}
		statusMsg = fmt.Sprintf("Work claimed by %s for %s", actor, p.ID)
	}

	if err := m.Storage.Update(m.Ctx, m.SecCtx, p.ID, updates); err != nil {
		m.SetStatus("Claim update failed: "+err.Error(), 3*time.Second)
		return
	}

	m.SetStatus(statusMsg, 3*time.Second)
	m.ActionPaletteOpen = false
	m.RefreshObjects()
}

func (m *InspectTUIModel) executeStatusTransition() {
	if m.SelectedIndex >= len(m.VisibleProjections) {
		return
	}
	p := m.VisibleProjections[m.SelectedIndex]
	nextStatus := getNextLifecycleState(p.Kind, p.Status)

	updates := map[string]any{"status": nextStatus}
	if err := m.Storage.Update(m.Ctx, m.SecCtx, p.ID, updates); err != nil {
		m.SetStatus("Transition failed: "+err.Error(), 3*time.Second)
		return
	}

	m.SetStatus(fmt.Sprintf("Status transitioned: %s ➔ %s for %s", p.Status, nextStatus, p.ID), 3*time.Second)
	m.ActionPaletteOpen = false
	m.RefreshObjects()
}

func (m *InspectTUIModel) executeEditObject() {
	if m.SelectedIndex >= len(m.VisibleProjections) {
		return
	}
	p := m.VisibleProjections[m.SelectedIndex]
	allowed, reason := canEdit(m.SecCtx, p.Kind)
	if !allowed {
		m.SetStatus("Permission denied for edit: "+reason, 3*time.Second)
		m.ActionPaletteOpen = false
		return
	}

	rawObj, err := m.Storage.Read(m.Ctx, m.SecCtx, p.ID)
	if err != nil || rawObj == nil {
		m.SetStatus("Failed to read object for edit: "+p.ID, 3*time.Second)
		return
	}

	data, err := yaml.Marshal(rawObj)
	if err != nil {
		m.SetStatus("Failed to serialize object: "+err.Error(), 3*time.Second)
		return
	}

	tmpFile, err := os.CreateTemp("", fmt.Sprintf("zqk-edit-%s-*.yaml", p.ID))
	if err != nil {
		m.SetStatus("Failed to create temp file: "+err.Error(), 3*time.Second)
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		m.SetStatus("Failed to write temp file: "+err.Error(), 3*time.Second)
		return
	}
	_ = tmpFile.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		cmd := exec.Command(editor, tmpPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}

	editedBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		m.SetStatus("Failed to read edited file", 3*time.Second)
		return
	}

	var updatedObj map[string]any
	if err := yaml.Unmarshal(editedBytes, &updatedObj); err != nil {
		m.SetStatus("Invalid YAML: "+err.Error(), 3*time.Second)
		return
	}

	if err := m.Storage.Update(m.Ctx, m.SecCtx, p.ID, updatedObj); err != nil {
		m.SetStatus("Update failed: "+err.Error(), 3*time.Second)
		return
	}

	m.SetStatus("Successfully updated: "+p.ID, 3*time.Second)
	m.ActionPaletteOpen = false
	m.RefreshObjects()
}

func (m *InspectTUIModel) executeDeleteObject() {
	if m.SelectedIndex >= len(m.VisibleProjections) {
		return
	}
	p := m.VisibleProjections[m.SelectedIndex]
	allowed, reason := canDelete(m.SecCtx, p.Kind)
	if !allowed {
		m.SetStatus("Permission denied for delete: "+reason, 3*time.Second)
		m.ActionPaletteOpen = false
		return
	}

	if err := m.Storage.Delete(m.Ctx, m.SecCtx, p.ID, false); err != nil {
		m.SetStatus("Failed to delete "+p.ID+": "+err.Error(), 3*time.Second)
		return
	}

	m.SetStatus("Object deleted from CAS: "+p.ID, 3*time.Second)
	m.ActionPaletteOpen = false
	m.DetailModalOpen = false
	m.RefreshObjects()
}

func canDelete(secCtx *pkgctx.SecurityContext, kind string) (bool, string) {
	if secCtx == nil {
		return false, "unauthenticated"
	}
	for _, r := range secCtx.Roles {
		if r == "admin" || r == "test" || r == "system" {
			return true, ""
		}
	}
	for _, p := range secCtx.Permissions {
		if p == "delete:*" || p == "delete:"+kind || p == "access:*" || p == "write:*" {
			return true, ""
		}
	}
	return false, "requires role:admin or permission:delete:object"
}

func canEdit(secCtx *pkgctx.SecurityContext, kind string) (bool, string) {
	if secCtx == nil {
		return false, "unauthenticated"
	}
	for _, r := range secCtx.Roles {
		if r == "admin" || r == "test" || r == "system" || r == "developer" || r == "operator" {
			return true, ""
		}
	}
	for _, p := range secCtx.Permissions {
		if p == "write:*" || p == "write:"+kind || p == "access:*" {
			return true, ""
		}
	}
	if secCtx.AccountID != "" {
		return true, ""
	}
	return false, "requires authenticated operator"
}

func getNextLifecycleState(kind, currentStatus string) string {
	switch kind {
	case objects.KindBacklogItem:
		switch currentStatus {
		case "originated", "planned":
			return "in_progress"
		case "in_progress":
			return "review"
		case "review":
			return "complete"
		case "complete":
			return "originated"
		default:
			return "in_progress"
		}
	case objects.KindPriorityPlan:
		switch currentStatus {
		case "planned":
			return "in_progress"
		case "in_progress":
			return "complete"
		default:
			return "in_progress"
		}
	default:
		switch currentStatus {
		case "draft", "originated":
			return "in_progress"
		case "in_progress":
			return "complete"
		case "complete":
			return "draft"
		default:
			return "in_progress"
		}
	}
}

func (m *InspectTUIModel) RefreshPolicyStudioDryRun() {
	if len(m.PolicyRules) == 0 {
		m.PolicyRules = DefaultPolicyRulesForKind(m.ActiveKind)
	}

	var results []RuleEvaluationResult
	evalObjects := m.AllObjects
	if len(evalObjects) == 0 && m.Storage != nil {
		if qRes, err := m.Storage.List(m.Ctx, m.SecCtx, m.StorageCtx, storage.ListFilter{Kind: m.ActiveKind}); err == nil && qRes != nil {
			evalObjects = qRes.Objects
		}
	}

	for _, rule := range m.PolicyRules {
		res := RuleEvaluationResult{
			RuleID:         rule.ID,
			Name:           rule.Name,
			Expression:     rule.Expression,
			TotalEvaluated: len(evalObjects),
			Passed:         true,
		}

		for _, obj := range evalObjects {
			id, _ := obj[objects.FieldKeyID].(string)
			matches, err := EvaluateDSLExpression(rule.Expression, obj)
			if err != nil || !matches {
				res.ViolationsCount++
				res.Passed = false
				if len(res.OffendingIDs) < 5 {
					res.OffendingIDs = append(res.OffendingIDs, id)
				}
			}
		}

		if res.Passed {
			res.Summary = fmt.Sprintf("All %d %s objects satisfy rule", len(evalObjects), m.ActiveKind)
		} else {
			res.Summary = fmt.Sprintf("%d of %d objects violate rule (offenders: %s)", res.ViolationsCount, len(evalObjects), strings.Join(res.OffendingIDs, ", "))
		}

		results = append(results, res)
	}
	m.DryRunResults = results
}

func (m *InspectTUIModel) renderPolicyStudioOverlay(width, height int) string {
	if len(m.PolicyRules) == 0 {
		m.PolicyRules = DefaultPolicyRulesForKind(m.ActiveKind)
		m.RefreshPolicyStudioDryRun()
	}

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
	headerStr := fmt.Sprintf("Target Kind: %s  │  Available Schema Fields: %d",
		color.New(color.FgCyan, color.Bold).Sprint(m.ActiveKind),
		len(registeredFields),
	)
	lines = append(lines, headerStr)
	lines = append(lines, "")

	lines = append(lines, "── Active Evaluation Rules (Select with [j/k], [c] to Edit Condition, [t] to Test) ──")
	for i, rule := range m.PolicyRules {
		cursor := "  "
		if i == m.ActiveRuleIndex {
			cursor = "❯ "
		}

		statusBadge := tds.Badge("PASS")
		violStr := ""
		if i < len(m.DryRunResults) {
			r := m.DryRunResults[i]
			if !r.Passed {
				statusBadge = tds.Badge("FAIL")
				violStr = fmt.Sprintf(" (%d violations)", r.ViolationsCount)
			}
		}

		line := fmt.Sprintf("%s[%s] %s  %s%s", cursor, rule.ID, rule.Name, statusBadge, violStr)
		if i == m.ActiveRuleIndex {
			line = color.New(color.FgHiWhite, color.Bold).Sprint(line)
		}
		lines = append(lines, line)
		lines = append(lines, fmt.Sprintf("    DSL: %s", color.New(color.FgYellow).Sprint(rule.Expression)))
	}
	lines = append(lines, "")

	// If DSL Edit Mode is active, render interactive input editor and autocomplete suggestions
	if m.DSLEditMode {
		lines = append(lines, "── Live DSL Predicate Editor ──")
		inputDisplay := fmt.Sprintf("Condition > %s", m.DSLInputBuffer)
		lines = append(lines, color.New(color.FgGreen, color.Bold).Sprint(inputDisplay)+"█")

		if len(m.DSLSuggestions) > 0 {
			lines = append(lines, "")
			lines = append(lines, "Autocomplete Suggestions (Press [Tab] to insert, [↑/↓] to cycle):")
			var pills []string
			maxShow := 8
			if len(m.DSLSuggestions) < maxShow {
				maxShow = len(m.DSLSuggestions)
			}
			for si := 0; si < maxShow; si++ {
				s := m.DSLSuggestions[si]
				pillText := fmt.Sprintf("[%s]", s.Token)
				if si == m.DSLSuggestionIndex {
					pills = append(pills, color.New(color.BgCyan, color.FgBlack, color.Bold).Sprint(pillText))
				} else {
					pills = append(pills, color.New(color.FgCyan).Sprint(pillText))
				}
			}
			if len(m.DSLSuggestions) > maxShow {
				pills = append(pills, fmt.Sprintf("+%d more", len(m.DSLSuggestions)-maxShow))
			}
			lines = append(lines, "  "+strings.Join(pills, " "))
			if m.DSLSuggestionIndex < len(m.DSLSuggestions) {
				sel := m.DSLSuggestions[m.DSLSuggestionIndex]
				lines = append(lines, fmt.Sprintf("  ↳ %s: %s", sel.Token, sel.Description))
			}
		}
		lines = append(lines, "")
		lines = append(lines, "Actions: [Tab] Complete Token  │  [Enter] Apply Rule  │  [Esc] Cancel Edit")
	} else {
		// Dry Run Outcome summary
		if len(m.DryRunResults) > 0 && m.ActiveRuleIndex < len(m.DryRunResults) {
			res := m.DryRunResults[m.ActiveRuleIndex]
			lines = append(lines, "── Dry-Run Verification Summary ──")
			lines = append(lines, fmt.Sprintf("  • Status      : %s", res.Summary))
			if len(res.OffendingIDs) > 0 {
				lines = append(lines, fmt.Sprintf("  • Violations  : %s", strings.Join(res.OffendingIDs, ", ")))
			}
			lines = append(lines, "")
		}
		lines = append(lines, "Actions: [c] Edit Condition │ [t] Re-run Dry Run │ [j/k] Navigate │ [Esc]/[q] Close Studio")
	}

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

	// 2. Action Palette Modal
	if m.ActionPaletteOpen {
		if len(key) == 1 {
			switch key[0] {
			case 27, 'q', 'Q': // Esc / q closes action palette
				m.ActionPaletteOpen = false
				return false
			case 'j': // Next action
				items := m.prepareActionItems()
				if m.ActionIndex < len(items)-1 {
					m.ActionIndex++
				}
				return false
			case 'k': // Prev action
				if m.ActionIndex > 0 {
					m.ActionIndex--
				}
				return false
			case 13, 10: // Enter: execute selected action
				m.executeCurrentAction()
				return false
			case 'c', 'C': // Direct hotkey Claim/Unclaim
				m.executeClaimToggle()
				return false
			case 't', 'T': // Direct hotkey Transition Status
				m.executeStatusTransition()
				return false
			case 'e', 'E': // Direct hotkey Edit in $EDITOR
				m.executeEditObject()
				return false
			case 'p', 'P': // Direct hotkey Policy Studio
				m.ActionPaletteOpen = false
				m.PolicyStudioOpen = true
				return false
			case 'd', 'D': // Direct hotkey Delete
				m.executeDeleteObject()
				return false
			}
		} else if len(key) >= 3 && key[0] == 27 && key[1] == '[' {
			switch key[2] {
			case 'A': // Up
				if m.ActionIndex > 0 {
					m.ActionIndex--
				}
				return false
			case 'B': // Down
				items := m.prepareActionItems()
				if m.ActionIndex < len(items)-1 {
					m.ActionIndex++
				}
				return false
			}
		}
		if key[0] == 3 { // Ctrl+C
			return true
		}
		return false
	}

	// 2.5 Policy Studio Modal
	if m.PolicyStudioOpen {
		if m.DSLEditMode {
			if len(key) == 1 {
				switch key[0] {
				case 27: // Esc: cancel edit mode
					m.DSLEditMode = false
					m.DSLSuggestions = nil
					m.SetStatus("Exited DSL edit mode", 2*time.Second)
					return false
				case 13, 10: // Enter: commit condition
					trimmed := strings.TrimSpace(m.DSLInputBuffer)
					if trimmed != "" {
						if m.ActiveRuleIndex < len(m.PolicyRules) {
							m.PolicyRules[m.ActiveRuleIndex].Expression = trimmed
						} else {
							m.PolicyRules = append(m.PolicyRules, PolicyRule{
								ID:          fmt.Sprintf("POL-CUSTOM-%03d", len(m.PolicyRules)+1),
								Name:        "Custom Policy Rule",
								Description: "User-defined DSL verification condition",
								TargetKind:  m.ActiveKind,
								Expression:  trimmed,
								Severity:    "warning",
							})
						}
						m.RefreshPolicyStudioDryRun()
						m.SetStatus("Applied DSL condition to policy rule", 3*time.Second)
					}
					m.DSLEditMode = false
					m.DSLSuggestions = nil
					return false
				case 9: // Tab: insert suggestion
					if len(m.DSLSuggestions) > 0 {
						if m.DSLSuggestionIndex >= len(m.DSLSuggestions) {
							m.DSLSuggestionIndex = 0
						}
						token := m.DSLSuggestions[m.DSLSuggestionIndex].Token
						parts := strings.Fields(m.DSLInputBuffer)
						if len(parts) > 0 && !strings.HasSuffix(m.DSLInputBuffer, " ") {
							parts[len(parts)-1] = token
							m.DSLInputBuffer = strings.Join(parts, " ") + " "
						} else {
							m.DSLInputBuffer = strings.TrimSpace(m.DSLInputBuffer+" "+token) + " "
						}
						m.DSLSuggestions = SuggestDSLTokens(m.ActiveKind, m.DSLInputBuffer)
						m.DSLSuggestionIndex = 0
					}
					return false
				case 127, 8: // Backspace
					if len(m.DSLInputBuffer) > 0 {
						m.DSLInputBuffer = m.DSLInputBuffer[:len(m.DSLInputBuffer)-1]
						m.DSLSuggestions = SuggestDSLTokens(m.ActiveKind, m.DSLInputBuffer)
						m.DSLSuggestionIndex = 0
					}
					return false
				default:
					if key[0] >= 32 && key[0] <= 126 {
						m.DSLInputBuffer += string(key[0])
						m.DSLSuggestions = SuggestDSLTokens(m.ActiveKind, m.DSLInputBuffer)
						m.DSLSuggestionIndex = 0
						return false
					}
				}
			} else if len(key) >= 3 && key[0] == 27 && key[1] == '[' {
				switch key[2] {
				case 'A': // Up suggestion
					if len(m.DSLSuggestions) > 0 {
						if m.DSLSuggestionIndex > 0 {
							m.DSLSuggestionIndex--
						} else {
							m.DSLSuggestionIndex = len(m.DSLSuggestions) - 1
						}
					}
					return false
				case 'B': // Down suggestion
					if len(m.DSLSuggestions) > 0 {
						if m.DSLSuggestionIndex < len(m.DSLSuggestions)-1 {
							m.DSLSuggestionIndex++
						} else {
							m.DSLSuggestionIndex = 0
						}
					}
					return false
				}
			}
			if key[0] == 3 {
				return true
			}
			return false
		}

		// Navigation Mode within Policy Studio
		if len(key) == 1 {
			switch key[0] {
			case 27, 'q', 'Q':
				m.PolicyStudioOpen = false
				m.SetStatus("Closed Policy Studio", 2*time.Second)
				return false
			case 'j':
				if m.ActiveRuleIndex < len(m.PolicyRules)-1 {
					m.ActiveRuleIndex++
				}
				return false
			case 'k':
				if m.ActiveRuleIndex > 0 {
					m.ActiveRuleIndex--
				}
				return false
			case 'c', 'C':
				m.DSLEditMode = true
				if m.ActiveRuleIndex < len(m.PolicyRules) {
					m.DSLInputBuffer = m.PolicyRules[m.ActiveRuleIndex].Expression
				} else {
					m.DSLInputBuffer = ""
				}
				m.DSLSuggestions = SuggestDSLTokens(m.ActiveKind, m.DSLInputBuffer)
				m.DSLSuggestionIndex = 0
				m.SetStatus("DSL Edit Mode: type condition, [Tab] autocomplete, [Enter] apply", 3*time.Second)
				return false
			case 't', 'T':
				m.RefreshPolicyStudioDryRun()
				m.SetStatus(fmt.Sprintf("Re-evaluated %d policy rules against objects", len(m.PolicyRules)), 3*time.Second)
				return false
			}
		} else if len(key) >= 3 && key[0] == 27 && key[1] == '[' {
			switch key[2] {
			case 'A': // Up
				if m.ActiveRuleIndex > 0 {
					m.ActiveRuleIndex--
				}
				return false
			case 'B': // Down
				if m.ActiveRuleIndex < len(m.PolicyRules)-1 {
					m.ActiveRuleIndex++
				}
				return false
			}
		}
		if key[0] == 3 {
			return true
		}
		return false
	}

	// 3. Detail Modal Dismissal
	if m.DetailModalOpen {
		if len(key) == 1 {
			switch key[0] {
			case 27, 'q', 'Q':
				m.DetailModalOpen = false
				return false
			case 'a', 'A':
				m.ActionPaletteOpen = true
				m.ActionIndex = 0
				return false
			case 't', 'T':
				m.executeStatusTransition()
				return false
			case 'e', 'E':
				m.executeEditObject()
				return false
			case 'p', 'P':
				m.DetailModalOpen = false
				m.PolicyStudioOpen = true
				m.PolicyRules = DefaultPolicyRulesForKind(m.ActiveKind)
				m.ActiveRuleIndex = 0
				m.DSLEditMode = false
				m.DSLInputBuffer = ""
				m.DSLSuggestions = nil
				m.RefreshPolicyStudioDryRun()
				return false
			}
		}
		if len(key) >= 3 && key[0] == 27 && key[1] == '[' {
			m.DetailModalOpen = false
			return false
		}
		if key[0] == 3 { // Ctrl+C
			return true
		}
		return false
	}

	// 4. Exit commands
	if key[0] == 'q' || key[0] == 'Q' || key[0] == 3 || (len(key) == 1 && key[0] == 27) {
		return true
	}

	// 5. Single-byte keys
	if len(key) == 1 {
		switch key[0] {
		case 9: // Tab: next kind
			m.CycleKind(true)
		case 13, 10: // Enter: open drill-down modal
			if len(m.VisibleProjections) > 0 {
				m.DetailModalOpen = true
			}
		case 'a', 'A': // Open role-gated Action Palette
			if len(m.VisibleProjections) > 0 {
				m.ActionPaletteOpen = true
				m.ActionIndex = 0
			}
		case 'c', 'C': // Direct claim toggle
			if len(m.VisibleProjections) > 0 {
				m.executeClaimToggle()
			}
		case 't', 'T': // Direct status transition
			if len(m.VisibleProjections) > 0 {
				m.executeStatusTransition()
			}
		case 'e', 'E': // Direct edit
			if len(m.VisibleProjections) > 0 {
				m.executeEditObject()
			}
		case 'd', 'D': // Direct delete prompt via Action Palette
			if len(m.VisibleProjections) > 0 {
				m.ActionPaletteOpen = true
				m.ActionIndex = 4 // Index of Delete Object
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
			if m.PolicyStudioOpen {
				m.DetailModalOpen = false
				m.ActionPaletteOpen = false
				m.PolicyRules = DefaultPolicyRulesForKind(m.ActiveKind)
				m.ActiveRuleIndex = 0
				m.DSLEditMode = false
				m.DSLInputBuffer = ""
				m.DSLSuggestions = nil
				m.RefreshPolicyStudioDryRun()
				m.SetStatus(fmt.Sprintf("Opened Policy Studio for %s", m.ActiveKind), 3*time.Second)
			} else {
				m.DSLEditMode = false
				m.SetStatus("Closed Policy Studio", 2*time.Second)
			}
		case 'z', 'Z', '?': // Cycle editor profile: newb -> pro -> jedi
			m.CycleEditorProfile()
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
	reconnect := cli.DisconnectTimeoutMonitor(func() {
		cli.TouchMeaningfulActivity()
	})
	defer reconnect()

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
	goroutinelabels.NewGoroutine("tui_stdin_reader", "reading keyboard input for inspect TUI").
		AsControlPlane().
		StartSimple(func() {
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
		})

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
			if cli.IsTimeoutMonitorDisconnected() {
				continue
			}
			return nil
		case <-sigCh:
			return nil
		case rawKeys := <-keyCh:
			cli.TouchMeaningfulActivity()
			if shouldExit := m.HandleInput(rawKeys); shouldExit {
				return nil
			}
			renderScreen()
		case <-refreshTicker.C:
			cli.TouchMeaningfulActivity()
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

func dim(s string) string {
	return color.New(color.Faint).Sprint(s)
}
