package main

import (
	"encoding/xml"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/test"
	"github.com/zqk-os/zqk/cmd/zqk/ui"
	"github.com/zqk-os/zqk/pkg/paths"
)

type Span struct {
	Col     int
	Text    string
	Color   string
	Bold    bool
	Dim     bool
	Reverse bool
}

type StyleState struct {
	Color   string
	Bold    bool
	Dim     bool
	Reverse bool
}

// Catppuccin Mocha terminal color palette
var fgColors = map[int]string{
	30: "#45475a", // black / surface1
	31: "#f38ba8", // red
	32: "#a6e3a1", // green
	33: "#f9e2af", // yellow
	34: "#89b4fa", // blue
	35: "#cba6f7", // mauve / magenta
	36: "#94e2d5", // teal / cyan
	37: "#ffffff", // white
	39: "#cdd6f4", // default text
	90: "#6c7086", // overlay0 (gray/dim)
}

func parseANSILine(raw string) []Span {
	var spans []Span
	curr := StyleState{Color: "#cdd6f4"}

	idx := 0
	col := 0
	runes := []rune(raw)

	for idx < len(runes) {
		if runes[idx] == 0x1b && idx+1 < len(runes) && runes[idx+1] == '[' {
			end := idx + 2
			for end < len(runes) && runes[end] != 'm' {
				end++
			}
			if end < len(runes) && runes[end] == 'm' {
				codeStr := string(runes[idx+2 : end])
				if codeStr == "" || codeStr == "0" {
					curr = StyleState{Color: "#cdd6f4"}
				} else {
					parts := strings.Split(codeStr, ";")
					for _, p := range parts {
						val, _ := strconv.Atoi(p)
						switch val {
						case 0:
							curr = StyleState{Color: "#cdd6f4"}
						case 1:
							curr.Bold = true
						case 2:
							curr.Dim = true
						case 7:
							curr.Reverse = true
						case 22:
							curr.Bold = false
							curr.Dim = false
						case 27:
							curr.Reverse = false
						default:
							if c, ok := fgColors[val]; ok {
								curr.Color = c
							}
						}
					}
				}
				idx = end + 1
				continue
			}
		}

		r := runes[idx]
		rw := runewidth.RuneWidth(r)
		if rw <= 0 {
			idx++
			continue
		}

		startCol := col
		var b strings.Builder
		b.WriteRune(r)
		col += rw
		idx++

		for idx < len(runes) {
			if runes[idx] == 0x1b {
				break
			}
			nr := runes[idx]
			nrw := runewidth.RuneWidth(nr)
			if nrw <= 0 {
				idx++
				continue
			}
			b.WriteRune(nr)
			col += nrw
			idx++
		}

		spans = append(spans, Span{
			Col:     startCol,
			Text:    b.String(),
			Color:   curr.Color,
			Bold:    curr.Bold,
			Dim:     curr.Dim,
			Reverse: curr.Reverse,
		})
	}

	return spans
}

func RenderScreenToSVG(filename, title string, screenText string, widthCols int) error {
	lines := strings.Split(screenText, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	charWidth := 8.4
	lineHeight := 19.0
	padX := 20.0
	padY := 14.0
	topBarH := 38.0

	totalW := int(float64(widthCols)*charWidth + padX*2)
	if totalW < 960 {
		totalW = 960
	}
	totalH := int(topBarH + padY*2 + float64(len(lines))*lineHeight + 8)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`+"\n",
		totalW, totalH, totalW, totalH))
	sb.WriteString(`  <defs>
    <style>
      .window-bg { fill: #1e1e2e; rx: 12px; }
      .top-bar { fill: #181825; }
      .dot-red { fill: #f38ba8; }
      .dot-yellow { fill: #f9e2af; }
      .dot-green { fill: #a6e3a1; }
      .term-text { font-family: "JetBrains Mono", "Fira Code", "Menlo", "Monaco", "Consolas", monospace;
                   font-size: 13px; fill: #cdd6f4; white-space: pre; }
      .title-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
                    font-size: 12px; fill: #a6adc8; font-weight: 500; text-anchor: middle; }
    </style>
    <filter id="shadow" x="-5%" y="-5%" width="110%" height="110%">
      <feDropShadow dx="0" dy="6" stdDeviation="12" flood-color="#000000" flood-opacity="0.45"/>
    </filter>
  </defs>
`)

	sb.WriteString(fmt.Sprintf(`  <rect x="4" y="4" width="%d" height="%d" class="window-bg" filter="url(#shadow)" stroke="#313244" stroke-width="1"/>`+"\n",
		totalW-8, totalH-8))
	sb.WriteString(fmt.Sprintf(`  <path d="M 4 14 A 10 10 0 0 1 14 4 L %d 4 A 10 10 0 0 1 %d 14 L %d %d L 4 %d Z" class="top-bar"/>`+"\n",
		totalW-14, totalW-4, totalW-4, int(topBarH), int(topBarH)))
	sb.WriteString(fmt.Sprintf(`  <line x1="4" y1="%d" x2="%d" y2="%d" stroke="#313244" stroke-width="1"/>`+"\n",
		int(topBarH), totalW-4, int(topBarH)))
	sb.WriteString(`  <circle cx="22" cy="21" r="5.5" class="dot-red"/>
  <circle cx="40" cy="21" r="5.5" class="dot-yellow"/>
  <circle cx="58" cy="21" r="5.5" class="dot-green"/>
`)
	sb.WriteString(fmt.Sprintf(`  <text x="%f" y="25" class="title-text">%s</text>`+"\n",
		float64(totalW)/2.0, html.EscapeString(title)))

	sb.WriteString(fmt.Sprintf(`  <g transform="translate(%f, %f)">`+"\n", padX, topBarH+padY))
	sb.WriteString(`    <text class="term-text">` + "\n")

	for rowIdx, line := range lines {
		spans := parseANSILine(line)
		yPos := float64(rowIdx+1)*lineHeight - 4.0

		for _, sp := range spans {
			var attrs []string
			attrs = append(attrs, fmt.Sprintf(`x="%.1f"`, float64(sp.Col)*charWidth))
			attrs = append(attrs, fmt.Sprintf(`y="%.1f"`, yPos))

			fill := sp.Color
			if sp.Dim {
				fill = "#6c7086"
			}
			if sp.Reverse {
				fill = "#11111b"
			}
			attrs = append(attrs, fmt.Sprintf(`fill="%s"`, fill))

			if sp.Bold {
				attrs = append(attrs, `font-weight="bold"`)
			}

			escapedText := html.EscapeString(sp.Text)
			escapedText = strings.ReplaceAll(escapedText, "\x1b", "")

			sb.WriteString(fmt.Sprintf(`      <tspan %s>%s</tspan>`+"\n", strings.Join(attrs, " "), escapedText))
		}
	}

	sb.WriteString(`    </text>
  </g>
</svg>
`)

	svgContent := sb.String()
	var dump any
	if err := xml.Unmarshal([]byte(svgContent), &dump); err != nil {
		return fmt.Errorf("XML validation failed for %s: %w", filename, err)
	}

	return os.WriteFile(filename, []byte(svgContent), paths.FilePerm644)
}

func RenderWebStudioSVG(filename string) error {
	width := 1040
	height := 720

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`+"\n",
		width, height, width, height))

	sb.WriteString(`  <defs>
    <style>
      .browser-bg { fill: #0d1117; rx: 12px; }
      .browser-top { fill: #161b22; }
      .dot-red { fill: #ff5f56; }
      .dot-yellow { fill: #ffbd2e; }
      .dot-green { fill: #27c93f; }
      .url-bar { fill: #0d1117; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .url-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #8b949e; }
      .url-host { fill: #f0f6fc; font-weight: 500; }
      .app-header { fill: #161b22; }
      .header-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 14px; font-weight: 600; fill: #f0f6fc; }
      .nav-tab-active { fill: #21262d; stroke: #58a6ff; stroke-width: 1; rx: 4px; }
      .nav-tab-inactive { fill: transparent; rx: 4px; }
      .nav-text-active { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 600; fill: #58a6ff; }
      .nav-text-inactive { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 500; fill: #8b949e; }
      .search-box { fill: #0d1117; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .search-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #8b949e; }
      .status-pill-bg { fill: rgba(63, 185, 80, 0.12); rx: 12px; }
      .status-pill-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 600; fill: #3fb950; }
      .filter-chip { fill: #21262d; stroke: #30363d; stroke-width: 1; rx: 4px; }
      .filter-chip-active { fill: #58a6ff; rx: 4px; }
      .chip-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 10px; font-weight: 600; fill: #8b949e; }
      .chip-text-active { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 10px; font-weight: 600; fill: #ffffff; }
      .node-card { fill: #161b22; stroke: #30363d; stroke-width: 1.5; rx: 8px; }
      .node-card-selected { fill: #1c2128; stroke: #58a6ff; stroke-width: 2.5; rx: 8px; }
      .node-kind { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 9px; font-weight: 700; letter-spacing: 0.5px; }
      .node-status { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 9px; font-weight: 600; fill: #8b949e; text-anchor: end; }
      .node-id { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11px; font-weight: 700; fill: #f0f6fc; }
      .node-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #c9d1d9; }
      .edge { fill: none; stroke: #30363d; stroke-width: 1.8; }
      .edge-active { fill: none; stroke: #58a6ff; stroke-width: 2.4; }
      .drawer-bg { fill: #161b22; stroke: #30363d; stroke-width: 1; }
      .drawer-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 700; fill: #f0f6fc; letter-spacing: 0.5px; }
      .drawer-label { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 500; fill: #8b949e; }
      .drawer-val { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11px; fill: #c9d1d9; }
      .btn-primary { fill: #238636; rx: 6px; }
      .btn-sec { fill: #21262d; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .btn-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 600; fill: #ffffff; text-anchor: middle; }
    </style>
    <filter id="win-shadow" x="-5%" y="-5%" width="110%" height="110%">
      <feDropShadow dx="0" dy="8" stdDeviation="16" flood-color="#000000" flood-opacity="0.55"/>
    </filter>
    <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 9 5 L 0 9 z" fill="#30363d"/>
    </marker>
    <marker id="arrow-active" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 9 5 L 0 9 z" fill="#58a6ff"/>
    </marker>
  </defs>

  <!-- Browser Window Frame -->
  <rect x="4" y="4" width="1032" height="712" class="browser-bg" filter="url(#win-shadow)" stroke="#30363d" stroke-width="1"/>
  <path d="M 4 16 A 12 12 0 0 1 16 4 L 1024 4 A 12 12 0 0 1 1036 16 L 1036 44 L 4 44 Z" class="browser-top"/>
  <line x1="4" y1="44" x2="1036" y2="44" stroke="#30363d" stroke-width="1"/>

  <!-- macOS Window Controls -->
  <circle cx="22" cy="24" r="5.5" class="dot-red"/>
  <circle cx="40" cy="24" r="5.5" class="dot-yellow"/>
  <circle cx="58" cy="24" r="5.5" class="dot-green"/>

  <!-- Browser URL Bar -->
  <rect x="240" y="10" width="560" height="26" class="url-bar"/>
  <text x="256" y="27" class="url-text"><tspan fill="#3fb950">🔒 </tspan><tspan class="url-host">http://127.0.0.1:8080</tspan>/studio/dag-visualizer</text>

  <!-- App Header -->
  <rect x="4" y="45" width="1032" height="48" class="app-header"/>
  <line x1="4" y1="93" x2="1036" y2="93" stroke="#30363d" stroke-width="1"/>

  <!-- Brand -->
  <text x="24" y="75" class="header-title"><tspan fill="#58a6ff">⚡</tspan> ZQK Knowledge Kernel Visual Studio</text>

  <!-- View Switcher Tabs -->
  <g transform="translate(320, 56)">
    <rect x="0" y="0" width="124" height="26" class="nav-tab-inactive"/>
    <text x="10" y="17" class="nav-text-inactive">📅 Timeline &amp; Gantt</text>

    <rect x="130" y="0" width="154" height="26" class="nav-tab-active"/>
    <text x="140" y="17" class="nav-text-active">🕸️ Ontology DAG Visualizer</text>

    <rect x="290" y="0" width="134" height="26" class="nav-tab-inactive"/>
    <text x="300" y="17" class="nav-text-inactive">🛡️ System Health &amp; DoD</text>

    <rect x="430" y="0" width="118" height="26" class="nav-tab-inactive"/>
    <text x="440" y="17" class="nav-text-inactive">📊 Metrics &amp; Trace</text>
  </g>

  <!-- Right App Controls -->
  <g transform="translate(890, 56)">
    <rect x="0" y="0" width="130" height="26" class="status-pill-bg"/>
    <circle cx="12" cy="13" r="3.5" fill="#3fb950"/>
    <text x="22" y="17" class="status-pill-text">● Live CAS (185 nodes)</text>
  </g>

  <!-- Main Viewport Grid: Left Canvas (660px) | Right Inspector (372px) -->
  <g transform="translate(4, 94)">
    <!-- Canvas Background -->
    <rect x="0" y="0" width="660" height="622" fill="#0d1117"/>
    <line x1="660" y1="0" x2="660" y2="622" stroke="#30363d" stroke-width="1"/>

    <!-- Graph Canvas Toolbar -->
    <g transform="translate(18, 16)">
      <rect x="0" y="0" width="144" height="30" fill="#161b22" stroke="#30363d" stroke-width="1" rx="6"/>
      <text x="12" y="20" font-family="-apple-system, sans-serif" font-size="12" fill="#c9d1d9">🔍 Zoom: 100% │ ☊ DAG</text>
    </g>

    <!-- Filter Chips -->
    <g transform="translate(240, 18)">
      <rect x="0" y="0" width="56" height="24" class="filter-chip-active"/>
      <text x="12" y="16" class="chip-text-active">All (185)</text>

      <rect x="62" y="0" width="62" height="24" class="filter-chip"/>
      <text x="72" y="16" class="chip-text">Goals (10)</text>

      <rect x="130" y="0" width="64" height="24" class="filter-chip"/>
      <text x="140" y="16" class="chip-text">Plans (85)</text>

      <rect x="200" y="0" width="94" height="24" class="filter-chip"/>
      <text x="210" y="16" class="chip-text">Backlog (196)</text>

      <rect x="300" y="0" width="102" height="24" class="filter-chip"/>
      <text x="310" y="16" class="chip-text">Test Cases (14)</text>
    </g>

    <!-- Connecting Edges (Smooth Bezier Splines) -->
    <!-- Edge 1: Goal -> Req 1 -->
    <path d="M 200 130 C 240 130, 240 130, 280 130" class="edge-active" marker-end="url(#arrow-active)"/>
    <!-- Edge 2: Goal -> Req 2 -->
    <path d="M 200 130 C 240 130, 240 260, 280 260" class="edge" marker-end="url(#arrow)"/>
    <!-- Edge 3: Req 1 -> BLI-001 -->
    <path d="M 460 130 C 490 130, 490 180, 520 180" class="edge-active" marker-end="url(#arrow-active)"/>
    <!-- Edge 4: BLI-001 -> Test Case -->
    <path d="M 640 180 C 650 180, 650 330, 520 330" class="edge-active" marker-end="url(#arrow-active)"/>
    <!-- Edge 5: Test Case -> Criteria -->
    <path d="M 520 330 C 470 330, 470 450, 420 450" class="edge-active" marker-end="url(#arrow-active)"/>

    <!-- DAG Node 1: GOAL-COMMUNITY-LAUNCH -->
    <g transform="translate(30, 95)">
      <rect width="170" height="70" class="node-card"/>
      <rect x="0" y="0" width="4" height="70" fill="#3fb950" rx="2"/>
      <text x="14" y="20" class="node-kind" fill="#3fb950">GOAL</text>
      <text x="156" y="20" class="node-status">active</text>
      <text x="14" y="38" class="node-id">GOAL-COMMUNITY</text>
      <text x="14" y="55" class="node-title">Open-Core Community Gate</text>
    </g>

    <!-- DAG Node 2: REQ-LAUNCH-DOCS -->
    <g transform="translate(280, 95)">
      <rect width="180" height="70" class="node-card"/>
      <rect x="0" y="0" width="4" height="70" fill="#d29922" rx="2"/>
      <text x="14" y="20" class="node-kind" fill="#d29922">REQUIREMENT</text>
      <text x="166" y="20" class="node-status">active</text>
      <text x="14" y="38" class="node-id">REQ-LAUNCH-DOCS</text>
      <text x="14" y="55" class="node-title">100% Documentation Accuracy</text>
    </g>

    <!-- DAG Node 3: REQ-STORAGE-PUREGO -->
    <g transform="translate(280, 225)">
      <rect width="180" height="70" class="node-card"/>
      <rect x="0" y="0" width="4" height="70" fill="#d29922" rx="2"/>
      <text x="14" y="20" class="node-kind" fill="#d29922">REQUIREMENT</text>
      <text x="166" y="20" class="node-status">complete</text>
      <text x="14" y="38" class="node-id">REQ-STORAGE-PUREGO</text>
      <text x="14" y="55" class="node-title">Pure-Go Storage Engine</text>
    </g>

    <!-- DAG Node 4: BLI-COMMUNITY-FIRST-RUN (Selected Node) -->
    <g transform="translate(480, 145)">
      <rect width="170" height="70" class="node-card-selected"/>
      <rect x="0" y="0" width="4" height="70" fill="#39c5bb" rx="2"/>
      <text x="14" y="20" class="node-kind" fill="#39c5bb">BACKLOG ITEM</text>
      <text x="156" y="20" class="node-status" fill="#58a6ff">planned</text>
      <text x="14" y="38" class="node-id">BLI-FIRST-RUN</text>
      <text x="14" y="55" class="node-title">First-run tutorial walkthrough</text>
    </g>

    <!-- DAG Node 5: TST-COMMUNITY-FIRST-RUN -->
    <g transform="translate(360, 295)">
      <rect width="175" height="70" class="node-card"/>
      <rect x="0" y="0" width="4" height="70" fill="#db61a2" rx="2"/>
      <text x="14" y="20" class="node-kind" fill="#db61a2">TEST CASE</text>
      <text x="161" y="20" class="node-status">active</text>
      <text x="14" y="38" class="node-id">TST-FIRST-RUN</text>
      <text x="14" y="55" class="node-title">Verify Start-Here Tutorial</text>
    </g>

    <!-- DAG Node 6: CRIT-COMMUNITY-TUTORIAL -->
    <g transform="translate(180, 415)">
      <rect width="180" height="70" class="node-card"/>
      <rect x="0" y="0" width="4" height="70" fill="#3fb950" rx="2"/>
      <text x="14" y="20" class="node-kind" fill="#3fb950">CRITERIA</text>
      <text x="166" y="20" class="node-status">complete</text>
      <text x="14" y="38" class="node-id">CRIT-TUTORIAL</text>
      <text x="14" y="55" class="node-title">Criteria satisfied by test</text>
    </g>

    <!-- Focus Halo Label -->
    <g transform="translate(180, 550)">
      <rect x="0" y="0" width="310" height="32" fill="#161b22" stroke="#58a6ff" stroke-width="1" rx="16"/>
      <text x="20" y="20" font-family="-apple-system, sans-serif" font-size="12" fill="#f0f6fc">🔍 Focus Chain: <tspan fill="#3fb950">Goal</tspan> ➔ <tspan fill="#d29922">Req</tspan> ➔ <tspan fill="#39c5bb">BLI</tspan> ➔ <tspan fill="#db61a2">Test</tspan> ➔ <tspan fill="#3fb950">Crit</tspan></text>
    </g>

    <!-- Right Side: Object Inspector Drawer (372px) -->
    <g transform="translate(660, 0)">
      <rect x="0" y="0" width="372" height="622" class="drawer-bg"/>

      <!-- Drawer Header -->
      <g transform="translate(20, 24)">
        <text x="0" y="0" class="drawer-title">🔍 OBJECT INSPECTOR</text>
        <text x="320" y="0" font-family="-apple-system, sans-serif" font-size="14" fill="#8b949e" cursor="pointer">✕</text>
      </g>
      <line x1="0" y1="42" x2="372" y2="42" stroke="#30363d" stroke-width="1"/>

      <!-- Selected Entity Hero Card -->
      <g transform="translate(20, 60)">
        <rect x="0" y="0" width="332" height="96" fill="#0d1117" stroke="#30363d" stroke-width="1" rx="8"/>
        <rect x="0" y="0" width="4" height="96" fill="#39c5bb" rx="2"/>
        <text x="16" y="24" class="node-kind" fill="#39c5bb">BACKLOG ITEM</text>
        <rect x="236" y="10" width="80" height="20" fill="rgba(88, 166, 255, 0.15)" stroke="#58a6ff" stroke-width="1" rx="10"/>
        <text x="276" y="24" font-family="-apple-system, sans-serif" font-size="10" font-weight="600" fill="#58a6ff" text-anchor="middle">planned</text>
        <text x="16" y="48" font-family="ui-monospace, monospace" font-size="13" font-weight="700" fill="#f0f6fc">BLI-COMMUNITY-FIRST-RUN</text>
        <text x="16" y="70" class="drawer-val">Complete Start Here tutorial walkthrough</text>
      </g>

      <!-- Inspector Property Fields -->
      <g transform="translate(20, 180)">
        <text x="0" y="0" class="drawer-label">CAS HASH DIGEST</text>
        <text x="0" y="18" class="drawer-val">sha256:e3b0c44298fc1c149afbf4c8...</text>

        <text x="0" y="48" class="drawer-label">STORAGE PLANE</text>
        <text x="0" y="66" class="drawer-val" fill="#3fb950">PlanePromoted (CAS Master)</text>

        <text x="0" y="96" class="drawer-label">AUTHOR / SEATED ACTOR</text>
        <text x="0" y="114" class="drawer-val">PER-DEFAULT-LEAD (verified)</text>

        <text x="0" y="144" class="drawer-label">TRACEABILITY RADAR</text>
        <text x="0" y="162" class="drawer-val">Upstream Goal: [🟢 GOAL-COMMUNITY]</text>
        <text x="0" y="180" class="drawer-val">Requirement  : [🟢 REQ-LAUNCH-DOCS]</text>
        <text x="0" y="198" class="drawer-val">Test Target  : [🟢 TST-FIRST-RUN]</text>
        <text x="0" y="216" class="drawer-val">Acceptance   : [🟢 CRIT-TUTORIAL] (Satisfied)</text>
      </g>

      <line x1="20" y1="440" x2="352" y2="440" stroke="#30363d" stroke-width="1"/>

      <!-- Action Buttons Palette -->
      <g transform="translate(20, 460)">
        <rect x="0" y="0" width="332" height="34" class="btn-primary"/>
        <text x="166" y="22" class="btn-text">` + html.EscapeString(paths.RewriteCanonicalCLIInvocations("🚀 Promote Object (zqk object promote)")) + `</text>

        <rect x="0" y="44" width="160" height="32" class="btn-sec"/>
        <text x="80" y="64" class="btn-text" fill="#c9d1d9">🔗 Add Reference</text>

        <rect x="172" y="44" width="160" height="32" class="btn-sec"/>
        <text x="252" y="64" class="btn-text" fill="#c9d1d9">📜 Raw CAS JSON</text>
      </g>
    </g>
  </g>
</svg>
`)

	svgContent := sb.String()
	var dump any
	if err := xml.Unmarshal([]byte(svgContent), &dump); err != nil {
		return fmt.Errorf("XML validation failed for Web Studio: %w", err)
	}

	return os.WriteFile(filename, []byte(svgContent), paths.FilePerm644)
}

func main() {
	color.NoColor = false
	outDir := "docs/manual/screenshots"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	_ = os.MkdirAll(outDir, paths.DirPerm755)

	// 1. Tab 1: State
	{
		m := ui.NewUIModel(".", "state")
		m.Width = 100
		m.Height = 24
		m.AutoScroll = true
		m.Mutations = []state.JournalMutation{
			{CreatedAt: time.Now().Add(-180 * time.Second).Unix(), ChangeType: "PROMOTE", ObjectRef: "BLI-COMMUNITY-FIRST-RUN", DiffSummary: "Promoted draft to CAS master (hash: e3b0c442)"},
			{CreatedAt: time.Now().Add(-120 * time.Second).Unix(), ChangeType: "UPDATE", ObjectRef: "REQ-DATA-PLANE-004", DiffSummary: "Linked requirement_ref to CRIT-019"},
			{CreatedAt: time.Now().Add(-90 * time.Second).Unix(), ChangeType: "CREATE", ObjectRef: "QUE-199-FIRST-RUN", DiffSummary: "Minted new object on PlaneDraft"},
			{CreatedAt: time.Now().Add(-45 * time.Second).Unix(), ChangeType: "LATCH", ObjectRef: "CRIT-STORAGE-PUREGO", DiffSummary: "Latch satisfied: TestPureGoIndex passed"},
			{CreatedAt: time.Now().Add(-15 * time.Second).Unix(), ChangeType: "TRANSITION", ObjectRef: "PRI-LAUNCH-READINESS", DiffSummary: "Status transition planned -> in_progress"},
		}
		m.SelectedIndex = 4
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab1_state.svg"), "zqk ui (Tab 1: ⚡ State) — Real-Time State Seismograph & Mutation WAL", out, 100); err != nil {
			fmt.Printf("Error rendering Tab 1: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab1_state.svg")
		}
	}

	// 2. Tab 2: Audit
	{
		m := ui.NewUIModel(".", "audit")
		m.Width = 100
		m.Height = 24
		m.AutoScroll = true
		m.AuditEvents = []state.JournalMutation{
			{CreatedAt: time.Now().Add(-240 * time.Second).Unix(), Actor: "PER-DEFAULT-LEAD", ChangeType: "claim_work", ObjectRef: "BLI-STARTER-001", DiffSummary: "Claimed item for execution loop"},
			{CreatedAt: time.Now().Add(-180 * time.Second).Unix(), Actor: "agent-alpha", ChangeType: "ref_add", ObjectRef: "BLI-STARTER-001", DiffSummary: "Linked target REQ-LAUNCH-DOCS"},
			{CreatedAt: time.Now().Add(-120 * time.Second).Unix(), Actor: "agent-alpha", ChangeType: "promote", ObjectRef: "BLI-STARTER-001", DiffSummary: "Transition planned -> in_progress"},
			{CreatedAt: time.Now().Add(-60 * time.Second).Unix(), Actor: "ACC-SYSTEM", ChangeType: "scheduler_tick", ObjectRef: "SCH-RETENTION", DiffSummary: "Scanned 185 objects; pruned 0 stale"},
			{CreatedAt: time.Now().Add(-10 * time.Second).Unix(), Actor: "ACC-SYSTEM", ChangeType: "cas_verify", ObjectRef: "CAS-BLOB-9821", DiffSummary: "Verified SHA-256 integrity match"},
		}
		m.SelectedIndex = 4
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab2_audit.svg"), "zqk ui (Tab 2: 📜 Audit) — Cryptographic Provenance & Operational Audit Trail", out, 100); err != nil {
			fmt.Printf("Error rendering Tab 2: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab2_audit.svg")
		}
	}

	// 3. Tab 3: Swarm
	{
		m := ui.NewUIModel(".", "swarm")
		m.Width = 100
		m.Height = 24
		m.DaemonHealth = []ui.DaemonHealthRow{
			{Name: "ambient", DesiredState: "enabled", ActualState: "running", PID: 84912, RestartCount: 0, Uptime: "4h12m", Status: "HEALTHY"},
			{Name: "privileged-writer", DesiredState: "enabled", ActualState: "running", PID: 84915, RestartCount: 0, Uptime: "4h12m", Status: "HEALTHY"},
			{Name: "scheduler", DesiredState: "enabled", ActualState: "running", PID: 84920, RestartCount: 0, Uptime: "4h12m", Status: "HEALTHY"},
			{Name: "steward", DesiredState: "enabled", ActualState: "running", PID: 84925, RestartCount: 0, Uptime: "4h12m", Status: "HEALTHY"},
			{Name: "seat-worker-peer-1", DesiredState: "enabled", ActualState: "running", PID: 84930, RestartCount: 0, Uptime: "1h45m", Status: "HEALTHY"},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab3_swarm.svg"), "zqk ui (Tab 3: 🤖 Swarm) — Multi-Agent Swarm Topology & Seating", out, 100); err != nil {
			fmt.Printf("Error rendering Tab 3: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab3_swarm.svg")
		}
	}

	// 4. Tab 4: PM
	{
		m := ui.NewUIModel(".", "pm")
		m.Width = 100
		m.Height = 25
		m.RecentBacklog = []ui.PMBacklogRow{
			{ID: "BLI-COMMUNITY-FIRST-RUN", Title: "Community first-run tutorial verification", Status: "in_progress", Priority: "P0", ClaimedBy: "agent-alpha", PlanRef: "PRI-STARTER-COMMUNITY-001"},
			{ID: "BLI-STORAGE-PUREGO-001", Title: "Pure-Go embedded storage backend & WAL engine", Status: "complete", Priority: "P0", ClaimedBy: "ACC-SYSTEM", PlanRef: "PRI-STORAGE-PUREGO-001"},
			{ID: "BLI-ZQL-ACID-TRANSACT", Title: "Multi-object atomic transactional mutations", Status: "complete", Priority: "P1", ClaimedBy: "ACC-SYSTEM", PlanRef: "PRI-STORAGE-PUREGO-001"},
			{ID: "BLI-AIRGAP-BUILD-TARBALL", Title: "Zero-network hermetic build verification", Status: "planned", Priority: "P1", ClaimedBy: "", PlanRef: "PRI-STARTER-COMMUNITY-001"},
		}
		m.PriorityPlans = []ui.PMPlanRow{
			{ID: "PRI-STARTER-COMMUNITY-001", Title: "Community Launch Gate & Starter Experience", Status: "in_progress", Workstreams: []string{"WS-LAUNCH"}, BLICount: 3},
			{ID: "PRI-STORAGE-PUREGO-001", Title: "Storage Engine Pure-Go Modernization", Status: "complete", Workstreams: []string{"WS-KERNEL"}, BLICount: 2},
		}
		m.TechnicalDebt = []ui.PMDebtRow{
			{ID: "DEBT-001", Title: "Purge legacy object update references", Status: "resolved", Priority: "high", Category: "hygiene"},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab4_pm.svg"), "zqk ui (Tab 4: 📋 PM) — Technical Program Management & Shovel-Ready Backlog", out, 100); err != nil {
			fmt.Printf("Error rendering Tab 4: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab4_pm.svg")
		}
	}

	// 5. Tab 5: Metrics
	{
		m := ui.NewUIModel(".", "metrics")
		m.Width = 100
		m.Height = 24
		m.CommandMetrics = []ui.CommandMetricRow{
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk system check"), ExecCount: 142, AvgDuration: "42ms", LastRunAt: "12:15:02", Status: "pass"},
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk object list"), ExecCount: 389, AvgDuration: "18ms", LastRunAt: "12:15:10", Status: "pass"},
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk test run"), ExecCount: 64, AvgDuration: "124ms", LastRunAt: "12:14:30", Status: "pass"},
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk workflow whats-next"), ExecCount: 95, AvgDuration: "28ms", LastRunAt: "12:14:55", Status: "pass"},
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk scheduler trigger"), ExecCount: 28, AvgDuration: "8ms", LastRunAt: "12:13:40", Status: "pass"},
		}
		m.LockMetrics = []ui.FileLockMetricRow{
			{ID: "1", TargetKind: ".zqk/process/CAS", Contention: 0, Duration: "2ms", Status: "healthy"},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab5_metrics.svg"), "zqk ui (Tab 5: 📊 Metrics) — Kernel Latency & Execution Performance Telemetry", out, 100); err != nil {
			fmt.Printf("Error rendering Tab 5: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab5_metrics.svg")
		}
	}

	// 6. Tab 6: Scheduler
	{
		m := ui.NewUIModel(".", "sched")
		m.Width = 100
		m.Height = 24
		m.SchedulerJobs = []ui.SchedulerJobRow{
			{ID: "SCH-001", Title: "change_journal_compaction", Schedule: "@every 5m", LastRunAt: "12:10:00", NextRunAt: "12:15:00", Status: "active"},
			{ID: "SCH-002", Title: "audit_aggregation", Schedule: "@every 1h", LastRunAt: "12:00:00", NextRunAt: "13:00:00", Status: "active"},
			{ID: "SCH-003", Title: "cas_hygiene_scan", Schedule: "@every 6h", LastRunAt: "06:00:00", NextRunAt: "12:00:00", Status: "active"},
			{ID: "SCH-004", Title: "memory_leak_watchdog", Schedule: "@every 1m", LastRunAt: "12:14:00", NextRunAt: "12:15:00", Status: "active"},
			{ID: "SCH-005", Title: "test_matrix_prewarm", Schedule: "@every 2m", LastRunAt: "12:13:30", NextRunAt: "12:15:30", Status: "active"},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab6_scheduler.svg"), "zqk ui (Tab 6: ⏱️ Sched) — Autonomous Background Daemons & Maintenance Jobs", out, 100); err != nil {
			fmt.Printf("Error rendering Tab 6: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab6_scheduler.svg")
		}
	}

	// 7. Tab 7: QA
	{
		m := ui.NewUIModel(".", "qa")
		m.Width = 100
		m.Height = 24
		m.TestCases = []*test.TestCaseModel{
			{ID: "TST-STORAGE-PUREGO-001", Title: "Verify Pure-Go Indexing Engine Performance", Status: "complete", TotalCriteria: 1, CompletedCriteria: 1, Lineage: &test.LineageChain{IsIntact: true}},
			{ID: "TST-ZQL-ACID-TRANSACT-01", Title: "Verify Multi-Object Atomic Rollbacks", Status: "complete", TotalCriteria: 3, CompletedCriteria: 3, Lineage: &test.LineageChain{IsIntact: true}},
			{ID: "TST-COMMUNITY-FIRST-RUN", Title: "Community first-run end-to-end verification", Status: "active", TotalCriteria: 1, CompletedCriteria: 1, Lineage: &test.LineageChain{IsIntact: true}},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab7_qa.svg"), "zqk ui (Tab 7: 🧪 QA) — Definition of Done & Traceability Verification Radar", out, 100); err != nil {
			fmt.Printf("Error rendering Tab 7: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab7_qa.svg")
		}
	}

	// 8. Tab 8: Health
	{
		m := ui.NewUIModel(".", "health")
		m.Width = 100
		m.Height = 24
		m.HealthSummary = ui.HealthSummary{
			LastChecked:     time.Now().Add(-2 * time.Minute),
			CheckFreshness:  "FRESH (2m ago)",
			OverallStatus:   "HEALTHY",
			TotalViolations: 0,
			StaleLocksCount: 0,
			StorageFiles:    26920,
			StorageSizeStr:  "200MiB",
			OpenFileDesc:    10,
			MaxFileDesc:     245760,
			DaemonsRunning:  4,
			DaemonsTotal:    4,
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab8_health.svg"), "zqk ui (Tab 8: 🛡️ Health) — 4-Layer Compliance Cake & CAS Storage Health", out, 100); err != nil {
			fmt.Printf("Error rendering Tab 8: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab8_health.svg")
		}
	}

	// 9. Object Inspector (7-Panel Detail Modal rendered via actual tds.Panel & ui.Render)
	{
		m := ui.NewUIModel(".", "pm")
		m.Width = 100
		m.Height = 28
		m.DetailModal = &ui.ItemDetailModel{
			Kind:      "backlog_item",
			ID:        "BLI-COMMUNITY-FIRST-RUN",
			Status:    "validated",
			Title:     "Execute Start-Here Onboarding Walkthrough & Verify Downward Traceability",
			Actor:     "PER-DEFAULT-LEAD (signature: e3b0c442)",
			Timestamp: "2026-09-29T12:00:00Z",
			Summary:   "Grounded in verified acceptance criteria and validated against Definition of Done.\nReady for canonical lifecycle promotion (PlaneDraft -> PlanePromoted).",
			Details: []string{
				"Category: Developer Experience │ Priority: P0 (Critical)",
				"CAS Digest: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"Active Membrane: Mode B (Strict CAS Verification)",
			},
			Lineage: []string{
				"Upstream Goal       : [🟢 GOAL-STARTER-COMMUNITY-001] Open-Core Community Gate",
				"Parent Requirement  : [🟢 REQ-COMMUNITY-FIRST-RUN] 100% Documentation Accuracy",
				"Bound Test Case     : [🟢 TST-COMMUNITY-FIRST-RUN] pkg/community/onboarding_test.go",
				"Lineage Integrity   : ✓ 100% INTACT (Zero orphan references, zero cycles)",
			},
			Criteria: []string{
				"[🟢 CRIT-COMMUNITY-001] (satisfied) Complete Start Here walkthrough succeeds without manual interventions",
				"[🟢 CRIT-COMMUNITY-002] (satisfied) All 4 layers in system check pass with 0 warnings",
				"[🟢 CRIT-COMMUNITY-003] (satisfied) Documentation portal compiles with 188 verified articles",
			},
		}
		out := ui.Render(m)
		inspectorTitle := paths.RewriteCanonicalCLIInvocations("zqk object inspect") + " — 7-Panel Interactive Object Inspector Console"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_object_inspector.svg"), inspectorTitle, out, 100); err != nil {
			fmt.Printf("Error rendering Object Inspector: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_object_inspector.svg")
		}
	}

	// 10. Test Verification Dashboard
	{
		var buf strings.Builder
		cyanBold := color.New(color.FgCyan, color.Bold).SprintFunc()
		greenBold := color.New(color.FgGreen, color.Bold).SprintFunc()
		yellowBold := color.New(color.FgYellow, color.Bold).SprintFunc()
		dimStyle := color.New(color.Faint).SprintFunc()

		buf.WriteString(cyanBold("====================================================================================================") + "\n")
		buf.WriteString(cyanBold("🚀 ZQK TEST & DEFINITION OF DONE (DoD) DASHBOARD | [ACTIVE WORKING SET]") + "\n")
		buf.WriteString(cyanBold("====================================================================================================") + "\n")
		buf.WriteString(fmt.Sprintf("Working Set: 3 In-Flight Tests │ Regression Pool: 160 Verified Chains (Green) │ DoD: %s\n\n", greenBold("100% PASS")))

		buf.WriteString(fmt.Sprintf("▶ %s  %s  %s\n", cyanBold("TST-STORAGE-PUREGO-001"), yellowBold("[ACTIVE]"), "Verify Pure-Go Indexing Engine Performance"))
		buf.WriteString("    Lineage  : [🟢 GOAL-STORAGE] ➔ [🟢 REQ-STORAGE] ➔ [🟢 BLI-STORAGE] ➔ [🟡 TST-STORAGE-001]  " + greenBold("✓ Chain Intact") + "\n")
		buf.WriteString("    Target   : pkg/storage/purego_index_test.go:TestPureGoIndex (integration)\n")
		buf.WriteString("    Criteria : [🟢 CRIT-STORAGE-PUREGO-EMBEDDED-001] (satisfied)\n")
		buf.WriteString("    Progress : [" + greenBold("████████████████████") + "] 100% (1/1 satisfied, 0 open)\n\n")

		buf.WriteString(fmt.Sprintf("▶ %s  %s  %s\n", cyanBold("TST-ZQL-ACID-TRANSACT-01"), yellowBold("[ACTIVE]"), "Verify Multi-Object Atomic Rollbacks"))
		buf.WriteString("    Lineage  : [🟢 GOAL-ZQL] ➔ [🟢 REQ-ZQL-ACID] ➔ [🟢 BLI-ZQL] ➔ [🟡 TST-ZQL-ACID-01]  " + greenBold("✓ Chain Intact") + "\n")
		buf.WriteString("    Target   : pkg/zql/transaction_test.go:TestAtomicRollback (unit)\n")
		buf.WriteString("    Criteria : [🟢 CRIT-ZQL-STAGED-ISOLATION] [🟢 CRIT-ZQL-ROLLBACK-JOURNAL] (satisfied)\n")
		buf.WriteString("    Progress : [" + greenBold("████████████████████") + "] 100% (2/2 satisfied, 0 open)\n\n")

		buf.WriteString(greenBold("🛡️  REGRESSION TESTING POOL: 160 verified chains green & passing") + "\n")
		buf.WriteString(dimStyle(paths.RewriteCanonicalCLIInvocations("[Pruned from active view — inspect full regression suite with: zqk test dashboard --view regression]")) + "\n\n")

		buf.WriteString(dimStyle(strings.Repeat("─", 100)) + "\n")
		buf.WriteString("📡 RECENT CRITERIA SATISFACTION & SHOCKWAVE EVENTS\n")
		buf.WriteString(dimStyle(strings.Repeat("─", 100)) + "\n")
		buf.WriteString("  ⚡ [12:14:01] CRITERION SATISFIED: CRIT-STORAGE-PUREGO-EMBEDDED-001\n")
		buf.WriteString("  ⚡ [12:14:01] TEST CASE TST-STORAGE-PUREGO-001: active -> complete\n")
		buf.WriteString("  ⚡ [12:14:01] TRACEABILITY CHAIN GRADUATED: TST-STORAGE-PUREGO-001 ➔ Moved to Regression Pool\n")
		buf.WriteString("  ⚡ [12:14:02] SHOCKWAVE PROPAGATED: Latch complete on requirement REQ-STORAGE-PUREGO\n")
		buf.WriteString(dimStyle(strings.Repeat("─", 100)) + "\n")

		testDashTitle := paths.RewriteCanonicalCLIInvocations("zqk test dashboard") + " — Definition of Done & Live Test Verification Matrix"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_test_dashboard.svg"), testDashTitle, buf.String(), 100); err != nil {
			fmt.Printf("Error rendering Test Dashboard: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_test_dashboard.svg")
		}
	}

	// 11. Visual Web Studio (Authentic Web Application UI matching pkg/studio/ui_server.go)
	{
		if err := RenderWebStudioSVG(filepath.Join(outDir, "ui_web_studio.svg")); err != nil {
			fmt.Printf("Error rendering Web Studio: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_web_studio.svg")
		}
	}

	fmt.Println("🎉 All 11 SVG visual mockups successfully generated and validated!")
}
