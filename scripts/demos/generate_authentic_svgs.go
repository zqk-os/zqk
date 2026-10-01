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
	"github.com/zqk-os/zqk/pkg/paths"
	ui "github.com/zqk-os/zqk/pkg/tui"
	"github.com/zqk-os/zqk/pkg/tui/tds"
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

		isEmojiOrWide := rw > 1
		isBoxBorder := r == '│' || r == '║' || r == '╮' || r == '╯' || r == '┐' || r == '┘' || r == '╗' || r == '╝' || r == '╭' || r == '╰' || r == '┌' || r == '└' || r == '╔' || r == '╚' || r == '├' || r == '┤' || r == '┬' || r == '┴' || r == '┼'

		if !isEmojiOrWide && !isBoxBorder {
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
				if nrw > 1 || nr == '│' || nr == '║' || nr == '╮' || nr == '╯' || nr == '┐' || nr == '┘' || nr == '╗' || nr == '╝' || nr == '╭' || nr == '╰' || nr == '┌' || nr == '└' || nr == '╔' || nr == '╚' || nr == '├' || nr == '┤' || nr == '┬' || nr == '┴' || nr == '┼' {
					break
				}
				b.WriteRune(nr)
				col += nrw
				idx++
			}
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

	charWidth := 8.05
	lineHeight := 19.5
	padX := 24.0
	padY := 16.0
	topBarH := 40.0

	contentW := float64(widthCols) * charWidth
	totalW := int(contentW + padX*2 + 0.5)
	totalH := int(topBarH + padY*2 + float64(len(lines))*lineHeight + 10)

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
      .term-text { font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Monaco, Consolas, monospace;
                   font-size: 13px; fill: #cdd6f4; letter-spacing: 0px; white-space: pre; }
      .term-bg-reverse { fill: #89b4fa; rx: 3px; }
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
	sb.WriteString(`  <circle cx="22" cy="22" r="5.5" class="dot-red"/>
  <circle cx="40" cy="22" r="5.5" class="dot-yellow"/>
  <circle cx="58" cy="22" r="5.5" class="dot-green"/>
`)
	sb.WriteString(fmt.Sprintf(`  <text x="%f" y="26" class="title-text">%s</text>`+"\n",
		float64(totalW)/2.0, html.EscapeString(title)))

	sb.WriteString(fmt.Sprintf(`  <g transform="translate(%f, %f)">`+"\n", padX, topBarH+padY))

	for rowIdx, line := range lines {
		spans := parseANSILine(line)
		yPos := float64(rowIdx+1)*lineHeight - 4.5

		// Pass 1: Render background highlight pill for reverse-video spans (e.g. active tabs)
		type pillRect struct {
			startCol int
			endCol   int
		}
		var pills []pillRect
		var curPill *pillRect
		for _, sp := range spans {
			if sp.Reverse {
				w := runewidth.StringWidth(sp.Text)
				if curPill == nil {
					curPill = &pillRect{startCol: sp.Col, endCol: sp.Col + w}
				} else if sp.Col == curPill.endCol {
					curPill.endCol += w
				} else {
					pills = append(pills, *curPill)
					curPill = &pillRect{startCol: sp.Col, endCol: sp.Col + w}
				}
			} else {
				if curPill != nil {
					pills = append(pills, *curPill)
					curPill = nil
				}
			}
		}
		if curPill != nil {
			pills = append(pills, *curPill)
		}

		for _, p := range pills {
			pillX := float64(p.startCol)*charWidth - 1.5
			pillW := float64(p.endCol-p.startCol)*charWidth + 3.0
			sb.WriteString(fmt.Sprintf(`    <rect x="%.1f" y="%.1f" width="%.1f" height="17.5" rx="3" class="term-bg-reverse"/>`+"\n",
				pillX, yPos-13.0, pillW))
		}

		// Pass 2: Render character-grid aligned monospace spans
		sb.WriteString(fmt.Sprintf(`    <text y="%.1f" class="term-text" xml:space="preserve">`, yPos))
		for _, sp := range spans {
			var attrs []string
			spanX := float64(sp.Col) * charWidth
			attrs = append(attrs, fmt.Sprintf(`x="%.1f"`, spanX))

			fill := sp.Color
			if sp.Dim {
				fill = "#6c7086"
			}
			if sp.Reverse {
				fill = "#11111b"
			}
			attrs = append(attrs, fmt.Sprintf(`fill="%s"`, fill))

			if sp.Bold || sp.Reverse {
				attrs = append(attrs, `font-weight="bold"`)
			}

			escapedText := html.EscapeString(sp.Text)
			escapedText = strings.ReplaceAll(escapedText, "\x1b", "")

			sb.WriteString(fmt.Sprintf(`<tspan %s>%s</tspan>`, strings.Join(attrs, " "), escapedText))
		}
		sb.WriteString("</text>\n")
	}

	sb.WriteString(`  </g>
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
	width := 1100
	height := 700

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
      .view-switcher { fill: #0d1117; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .view-tab-active { fill: #21262d; rx: 4px; }
      .view-tab-text-active { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 600; fill: #58a6ff; }
      .view-tab-text-inactive { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 500; fill: #8b949e; }
      .ctrl-input { fill: #0d1117; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .ctrl-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #f0f6fc; font-weight: 500; }
      .ctrl-placeholder { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #8b949e; }
      .status-pill-bg { fill: rgba(63, 185, 80, 0.12); rx: 12px; }
      .status-pill-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 500; fill: #3fb950; }
      .btn-header { fill: #21262d; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .btn-header-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 500; fill: #f0f6fc; text-anchor: middle; }
      .canvas-panel { fill: #0d1117; }
      .toolbar-box { fill: rgba(22, 27, 34, 0.92); stroke: #30363d; stroke-width: 1; rx: 8px; }
      .toolbar-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; fill: #c9d1d9; font-weight: 500; }
      .filter-chip { fill: transparent; rx: 4px; }
      .filter-chip-active { fill: #58a6ff; rx: 4px; }
      .chip-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 500; fill: #8b949e; text-anchor: middle; }
      .chip-text-active { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 600; fill: #ffffff; text-anchor: middle; }
      .focus-banner { fill: rgba(33, 38, 45, 0.95); stroke: #58a6ff; stroke-width: 1; rx: 14px; }
      .focus-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #f0f6fc; }
      .btn-clear-focus { fill: #58a6ff; rx: 4px; }
      .btn-clear-focus-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 10px; font-weight: 600; fill: #ffffff; text-anchor: middle; }
      .node-bg { fill: #161b22; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .node-bg-selected { fill: #1c2128; stroke: #58a6ff; stroke-width: 2.5; rx: 6px; }
      .node-badge { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 9px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; }
      .node-status { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 9px; font-weight: 500; fill: #8b949e; text-anchor: end; }
      .node-text-id { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11px; font-weight: 600; fill: #f0f6fc; }
      .node-text-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #c9d1d9; }
      .edge { fill: none; stroke: #484f58; stroke-width: 1.5; }
      .edge-active { fill: none; stroke: #58a6ff; stroke-width: 2.2; }
      .sidebar-panel { fill: #161b22; }
      .tab-bar-bg { fill: #11151c; }
      .sidebar-tab-active { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 600; fill: #f0f6fc; text-anchor: middle; }
      .sidebar-tab-inactive { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 500; fill: #8b949e; text-anchor: middle; }
      .prop-header { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.5px; fill: #8b949e; }
      .prop-label { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 500; fill: #8b949e; }
      .prop-val { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11px; fill: #c9d1d9; }
      .ref-chip { fill: #21262d; stroke: #30363d; stroke-width: 1; rx: 4px; }
      .ref-chip-text { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 10px; font-weight: 500; fill: #58a6ff; }
      .btn-primary { fill: #238636; rx: 6px; }
      .btn-sec { fill: #21262d; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .btn-accent { fill: #58a6ff; rx: 6px; }
      .btn-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 600; fill: #ffffff; text-anchor: middle; }
    </style>
    <filter id="win-shadow" x="-5%" y="-5%" width="110%" height="110%">
      <feDropShadow dx="0" dy="8" stdDeviation="16" flood-color="#000000" flood-opacity="0.55"/>
    </filter>
    <filter id="node-glow" x="-20%" y="-20%" width="140%" height="140%">
      <feDropShadow dx="0" dy="0" stdDeviation="4" flood-color="#58a6ff" flood-opacity="0.4"/>
    </filter>
    <marker id="arrow" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 10 5 L 0 9 z" fill="#484f58"/>
    </marker>
    <marker id="arrow-active" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1 L 10 5 L 0 9 z" fill="#58a6ff"/>
    </marker>
  </defs>

  <!-- Browser Window Frame -->
  <rect x="4" y="4" width="1092" height="692" class="browser-bg" filter="url(#win-shadow)" stroke="#30363d" stroke-width="1"/>
  <path d="M 4 16 A 12 12 0 0 1 16 4 L 1084 4 A 12 12 0 0 1 1096 16 L 1096 42 L 4 42 Z" class="browser-top"/>
  <line x1="4" y1="42" x2="1096" y2="42" stroke="#30363d" stroke-width="1"/>

  <!-- macOS Window Controls -->
  <circle cx="22" cy="23" r="5.5" class="dot-red"/>
  <circle cx="40" cy="23" r="5.5" class="dot-yellow"/>
  <circle cx="58" cy="23" r="5.5" class="dot-green"/>

  <!-- Browser URL Bar -->
  <rect x="280" y="8" width="540" height="26" class="url-bar"/>
  <text x="296" y="25" class="url-text"><tspan fill="#3fb950">🔒 </tspan><tspan class="url-host">http://127.0.0.1:8080/</tspan></text>

  <!-- App Header -->
  <rect x="4" y="43" width="1092" height="48" class="app-header"/>
  <line x1="4" y1="91" x2="1096" y2="91" stroke="#30363d" stroke-width="1"/>

  <!-- Brand (Left) -->
  <text x="22" y="72" class="header-title"><tspan fill="#58a6ff">⚡</tspan> ZQK Knowledge Kernel Visual Studio</text>

  <!-- View Mode Switcher (Center-Left) -->
  <g transform="translate(340, 53)">
    <rect x="0" y="0" width="248" height="28" class="view-switcher"/>
    <rect x="2" y="2" width="104" height="24" class="view-tab-active"/>
    <text x="14" y="18" class="view-tab-text-active">☊ DAG Graph</text>
    <text x="120" y="18" class="view-tab-text-inactive">▤ Timeline &amp; Gantt</text>
  </g>

  <!-- Header Controls (Right) -->
  <g transform="translate(605, 53)">
    <!-- Workstream Dropdown -->
    <rect x="0" y="0" width="130" height="28" class="ctrl-input"/>
    <text x="10" y="18" class="ctrl-text">🌐 All Workstreams ▾</text>

    <!-- Density Filter Dropdown -->
    <rect x="138" y="0" width="154" height="28" class="ctrl-input"/>
    <text x="148" y="18" class="ctrl-text">Execution (+ BLIs) ▾</text>

    <!-- Global Search -->
    <rect x="300" y="0" width="78" height="28" class="ctrl-input"/>
    <text x="308" y="18" class="ctrl-placeholder">🔍 Search...</text>

    <!-- Live Status Pill -->
    <rect x="386" y="1" width="94" height="26" class="status-pill-bg"/>
    <circle cx="398" cy="14" r="3.5" fill="#3fb950"/>
    <text x="408" y="18" class="status-pill-text">Connected</text>
  </g>

  <!-- Main Viewport: Left Canvas (720px) | Right Sidebar (372px) -->
  <g transform="translate(4, 92)">
    <!-- DAG Graph Canvas -->
    <rect x="0" y="0" width="720" height="604" class="canvas-panel"/>
    <line x1="720" y1="0" x2="720" y2="604" stroke="#30363d" stroke-width="1"/>

    <!-- Canvas Toolbar (Top-Left: Zoom & Fit) -->
    <g transform="translate(16, 14)">
      <rect x="0" y="0" width="120" height="30" class="toolbar-box"/>
      <text x="12" y="20" class="toolbar-text">+   −   ⟲   ⛶</text>
    </g>

    <!-- Focus Subgraph Banner (Top-Center) -->
    <g transform="translate(150, 14)">
      <rect x="0" y="0" width="300" height="30" class="focus-banner"/>
      <text x="14" y="19" class="focus-text">🎯 Subgraph: <tspan fill="#58a6ff" font-weight="600">BLI-COMMUNITY-FIRST-RUN</tspan></text>
      <rect x="238" y="5" width="52" height="20" class="btn-clear-focus"/>
      <text x="264" y="18" class="btn-clear-focus-text">✕ All</text>
    </g>

    <!-- Kind Filter Chips (Top-Right) -->
    <g transform="translate(465, 14)">
      <rect x="0" y="0" width="242" height="30" class="toolbar-box"/>
      <rect x="4" y="4" width="32" height="22" class="filter-chip-active"/>
      <text x="20" y="19" class="chip-text-active">All</text>
      <text x="68" y="19" class="chip-text">WS (2)</text>
      <text x="116" y="19" class="chip-text">Goals (4)</text>
      <text x="166" y="19" class="chip-text">Plans (8)</text>
      <text x="214" y="19" class="chip-text">BLIs (16)</text>
    </g>

    <!-- Directed Graph Edges (Smooth Bezier Splines) -->
    <!-- Edge 1: WS -> Goal -->
    <path d="M 210 115 C 240 115, 240 115, 270 115" class="edge-active" marker-end="url(#arrow-active)"/>
    <!-- Edge 2: Goal -> Priority Plan -->
    <path d="M 460 115 C 490 115, 490 225, 270 225" class="edge-active" marker-end="url(#arrow-active)"/>
    <!-- Edge 3: Priority Plan -> Milestone -->
    <path d="M 460 225 C 490 225, 490 335, 270 335" class="edge-active" marker-end="url(#arrow-active)"/>
    <!-- Edge 4: Milestone -> BLI (Selected) -->
    <path d="M 460 335 C 490 335, 490 225, 510 225" class="edge-active" marker-end="url(#arrow-active)"/>
    <!-- Edge 5: BLI -> Test Case -->
    <path d="M 605 260 C 605 310, 605 360, 470 445" class="edge-active" marker-end="url(#arrow-active)"/>
    <!-- Edge 6: Test Case -> Criteria -->
    <path d="M 280 445 C 250 445, 250 445, 220 445" class="edge-active" marker-end="url(#arrow-active)"/>

    <!-- DAG Node 1: Workstream (kind color #39c5bb) -->
    <g transform="translate(25, 82)">
      <rect width="185" height="66" class="node-bg"/>
      <rect x="0" y="0" width="4" height="66" fill="#39c5bb" rx="2"/>
      <text x="14" y="18" class="node-badge" fill="#39c5bb">WORKSTREAM</text>
      <text x="173" y="18" class="node-status">active</text>
      <text x="14" y="36" class="node-text-id">WS-CORE-LAUNCH</text>
      <text x="14" y="52" class="node-text-title">Community Core Readiness</text>
    </g>

    <!-- DAG Node 2: Goal (kind color #a371f7) -->
    <g transform="translate(275, 82)">
      <rect width="185" height="66" class="node-bg"/>
      <rect x="0" y="0" width="4" height="66" fill="#a371f7" rx="2"/>
      <text x="14" y="18" class="node-badge" fill="#a371f7">GOAL</text>
      <text x="173" y="18" class="node-status">active</text>
      <text x="14" y="36" class="node-text-id">GOAL-COMMUNITY</text>
      <text x="14" y="52" class="node-text-title">Open-Core Community Gate</text>
    </g>

    <!-- DAG Node 3: Priority Plan (kind color #58a6ff) -->
    <g transform="translate(275, 192)">
      <rect width="185" height="66" class="node-bg"/>
      <rect x="0" y="0" width="4" height="66" fill="#58a6ff" rx="2"/>
      <text x="14" y="18" class="node-badge" fill="#58a6ff">PRIORITY PLAN</text>
      <text x="173" y="18" class="node-status">in_progress</text>
      <text x="14" y="36" class="node-text-id">PRI-LAUNCH-READY</text>
      <text x="14" y="52" class="node-text-title">Kernel Launch Readiness</text>
    </g>

    <!-- DAG Node 4: Milestone (kind color #d29922) -->
    <g transform="translate(275, 302)">
      <rect width="185" height="66" class="node-bg"/>
      <rect x="0" y="0" width="4" height="66" fill="#d29922" rx="2"/>
      <text x="14" y="18" class="node-badge" fill="#d29922">MILESTONE</text>
      <text x="173" y="18" class="node-status">active</text>
      <text x="14" y="36" class="node-text-id">MLS-M1-DOCS</text>
      <text x="14" y="52" class="node-text-title">100% Verified Manual &amp; Specs</text>
    </g>

    <!-- DAG Node 5: Backlog Item (SELECTED / FOCUSED) (kind color #3fb950) -->
    <g transform="translate(510, 192)" filter="url(#node-glow)">
      <rect width="195" height="66" class="node-bg-selected"/>
      <rect x="0" y="0" width="4" height="66" fill="#3fb950" rx="2"/>
      <text x="14" y="18" class="node-badge" fill="#3fb950">BACKLOG ITEM</text>
      <text x="181" y="18" class="node-status" fill="#58a6ff">planned</text>
      <text x="14" y="36" class="node-text-id">BLI-COMMUNITY-FIRST-RUN</text>
      <text x="14" y="52" class="node-text-title">First-run tutorial walkthrough</text>
    </g>

    <!-- DAG Node 6: Test Case (kind color #db61a2) -->
    <g transform="translate(285, 412)">
      <rect width="185" height="66" class="node-bg"/>
      <rect x="0" y="0" width="4" height="66" fill="#db61a2" rx="2"/>
      <text x="14" y="18" class="node-badge" fill="#db61a2">TEST CASE</text>
      <text x="173" y="18" class="node-status">active</text>
      <text x="14" y="36" class="node-text-id">TST-FIRST-RUN</text>
      <text x="14" y="52" class="node-text-title">Verify Start-Here DoD</text>
    </g>

    <!-- DAG Node 7: Criteria (kind color #3fb950) -->
    <g transform="translate(35, 412)">
      <rect width="185" height="66" class="node-bg"/>
      <rect x="0" y="0" width="4" height="66" fill="#3fb950" rx="2"/>
      <text x="14" y="18" class="node-badge" fill="#3fb950">CRITERIA</text>
      <text x="173" y="18" class="node-status">complete</text>
      <text x="14" y="36" class="node-text-id">CRIT-COMMUNITY-001</text>
      <text x="14" y="52" class="node-text-title">Zero-defect DoD satisfaction</text>
    </g>

    <!-- Right Sidebar Panel: Inspector & Objects -->
    <g transform="translate(720, 0)">
      <rect x="0" y="0" width="372" height="604" class="sidebar-panel"/>

      <!-- Tab Bar -->
      <rect x="0" y="0" width="372" height="38" class="tab-bar-bg"/>
      <line x1="0" y1="38" x2="372" y2="38" stroke="#30363d" stroke-width="1"/>
      <text x="92" y="24" class="sidebar-tab-active">Inspector</text>
      <line x1="16" y1="37" x2="168" y2="37" stroke="#58a6ff" stroke-width="2"/>
      <text x="270" y="24" class="sidebar-tab-inactive">Kernel Objects (185)</text>

      <!-- Inspector Content Body -->
      <g transform="translate(18, 52)">
        <!-- Top Row: Kind Pill & Status -->
        <rect x="0" y="0" width="94" height="20" fill="rgba(63, 185, 80, 0.15)" stroke="#3fb950" stroke-width="1" rx="10"/>
        <text x="47" y="14" font-family="-apple-system, sans-serif" font-size="9" font-weight="600" fill="#3fb950" text-anchor="middle">BACKLOG ITEM</text>
        <rect x="272" y="0" width="64" height="20" fill="rgba(88, 166, 255, 0.15)" stroke="#58a6ff" stroke-width="1" rx="10"/>
        <text x="304" y="14" font-family="-apple-system, sans-serif" font-size="9" font-weight="600" fill="#58a6ff" text-anchor="middle">planned</text>

        <!-- ID Row + Copy Button -->
        <text x="0" y="42" font-family="ui-monospace, monospace" font-size="13" font-weight="700" fill="#f0f6fc">BLI-COMMUNITY-FIRST-RUN</text>
        <rect x="286" y="28" width="50" height="20" class="btn-sec"/>
        <text x="311" y="42" class="btn-text" fill="#c9d1d9">Copy</text>

        <!-- Title -->
        <text x="0" y="66" font-family="-apple-system, sans-serif" font-size="12" fill="#c9d1d9">Complete Start Here tutorial walkthrough</text>

        <!-- Focus Subgraph Action Button -->
        <rect x="0" y="82" width="138" height="26" class="btn-accent"/>
        <text x="69" y="99" class="btn-text">🎯 Focus Subgraph</text>

        <line x1="0" y1="122" x2="336" y2="122" stroke="#30363d" stroke-width="1"/>

        <!-- Properties Section -->
        <text x="0" y="142" class="prop-header">PROPERTIES</text>
        <text x="0" y="162" class="prop-label">KIND</text>
        <text x="120" y="162" class="prop-val">backlog_item</text>
        <text x="0" y="184" class="prop-label">STATUS</text>
        <text x="120" y="184" class="prop-val" fill="#58a6ff">planned</text>
        <text x="0" y="206" class="prop-label">CREATED</text>
        <text x="120" y="206" class="prop-val">2026-09-29T12:00:00Z</text>
        <text x="0" y="228" class="prop-label">PLAN REF</text>
        <text x="120" y="228" class="prop-val">PRI-LAUNCH-READY</text>

        <line x1="0" y1="248" x2="336" y2="248" stroke="#30363d" stroke-width="1"/>

        <!-- Lineage & Traceability Section -->
        <text x="0" y="268" class="prop-header">DOWNWARD / UPSTREAM LINEAGE &amp; DOD</text>
        <text x="0" y="288" class="prop-label">Upstream Milestone</text>
        <g transform="translate(0, 296)">
          <rect x="0" y="0" width="132" height="20" class="ref-chip"/>
          <text x="8" y="14" class="ref-chip-text">◆ MLS-M1-DOCS</text>
        </g>

        <text x="0" y="336" class="prop-label">Bound Test Case</text>
        <g transform="translate(0, 344)">
          <rect x="0" y="0" width="156" height="20" class="ref-chip"/>
          <text x="8" y="14" class="ref-chip-text">🧪 TST-FIRST-RUN</text>
        </g>

        <text x="0" y="384" class="prop-label">DoD Acceptance Criteria</text>
        <g transform="translate(0, 392)">
          <rect x="0" y="0" width="220" height="20" class="ref-chip"/>
          <text x="8" y="14" class="ref-chip-text">✓ CRIT-COMMUNITY-001 (Satisfied)</text>
        </g>

        <line x1="0" y1="432" x2="336" y2="432" stroke="#30363d" stroke-width="1"/>

        <!-- Action Buttons Section -->
        <g transform="translate(0, 448)">
          <rect x="0" y="0" width="336" height="32" class="btn-primary"/>
          <text x="168" y="21" class="btn-text">` + html.EscapeString(paths.RewriteCanonicalCLIInvocations("🚀 Promote Object (zqk object promote)")) + `</text>

          <rect x="0" y="42" width="162" height="30" class="btn-sec"/>
          <text x="81" y="61" class="btn-text" fill="#c9d1d9">🔗 Add Reference</text>

          <rect x="174" y="42" width="162" height="30" class="btn-sec"/>
          <text x="255" y="61" class="btn-text" fill="#c9d1d9">📜 Raw CAS JSON</text>
        </g>
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

func RenderWebStudioGanttSVG(filename string) error {
	width := 1100
	height := 700

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
      .view-switcher { fill: #0d1117; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .view-tab-active { fill: #21262d; rx: 4px; }
      .view-tab-text-active { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 600; fill: #58a6ff; }
      .view-tab-text-inactive { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 500; fill: #8b949e; }
      .ctrl-input { fill: #0d1117; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .ctrl-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #f0f6fc; font-weight: 500; }
      .ctrl-placeholder { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #8b949e; }
      .status-pill-bg { fill: rgba(63, 185, 80, 0.12); rx: 12px; }
      .status-pill-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 500; fill: #3fb950; }
      .canvas-panel { fill: #0d1117; }
      .gantt-toolbar-label { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #8b949e; font-weight: 500; }
      .gantt-pill { fill: #161b22; stroke: #30363d; stroke-width: 1; rx: 4px; }
      .gantt-pill-active { fill: rgba(88, 166, 255, 0.15); stroke: #58a6ff; stroke-width: 1; rx: 4px; }
      .gantt-pill-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #8b949e; font-weight: 500; text-anchor: middle; }
      .gantt-pill-text-active { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #58a6ff; font-weight: 600; text-anchor: middle; }
      .legend-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 10px; fill: #8b949e; font-weight: 500; }
      .tick-header { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 600; fill: #8b949e; }
      .tick-date { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 10px; font-weight: 500; fill: #c9d1d9; text-anchor: middle; }
      .tick-sub { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 9px; fill: #8b949e; text-anchor: middle; }
      .sec-header-bg { fill: #161b22; }
      .sec-header-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 600; fill: #58a6ff; }
      .gantt-row-bg { fill: #0d1117; }
      .gantt-row-selected { fill: rgba(88, 166, 255, 0.08); }
      .row-id { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11px; font-weight: 600; fill: #f0f6fc; }
      .row-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #8b949e; }
      .bar-planned { fill: #30363d; rx: 4px; }
      .bar-active { fill: #1f6feb; rx: 4px; }
      .bar-done { fill: #238636; rx: 4px; }
      .bar-testing { fill: #8957e5; rx: 4px; }
      .bar-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 10px; font-weight: 600; fill: #ffffff; }
      .today-badge { fill: #f85149; rx: 3px; }
      .today-badge-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 9px; font-weight: 700; fill: #ffffff; text-anchor: middle; }
      .sidebar-panel { fill: #161b22; }
      .tab-bar-bg { fill: #11151c; }
      .sidebar-tab-active { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 600; fill: #f0f6fc; text-anchor: middle; }
      .sidebar-tab-inactive { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; font-weight: 500; fill: #8b949e; text-anchor: middle; }
      .prop-header { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.5px; fill: #8b949e; }
      .prop-label { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 500; fill: #8b949e; }
      .prop-val { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 11px; fill: #c9d1d9; }
      .ref-chip { fill: #21262d; stroke: #30363d; stroke-width: 1; rx: 4px; }
      .ref-chip-text { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 10px; font-weight: 500; fill: #58a6ff; }
      .btn-primary { fill: #238636; rx: 6px; }
      .btn-sec { fill: #21262d; stroke: #30363d; stroke-width: 1; rx: 6px; }
      .btn-accent { fill: #58a6ff; rx: 6px; }
      .btn-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; font-weight: 600; fill: #ffffff; text-anchor: middle; }
    </style>
    <filter id="win-shadow-gantt" x="-5%" y="-5%" width="110%" height="110%">
      <feDropShadow dx="0" dy="8" stdDeviation="16" flood-color="#000000" flood-opacity="0.55"/>
    </filter>
  </defs>

  <!-- Browser Window Frame -->
  <rect x="4" y="4" width="1092" height="692" class="browser-bg" filter="url(#win-shadow-gantt)" stroke="#30363d" stroke-width="1"/>
  <path d="M 4 16 A 12 12 0 0 1 16 4 L 1084 4 A 12 12 0 0 1 1096 16 L 1096 42 L 4 42 Z" class="browser-top"/>
  <line x1="4" y1="42" x2="1096" y2="42" stroke="#30363d" stroke-width="1"/>

  <!-- macOS Window Controls -->
  <circle cx="22" cy="23" r="5.5" class="dot-red"/>
  <circle cx="40" cy="23" r="5.5" class="dot-yellow"/>
  <circle cx="58" cy="23" r="5.5" class="dot-green"/>

  <!-- Browser URL Bar -->
  <rect x="280" y="8" width="540" height="26" class="url-bar"/>
  <text x="296" y="25" class="url-text"><tspan fill="#3fb950">🔒 </tspan><tspan class="url-host">http://127.0.0.1:8080/studio/gantt</tspan></text>

  <!-- App Header -->
  <rect x="4" y="43" width="1092" height="48" class="app-header"/>
  <line x1="4" y1="91" x2="1096" y2="91" stroke="#30363d" stroke-width="1"/>

  <!-- Brand (Left) -->
  <text x="22" y="72" class="header-title"><tspan fill="#58a6ff">⚡</tspan> ZQK Knowledge Kernel Visual Studio</text>

  <!-- View Mode Switcher (Center-Left) -->
  <g transform="translate(340, 53)">
    <rect x="0" y="0" width="248" height="28" class="view-switcher"/>
    <text x="24" y="18" class="view-tab-text-inactive">☊ DAG Graph</text>
    <rect x="116" y="2" width="130" height="24" class="view-tab-active"/>
    <text x="128" y="18" class="view-tab-text-active">▤ Timeline &amp; Gantt</text>
  </g>

  <!-- Header Controls (Right) -->
  <g transform="translate(605, 53)">
    <rect x="0" y="0" width="130" height="28" class="ctrl-input"/>
    <text x="10" y="18" class="ctrl-text">🌐 All Workstreams ▾</text>

    <rect x="138" y="0" width="154" height="28" class="ctrl-input"/>
    <text x="148" y="18" class="ctrl-text">Execution (+ BLIs) ▾</text>

    <rect x="300" y="0" width="78" height="28" class="ctrl-input"/>
    <text x="308" y="18" class="ctrl-placeholder">🔍 Search...</text>

    <rect x="386" y="1" width="94" height="26" class="status-pill-bg"/>
    <circle cx="398" cy="14" r="3.5" fill="#3fb950"/>
    <text x="408" y="18" class="status-pill-text">Connected</text>
  </g>

  <!-- Main Viewport: Left Gantt Canvas (720px) | Right Sidebar (372px) -->
  <g transform="translate(4, 92)">
    <!-- Canvas Panel -->
    <rect x="0" y="0" width="720" height="604" class="canvas-panel"/>
    <line x1="720" y1="0" x2="720" y2="604" stroke="#30363d" stroke-width="1"/>

    <!-- Gantt Sub-Toolbar (y=10) -->
    <g transform="translate(16, 10)">
      <text x="0" y="18" class="gantt-toolbar-label">Status:</text>
      <rect x="42" y="2" width="38" height="22" class="gantt-pill-active"/>
      <text x="61" y="17" class="gantt-pill-text-active">All</text>
      <rect x="86" y="2" width="78" height="22" class="gantt-pill"/>
      <text x="125" y="17" class="gantt-pill-text">In Progress</text>
      <rect x="170" y="2" width="60" height="22" class="gantt-pill"/>
      <text x="200" y="17" class="gantt-pill-text">Planned</text>
      <rect x="236" y="2" width="72" height="22" class="gantt-pill"/>
      <text x="272" y="17" class="gantt-pill-text">Completed</text>

      <text x="330" y="18" class="gantt-toolbar-label">Grouping:</text>
      <rect x="388" y="2" width="110" height="22" class="gantt-pill"/>
      <text x="398" y="17" class="gantt-pill-text" style="text-anchor: start;">Workstream ▾</text>

      <text x="640" y="18" font-family="-apple-system, sans-serif" font-size="11" fill="#8b949e" font-weight="500">8 items</text>
    </g>

    <!-- Gantt Legend Bar (y=40) -->
    <g transform="translate(16, 40)">
      <text x="0" y="14" class="gantt-toolbar-label">Legend:</text>
      <text x="56" y="14" font-family="-apple-system, sans-serif" font-size="11" fill="#f0883e">◆</text>
      <text x="70" y="14" class="legend-text">Milestone</text>

      <text x="136" y="14" font-family="-apple-system, sans-serif" font-size="11" fill="#58a6ff">🌐</text>
      <text x="152" y="14" class="legend-text">Workstream</text>

      <text x="234" y="14" font-family="-apple-system, sans-serif" font-size="11" fill="#58a6ff">▶</text>
      <text x="248" y="14" class="legend-text">In Progress</text>

      <text x="326" y="14" font-family="-apple-system, sans-serif" font-size="11" fill="#8b949e">⏳</text>
      <text x="340" y="14" class="legend-text">Planned</text>

      <text x="402" y="14" font-family="-apple-system, sans-serif" font-size="11" fill="#3fb950">✓</text>
      <text x="416" y="14" class="legend-text">Completed</text>

      <line x1="492" y1="5" x2="492" y2="17" stroke="#f85149" stroke-width="1.5" stroke-dasharray="2,2"/>
      <text x="502" y="14" class="legend-text" fill="#f85149" font-weight="600">Today Line</text>
    </g>

    <line x1="0" y1="64" x2="720" y2="64" stroke="#30363d" stroke-width="1"/>

    <!-- Timeline Header Row: Tasks vs Date Columns (y=65 to y=100) -->
    <g transform="translate(0, 65)">
      <rect x="0" y="0" width="720" height="36" fill="#161b22"/>
      <line x1="0" y1="36" x2="720" y2="36" stroke="#30363d" stroke-width="1"/>
      <text x="16" y="22" class="tick-header">TASK / ENTITY</text>
      <line x1="240" y1="0" x2="240" y2="36" stroke="#30363d" stroke-width="1"/>

      <!-- 7 Date Ticks: Sep 26 to Oct 02 (each 68px) -->
      <!-- Day 0: Sep 26 Sat -->
      <g transform="translate(240, 0)">
        <text x="34" y="16" class="tick-date">Sep 26</text>
        <text x="34" y="28" class="tick-sub">Sat</text>
        <line x1="68" y1="0" x2="68" y2="36" stroke="#21262d" stroke-width="1"/>
      </g>
      <!-- Day 1: Sep 27 Sun -->
      <g transform="translate(308, 0)">
        <text x="34" y="16" class="tick-date">Sep 27</text>
        <text x="34" y="28" class="tick-sub">Sun</text>
        <line x1="68" y1="0" x2="68" y2="36" stroke="#21262d" stroke-width="1"/>
      </g>
      <!-- Day 2: Sep 28 Mon -->
      <g transform="translate(376, 0)">
        <text x="34" y="16" class="tick-date">Sep 28</text>
        <text x="34" y="28" class="tick-sub">Mon</text>
        <line x1="68" y1="0" x2="68" y2="36" stroke="#21262d" stroke-width="1"/>
      </g>
      <!-- Day 3: Sep 29 Tue (TODAY) -->
      <g transform="translate(444, 0)">
        <rect x="14" y="3" width="40" height="12" class="today-badge"/>
        <text x="34" y="12" class="today-badge-text">TODAY</text>
        <text x="34" y="23" class="tick-date" fill="#f85149" font-weight="700">Sep 29</text>
        <text x="34" y="32" class="tick-sub" fill="#f85149">Tue</text>
        <line x1="68" y1="0" x2="68" y2="36" stroke="#21262d" stroke-width="1"/>
      </g>
      <!-- Day 4: Sep 30 Wed -->
      <g transform="translate(512, 0)">
        <text x="34" y="16" class="tick-date">Sep 30</text>
        <text x="34" y="28" class="tick-sub">Wed</text>
        <line x1="68" y1="0" x2="68" y2="36" stroke="#21262d" stroke-width="1"/>
      </g>
      <!-- Day 5: Oct 01 Thu -->
      <g transform="translate(580, 0)">
        <text x="34" y="16" class="tick-date">Oct 01</text>
        <text x="34" y="28" class="tick-sub">Thu</text>
        <line x1="68" y1="0" x2="68" y2="36" stroke="#21262d" stroke-width="1"/>
      </g>
      <!-- Day 6: Oct 02 Fri -->
      <g transform="translate(648, 0)">
        <text x="34" y="16" class="tick-date">Oct 02</text>
        <text x="34" y="28" class="tick-sub">Fri</text>
      </g>
    </g>

    <!-- Timeline Body & Grid (y=101 to y=604) -->
    <g transform="translate(0, 101)">
      <!-- Vertical Grid Lines -->
      <line x1="240" y1="0" x2="240" y2="503" stroke="#21262d" stroke-width="1"/>
      <line x1="308" y1="0" x2="308" y2="503" stroke="#21262d" stroke-width="1"/>
      <line x1="376" y1="0" x2="376" y2="503" stroke="#21262d" stroke-width="1"/>
      <line x1="444" y1="0" x2="444" y2="503" stroke="#21262d" stroke-width="1"/>
      <line x1="512" y1="0" x2="512" y2="503" stroke="#21262d" stroke-width="1"/>
      <line x1="580" y1="0" x2="580" y2="503" stroke="#21262d" stroke-width="1"/>
      <line x1="648" y1="0" x2="648" y2="503" stroke="#21262d" stroke-width="1"/>

      <!-- Vertical Today Line (Dropping down at x=478, center of Sep 29) -->
      <line x1="478" y1="0" x2="478" y2="503" stroke="#f85149" stroke-width="1.5" stroke-dasharray="3,3"/>

      <!-- SECTION 1: WS-CORE-LAUNCH Swimlane Header -->
      <rect x="0" y="0" width="720" height="26" class="sec-header-bg"/>
      <line x1="0" y1="26" x2="720" y2="26" stroke="#30363d" stroke-width="1"/>
      <text x="16" y="17" class="sec-header-text">🌐 WS-CORE-LAUNCH: Community Core Readiness <tspan fill="#8b949e" font-weight="400">(4 items)</tspan></text>

      <!-- Row 1: Milestone MIL-COMMUNITY-LAUNCH -->
      <g transform="translate(0, 26)">
        <rect x="0" y="0" width="720" height="42" class="gantt-row-bg"/>
        <line x1="0" y1="42" x2="720" y2="42" stroke="#21262d" stroke-width="1"/>
        <text x="16" y="19" class="row-id"><tspan fill="#f0883e">◆</tspan> MIL-COMMUNITY-LAUNCH</text>
        <text x="16" y="34" class="row-title">Community Launch Milestone Gate</text>

        <!-- Diamond Marker at Today (x=478) -->
        <polygon points="478,13 486,21 478,29 470,21" fill="#f0883e"/>
        <text x="494" y="24" font-family="-apple-system, sans-serif" font-size="10" font-weight="600" fill="#f0883e">M1 Ship Gate</text>
      </g>

      <!-- Row 2: PRI-STARTER-COMMUNITY-001 (Priority Plan - Selected!) -->
      <g transform="translate(0, 68)">
        <rect x="0" y="0" width="720" height="42" class="gantt-row-selected"/>
        <line x1="0" y1="42" x2="720" y2="42" stroke="#21262d" stroke-width="1"/>
        <rect x="0" y="0" width="3" height="42" fill="#58a6ff"/>
        <text x="16" y="19" class="row-id">PRI-STARTER-COMMUNITY-001</text>
        <text x="16" y="34" class="row-title">Community launch testing and vetting</text>

        <!-- Gantt Bar: Active spanning Sep 27 to Sep 30 (x=308 to x=512) -->
        <rect x="320" y="11" width="190" height="20" class="bar-active"/>
        <text x="330" y="25" class="bar-text">▶ 75% complete (3/4 BLIs)</text>
      </g>

      <!-- Row 3: BLI-COMMUNITY-FIRST-RUN -->
      <g transform="translate(0, 110)">
        <rect x="0" y="0" width="720" height="42" class="gantt-row-bg"/>
        <line x1="0" y1="42" x2="720" y2="42" stroke="#21262d" stroke-width="1"/>
        <text x="28" y="19" class="row-id" fill="#c9d1d9">└─ BLI-COMMUNITY-FIRST-RUN</text>
        <text x="46" y="34" class="row-title">Start Here tutorial walkthrough verification</text>

        <!-- Gantt Bar: Completed spanning Sep 27 to Sep 29 (x=320 to x=478) -->
        <rect x="320" y="12" width="158" height="18" class="bar-done"/>
        <text x="330" y="25" class="bar-text">✓ complete</text>
      </g>

      <!-- Row 4: BLI-LAUNCH-DOCS -->
      <g transform="translate(0, 152)">
        <rect x="0" y="0" width="720" height="42" class="gantt-row-bg"/>
        <line x1="0" y1="42" x2="720" y2="42" stroke="#21262d" stroke-width="1"/>
        <text x="28" y="19" class="row-id" fill="#c9d1d9">└─ BLI-LAUNCH-DOCS</text>
        <text x="46" y="34" class="row-title">Visual guide &amp; UI dashboard exposition</text>

        <!-- Gantt Bar: In Progress spanning Sep 28 to Sep 30 (x=376 to x=512) -->
        <rect x="380" y="12" width="130" height="18" class="bar-active"/>
        <text x="390" y="25" class="bar-text">▶ in_progress</text>
      </g>

      <!-- SECTION 2: WS-STORAGE Swimlane Header -->
      <g transform="translate(0, 194)">
        <rect x="0" y="0" width="720" height="26" class="sec-header-bg"/>
        <line x1="0" y1="26" x2="720" y2="26" stroke="#30363d" stroke-width="1"/>
        <text x="16" y="17" class="sec-header-text">🌐 WS-STORAGE: Pure-Go CAS Subsystem <tspan fill="#8b949e" font-weight="400">(3 items)</tspan></text>
      </g>

      <!-- Row 5: Milestone MIL-PUREGO-STORAGE -->
      <g transform="translate(0, 220)">
        <rect x="0" y="0" width="720" height="42" class="gantt-row-bg"/>
        <line x1="0" y1="42" x2="720" y2="42" stroke="#21262d" stroke-width="1"/>
        <text x="16" y="19" class="row-id"><tspan fill="#f0883e">◆</tspan> MIL-PUREGO-STORAGE</text>
        <text x="16" y="34" class="row-title">Pure-Go Storage Engine Graduation</text>

        <!-- Diamond Marker at Sep 28 (x=410) -->
        <polygon points="410,13 418,21 410,29 402,21" fill="#f0883e"/>
        <text x="426" y="24" font-family="-apple-system, sans-serif" font-size="10" font-weight="600" fill="#3fb950">✓ Achieved</text>
      </g>

      <!-- Row 6: PRI-STORAGE-PUREGO-001 -->
      <g transform="translate(0, 262)">
        <rect x="0" y="0" width="720" height="42" class="gantt-row-bg"/>
        <line x1="0" y1="42" x2="720" y2="42" stroke="#21262d" stroke-width="1"/>
        <text x="16" y="19" class="row-id">PRI-STORAGE-PUREGO-001</text>
        <text x="16" y="34" class="row-title">Implement pure-Go CAS storage backend</text>

        <!-- Gantt Bar: Complete spanning Sep 26 to Sep 28 (x=250 to x=410) -->
        <rect x="250" y="11" width="160" height="20" class="bar-done"/>
        <text x="260" y="25" class="bar-text">✓ complete (100% DoD)</text>
      </g>

      <!-- Row 7: BLI-STORAGE-001 -->
      <g transform="translate(0, 304)">
        <rect x="0" y="0" width="720" height="42" class="gantt-row-bg"/>
        <line x1="0" y1="42" x2="720" y2="42" stroke="#21262d" stroke-width="1"/>
        <text x="28" y="19" class="row-id" fill="#c9d1d9">└─ BLI-STORAGE-001</text>
        <text x="46" y="34" class="row-title">CAS blob store concurrency &amp; WAL integrity</text>

        <rect x="250" y="12" width="140" height="18" class="bar-done"/>
        <text x="260" y="25" class="bar-text">✓ complete</text>
      </g>
    </g>

    <!-- Right Sidebar Panel: Inspector & Causal Lineage -->
    <g transform="translate(720, 0)">
      <rect x="0" y="0" width="372" height="604" class="sidebar-panel"/>

      <!-- Tab Bar -->
      <rect x="0" y="0" width="372" height="38" class="tab-bar-bg"/>
      <line x1="0" y1="38" x2="372" y2="38" stroke="#30363d" stroke-width="1"/>
      <text x="92" y="24" class="sidebar-tab-active">Gantt Inspector</text>
      <line x1="16" y1="37" x2="168" y2="37" stroke="#58a6ff" stroke-width="2"/>
      <text x="270" y="24" class="sidebar-tab-inactive">Roadmap Schedule</text>

      <!-- Inspector Content Body -->
      <g transform="translate(18, 52)">
        <!-- Top Row: Kind Pill & Status -->
        <rect x="0" y="0" width="104" height="20" fill="rgba(88, 166, 255, 0.15)" stroke="#58a6ff" stroke-width="1" rx="10"/>
        <text x="52" y="14" font-family="-apple-system, sans-serif" font-size="9" font-weight="600" fill="#58a6ff" text-anchor="middle">PRIORITY PLAN</text>
        <rect x="262" y="0" width="74" height="20" fill="rgba(88, 166, 255, 0.15)" stroke="#58a6ff" stroke-width="1" rx="10"/>
        <text x="299" y="14" font-family="-apple-system, sans-serif" font-size="9" font-weight="600" fill="#58a6ff" text-anchor="middle">in_progress</text>

        <!-- ID Row + Copy Button -->
        <text x="0" y="42" font-family="ui-monospace, monospace" font-size="12" font-weight="700" fill="#f0f6fc">PRI-STARTER-COMMUNITY-001</text>
        <rect x="286" y="28" width="50" height="20" class="btn-sec"/>
        <text x="311" y="42" class="btn-text" fill="#c9d1d9">Copy</text>

        <!-- Title -->
        <text x="0" y="66" font-family="-apple-system, sans-serif" font-size="12" fill="#c9d1d9">Community launch testing and vetting</text>

        <!-- Schedule Duration Pill -->
        <rect x="0" y="82" width="220" height="26" class="ctrl-input"/>
        <text x="12" y="99" font-family="-apple-system, sans-serif" font-size="11" fill="#58a6ff">⏱ Sep 27 ➔ Sep 30 (4 days)</text>

        <line x1="0" y1="122" x2="336" y2="122" stroke="#30363d" stroke-width="1"/>

        <!-- Properties Section -->
        <text x="0" y="142" class="prop-header">GANTT &amp; TPM PROPERTIES</text>
        <text x="0" y="162" class="prop-label">WORKSTREAM</text>
        <text x="120" y="162" class="prop-val">WS-CORE-LAUNCH</text>
        <text x="0" y="184" class="prop-label">LEAD MILESTONE</text>
        <text x="120" y="184" class="prop-val" fill="#f0883e">◆ MIL-COMMUNITY</text>
        <text x="0" y="206" class="prop-label">RUNWAY STATUS</text>
        <text x="120" y="206" class="prop-val" fill="#3fb950">Active Shovel-Ready</text>
        <text x="0" y="228" class="prop-label">DOD PROGRESS</text>
        <text x="120" y="228" class="prop-val">75% (3/4 Done)</text>

        <line x1="0" y1="248" x2="336" y2="248" stroke="#30363d" stroke-width="1"/>

        <!-- Downward Backlog Items -->
        <text x="0" y="268" class="prop-header">DOWNWARD BACKLOG EXECUTION CHAIN</text>
        <text x="0" y="288" class="prop-label">Completed Items</text>
        <g transform="translate(0, 296)">
          <rect x="0" y="0" width="220" height="20" class="ref-chip"/>
          <text x="8" y="14" class="ref-chip-text">✓ BLI-COMMUNITY-FIRST-RUN</text>
        </g>
        <g transform="translate(0, 320)">
          <rect x="0" y="0" width="220" height="20" class="ref-chip"/>
          <text x="8" y="14" class="ref-chip-text">✓ BLI-STORAGE-001 (CAS Master)</text>
        </g>

        <text x="0" y="360" class="prop-label">Active / In-Flight Item</text>
        <g transform="translate(0, 368)">
          <rect x="0" y="0" width="220" height="20" class="ref-chip" fill="rgba(88, 166, 255, 0.15)" stroke="#58a6ff"/>
          <text x="8" y="14" class="ref-chip-text" fill="#58a6ff">▶ BLI-LAUNCH-DOCS</text>
        </g>

        <line x1="0" y1="406" x2="336" y2="406" stroke="#30363d" stroke-width="1"/>

        <!-- Action Buttons Section -->
        <g transform="translate(0, 420)">
          <rect x="0" y="0" width="336" height="32" class="btn-primary"/>
          <text x="168" y="21" class="btn-text">` + html.EscapeString(paths.RewriteCanonicalCLIInvocations("🚀 Transition Plan (zqk object promote)")) + `</text>

          <rect x="0" y="42" width="162" height="30" class="btn-sec"/>
          <text x="81" y="61" class="btn-text" fill="#c9d1d9">🔗 Link Milestone</text>

          <rect x="174" y="42" width="162" height="30" class="btn-sec"/>
          <text x="255" y="61" class="btn-text" fill="#c9d1d9">📜 Raw CAS JSON</text>
        </g>
      </g>
    </g>
  </g>
</svg>
`)

	svgContent := sb.String()
	var dump any
	if err := xml.Unmarshal([]byte(svgContent), &dump); err != nil {
		return fmt.Errorf("XML validation failed for Web Studio Gantt: %w", err)
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

	termWidth := 120

	// 0. Header & Navigation Bar
	{
		m := ui.NewUIModel(".", "state")
		m.Width = termWidth
		m.Height = 14
		m.DynamicMessage = "System operating normally — ambient telemetry stream active"
		m.IsSearching = true
		m.SearchBuffer = "auth"
		m.SearchQuery = "auth"
		out := ui.Render(m)
		lines := strings.Split(out, "\n")
		headerLines := lines
		if len(lines) > 9 {
			headerLines = lines[:9]
		}
		headerContent := strings.Join(headerLines, "\n")
		headerTitle := paths.RewriteCanonicalCLIInvocations("zqk ui") + " — Mission Control Header & Navigation Bar"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_header.svg"), headerTitle, headerContent, termWidth); err != nil {
			fmt.Printf("Error rendering Header: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_header.svg")
		}
	}

	// 1. Tab 1: State
	{
		m := ui.NewUIModel(".", "state")
		m.Width = termWidth
		m.Height = 0
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
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab1_state.svg"), "zqk ui (Tab 1: ⚡ State) — Real-Time State Seismograph & Mutation WAL", out, termWidth); err != nil {
			fmt.Printf("Error rendering Tab 1: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab1_state.svg")
		}
	}

	// 2. Tab 2: Audit
	{
		m := ui.NewUIModel(".", "audit")
		m.Width = termWidth
		m.Height = 0
		m.AutoScroll = true
		m.AuditEvents = []state.JournalMutation{
			{CreatedAt: time.Now().Add(-240 * time.Second).Unix(), Actor: "PER-DEFAULT-LEAD", ChangeType: "claim_work", ObjectRef: "BLI-STARTER-001", DiffSummary: "Claimed item for execution loop"},
			{CreatedAt: time.Now().Add(-180 * time.Second).Unix(), Actor: "PER-DEFAULT-OPERATOR", ChangeType: "ref_add", ObjectRef: "BLI-STARTER-001", DiffSummary: "Linked target REQ-STARTER-COMMUNITY-001"},
			{CreatedAt: time.Now().Add(-120 * time.Second).Unix(), Actor: "PER-DEFAULT-OPERATOR", ChangeType: "promote", ObjectRef: "BLI-STARTER-001", DiffSummary: "Transition planned -> in_progress"},
			{CreatedAt: time.Now().Add(-60 * time.Second).Unix(), Actor: "ACC-SYSTEM", ChangeType: "scheduler_tick", ObjectRef: "SCH-retention-tolerance", DiffSummary: "Scanned 185 objects; pruned 0 stale"},
			{CreatedAt: time.Now().Add(-10 * time.Second).Unix(), Actor: "ACC-SYSTEM", ChangeType: "cas_verify", ObjectRef: "CAS-BLOB-9821", DiffSummary: "Verified SHA-256 integrity match"},
		}
		m.SelectedIndex = 4
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab2_audit.svg"), "zqk ui (Tab 2: 📜 Audit) — Cryptographic Provenance & Operational Audit Trail", out, termWidth); err != nil {
			fmt.Printf("Error rendering Tab 2: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab2_audit.svg")
		}
	}

	// 3. Tab 3: Swarm
	{
		m := ui.NewUIModel(".", "swarm")
		m.Width = termWidth
		m.Height = 0
		m.SwarmData = map[string]any{
			"throughput_hint":              "executing",
			"active_priority_plans":        3,
			"executing_agent_tasks":        5,
			"agent_instructions_total":     12,
			"agent_instructions_by_status": map[string]any{"proposed": 2, "approved": 4, "executing": 5, "completed": 1},
			"persona_skill_bound":          map[string]any{"bound": 5, "unbound": 0, "total": 5},
			"cap_orchestrator_job":         map[string]any{"id": "SCH-cap-orchestrator", "present": true},
		}
		m.DaemonHealth = []ui.DaemonHealthRow{
			{Name: "ambient", DesiredState: "enabled", ActualState: "running", PID: 84912, RestartCount: 0, Uptime: "4h12m", Status: "HEALTHY"},
			{Name: "privileged-writer", DesiredState: "enabled", ActualState: "running", PID: 84915, RestartCount: 0, Uptime: "4h12m", Status: "HEALTHY"},
			{Name: "scheduler", DesiredState: "enabled", ActualState: "running", PID: 84920, RestartCount: 0, Uptime: "4h12m", Status: "HEALTHY"},
			{Name: "steward", DesiredState: "enabled", ActualState: "running", PID: 84925, RestartCount: 0, Uptime: "4h12m", Status: "HEALTHY"},
			{Name: "seat-worker-peer-1", DesiredState: "enabled", ActualState: "running", PID: 84930, RestartCount: 0, Uptime: "1h45m", Status: "HEALTHY"},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab3_swarm.svg"), "zqk ui (Tab 3: 🤖 Swarm) — Multi-Agent Swarm Topology & Seating", out, termWidth); err != nil {
			fmt.Printf("Error rendering Tab 3: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab3_swarm.svg")
		}
	}

	// 4. Tab 4: PM
	{
		m := ui.NewUIModel(".", "pm")
		m.Width = termWidth
		m.Height = 0
		m.MissionTitle = "Continuous Autonomous Development"
		m.BacklogSummary = ui.PMBacklogSummary{
			Total:      10,
			Planned:    4,
			InProgress: 3,
			Blocked:    0,
			Done:       3,
			Claimed:    3,
			Unclaimed:  7,
		}
		m.RecentBacklog = []ui.PMBacklogRow{
			{ID: "BLI-COMMUNITY-FIRST-RUN", Title: "Community first-run tutorial verification", Status: "in_progress", Priority: "P0", ClaimedBy: "PER-DEFAULT-OPERATOR", PlanRef: "PRI-STARTER-COMMUNITY-001"},
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
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab4_pm.svg"), "zqk ui (Tab 4: 📋 PM) — Technical Program Management & Shovel-Ready Backlog", out, termWidth); err != nil {
			fmt.Printf("Error rendering Tab 4: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab4_pm.svg")
		}
	}

	// 5. Tab 5: Metrics
	{
		m := ui.NewUIModel(".", "metrics")
		m.Width = termWidth
		m.Height = 0
		m.Hygiene = ui.ResourceHygieneRow{
			ProcessObjectCount: 385,
			KindCount:          24,
			StreamFileCount:    52,
			ActiveStreams:      6,
		}
		m.CommandMetrics = []ui.CommandMetricRow{
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk system check"), ExecCount: 142, AvgDuration: "42ms", LastRunAt: "12:15:02", Status: "pass"},
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk object list"), ExecCount: 389, AvgDuration: "18ms", LastRunAt: "12:15:10", Status: "pass"},
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk test run"), ExecCount: 64, AvgDuration: "124ms", LastRunAt: "12:14:30", Status: "pass"},
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk workflow whats-next"), ExecCount: 95, AvgDuration: "28ms", LastRunAt: "12:14:55", Status: "pass"},
			{CommandName: paths.RewriteCanonicalCLIInvocations("zqk scheduler trigger"), ExecCount: 28, AvgDuration: "8ms", LastRunAt: "12:13:40", Status: "pass"},
		}
		m.LockMetrics = []ui.FileLockMetricRow{
			{ID: "1", TargetKind: paths.ProcessDir + "/CAS", Contention: 0, Duration: "2ms", Status: "healthy"},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab5_metrics.svg"), "zqk ui (Tab 5: 📊 Metrics) — Kernel Latency & Execution Performance Telemetry", out, termWidth); err != nil {
			fmt.Printf("Error rendering Tab 5: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab5_metrics.svg")
		}
	}

	// 6. Tab 6: Scheduler
	{
		m := ui.NewUIModel(".", "sched")
		m.Width = termWidth
		m.Height = 0
		m.SchedulerJobs = []ui.SchedulerJobRow{
			{ID: "SCH-audit-event-aggregation", Title: "audit_event_aggregation", Schedule: "*/15 * * * *", LastRunAt: "12:10:00", NextRunAt: "12:15:00", Status: "active"},
			{ID: "SCH-cache-prewarm", Title: "cache_prewarm", Schedule: "*/10 * * * *", LastRunAt: "12:00:00", NextRunAt: "12:10:00", Status: "active"},
			{ID: "SCH-cap-orchestrator", Title: "cap_orchestrator", Schedule: "0 */10 * * * *", LastRunAt: "12:00:00", NextRunAt: "12:10:00", Status: "active"},
			{ID: "SCH-retention-tolerance", Title: "retention_tolerance", Schedule: "0 */4 * * *", LastRunAt: "08:00:00", NextRunAt: "12:00:00", Status: "active"},
			{ID: "SCH-val", Title: "object_validation", Schedule: "event (WAL / mutate)", LastRunAt: "12:14:50", NextRunAt: "on-mutation", Status: "active"},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab6_scheduler.svg"), "zqk ui (Tab 6: ⏱️ Sched) — Autonomous Background Daemons & Maintenance Jobs", out, termWidth); err != nil {
			fmt.Printf("Error rendering Tab 6: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab6_scheduler.svg")
		}
	}

	// 7. Tab 7: QA
	{
		m := ui.NewUIModel(".", "qa")
		m.Width = termWidth
		m.Height = 0
		m.TestCases = []*test.TestCaseModel{
			{ID: "TST-STORAGE-PUREGO-001", Title: "Verify Pure-Go Indexing Engine Performance", Status: "complete", TotalCriteria: 1, CompletedCriteria: 1, Lineage: &test.LineageChain{IsIntact: true}},
			{ID: "TST-ZQL-ACID-TRANSACT-01", Title: "Verify Multi-Object Atomic Rollbacks", Status: "complete", TotalCriteria: 3, CompletedCriteria: 3, Lineage: &test.LineageChain{IsIntact: true}},
			{ID: "TST-COMMUNITY-FIRST-RUN", Title: "Community first-run end-to-end verification", Status: "active", TotalCriteria: 1, CompletedCriteria: 1, Lineage: &test.LineageChain{IsIntact: true}},
		}
		m.SelectedIndex = 0
		out := ui.Render(m)
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab7_qa.svg"), "zqk ui (Tab 7: 🧪 QA) — Definition of Done & Traceability Verification Radar", out, termWidth); err != nil {
			fmt.Printf("Error rendering Tab 7: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab7_qa.svg")
		}
	}

	// 8. Tab 8: Health
	{
		m := ui.NewUIModel(".", "health")
		m.Width = termWidth
		m.Height = 0
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
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_tab8_health.svg"), "zqk ui (Tab 8: 🛡️ Health) — 4-Layer Compliance Cake & CAS Storage Health", out, termWidth); err != nil {
			fmt.Printf("Error rendering Tab 8: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_tab8_health.svg")
		}
	}

	// 9. Object Inspector (7-Panel Detail Modal rendered via actual tds.Panel & ui.Render)
	{
		m := ui.NewUIModel(".", "pm")
		m.Width = termWidth
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
				"Parent Requirement  : [🟢 REQ-STARTER-COMMUNITY-001] Community kernel launch-readiness",
				"Bound Test Case     : [🟢 TST-COMMUNITY-FIRST-RUN] pkg/community/self_onboarding_test.go",
				"Lineage Integrity   : ✓ 100% INTACT (Zero orphan references, zero cycles)",
			},
			Criteria: []string{
				"[🟢 CRIT-STARTER-COMMUNITY-001] (satisfied) Complete Start Here walkthrough succeeds without manual interventions",
				"[🟢 CRIT-COMMUNITY-002] (satisfied) All 4 layers in system check pass with 0 warnings",
				"[🟢 CRIT-COMMUNITY-003] (satisfied) Documentation portal compiles with 188 verified articles",
			},
		}
		out := ui.Render(m)
		inspectorTitle := paths.RewriteCanonicalCLIInvocations("zqk object inspect") + " — 7-Panel Interactive Object Inspector Console"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_object_inspector.svg"), inspectorTitle, out, termWidth); err != nil {
			fmt.Printf("Error rendering Object Inspector: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_object_inspector.svg")
		}
	}

	// 9b. Object Inspector Main Scoreboard Table
	{
		var buf strings.Builder
		cyanBold := color.New(color.FgCyan, color.Bold).SprintFunc()
		greenBold := color.New(color.FgGreen, color.Bold).SprintFunc()
		yellowBold := color.New(color.FgYellow, color.Bold).SprintFunc()
		redBold := color.New(color.FgRed, color.Bold).SprintFunc()
		dimStyle := color.New(color.Faint).SprintFunc()
		whiteBold := color.New(color.FgWhite, color.Bold).SprintFunc()

		bannerText := "ZQK KNOWLEDGE KERNEL — OBJECT INSPECTOR CONSOLE"
		buf.WriteString(cyanBold("╔"+strings.Repeat("═", termWidth-2)+"╗") + "\n")
		buf.WriteString(cyanBold("║") + tds.PadCenter(whiteBold(bannerText), termWidth-2) + cyanBold("║") + "\n")
		buf.WriteString(cyanBold("╚"+strings.Repeat("═", termWidth-2)+"╝") + "\n")

		filterLines := []string{
			fmt.Sprintf("%s %s   │   %s %s %s %s %s %s   │   %s %s",
				cyanBold("KIND:"), whiteBold("[backlog_item]"),
				dimStyle("FILTER:"), whiteBold("[ALL]"), dimStyle("[ACTIVE]"), dimStyle("[DRAFT]"), dimStyle("[BLOCKED]"), dimStyle("[COMPLETE]"),
				cyanBold("SORT:"), yellowBold("[updated_at ▼]"),
			),
			fmt.Sprintf("🔍 %s %s %s", cyanBold("SEARCH:"), greenBold("[/cas█]"), dimStyle("(Press Enter to lock search, Esc to cancel)")),
		}
		buf.WriteString(tds.Panel("FILTER & SEARCH", filterLines, termWidth, tds.BorderLight))

		tbl := tds.NewTable(termWidth)
		tbl.AddColumn("ID", tds.AlignLeft, 26, 2.5)
		tbl.AddColumn("STATUS", tds.AlignLeft, 12, 1.0)
		tbl.AddColumn("PRI", tds.AlignCenter, 6, 0.5)
		tbl.AddColumn("TITLE", tds.AlignLeft, 48, 4.0)
		tbl.AddColumn("UPDATED", tds.AlignRight, 10, 1.0)

		tbl.AddRow(
			cyanBold("> ")+whiteBold("BLI-STORAGE-PUREGO-001"), greenBold("complete"), redBold("P0"), "Implement pure-Go CAS storage backend", dimStyle("2m ago"),
		)
		tbl.AddRow(
			"  "+whiteBold("BLI-STORAGE-PUREGO-002"), greenBold("complete"), yellowBold("P1"), "Wire change journal dictionary compaction", dimStyle("14m ago"),
		)
		tbl.AddRow(
			"  "+whiteBold("BLI-LAUNCH-DOCS-001"), cyanBold("in_progress"), redBold("P0"), "Comprehensive visual UI & mutation manual", dimStyle("1m ago"),
		)
		tbl.AddRow(
			"  "+whiteBold("BLI-ONBOARD-ROADMAP-01"), yellowBold("planned"), yellowBold("P1"), "Greenfield onboarding roadmap seed", dimStyle("45m ago"),
		)
		tbl.AddRow(
			"  "+whiteBold("BLI-ECOSYSTEM-SYNC-001"), redBold("blocked"), dimStyle("P2"), "Linear/GitHub bidirectional bridge", dimStyle("2h ago"),
		)
		buf.WriteString(tbl.Render())

		actionLines := []string{
			fmt.Sprintf("%s │ %s │ %s │ %s │ %s │ %s",
				cyanBold("[Enter] Deep Inspection Modal"),
				dimStyle("[Tab] Next Kind"),
				dimStyle("[f] Filter"),
				dimStyle("[s] Sort"),
				greenBold("[p] Policy Studio"),
				dimStyle("[q] Quit"),
			),
		}
		buf.WriteString(tds.Panel("KEYBOARD SHORTCUTS", actionLines, termWidth, tds.BorderLight))

		inspectorTableTitle := paths.RewriteCanonicalCLIInvocations("zqk object inspect") + " — Interactive Object Inspector Scoreboard"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_object_inspector_table.svg"), inspectorTableTitle, buf.String(), termWidth); err != nil {
			fmt.Printf("Error rendering Object Inspector Table: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_object_inspector_table.svg")
		}
	}

	// 9c. Policy Rule Studio: Step 1 (Creation & Autocomplete)
	{
		var buf strings.Builder
		cyanBold := color.New(color.FgCyan, color.Bold).SprintFunc()
		greenBold := color.New(color.FgGreen, color.Bold).SprintFunc()
		yellowBold := color.New(color.FgYellow, color.Bold).SprintFunc()
		redBold := color.New(color.FgRed, color.Bold).SprintFunc()
		dimStyle := color.New(color.Faint).SprintFunc()
		whiteBold := color.New(color.FgWhite, color.Bold).SprintFunc()

		bannerText := "ZQK POLICY RULE STUDIO — STEP 1: INTERACTIVE RULE CREATION & DSL DRAFTING"
		buf.WriteString(cyanBold("╔"+strings.Repeat("═", termWidth-2)+"╗") + "\n")
		buf.WriteString(cyanBold("║") + tds.PadCenter(whiteBold(bannerText), termWidth-2) + cyanBold("║") + "\n")
		buf.WriteString(cyanBold("╚"+strings.Repeat("═", termWidth-2)+"╝") + "\n")

		targetLines := []string{
			fmt.Sprintf("%s %s   │   %s %s %s   │   %s %s",
				cyanBold("RULE ID:"), whiteBold("[POL-MUTATION-002]"),
				cyanBold("TARGET KIND:"), greenBold("[backlog_item]"), dimStyle("(Tab to cycle)"),
				cyanBold("SEVERITY:"), redBold("[ERROR / REJECT]"),
			),
			fmt.Sprintf("%s %s",
				dimStyle("DESCRIPTION:"), dimStyle(`"Enforce claimant assignment and criteria DoD linkage before in_progress state transition"`),
			),
		}
		buf.WriteString(tds.Panel("1. TARGET SPECIFICATION & RULE METADATA", targetLines, termWidth, tds.BorderLight))

		subTop := "  " + dimStyle("┌─ SCHEMA SUGGESTIONS (FieldRegistry Discovery) "+strings.Repeat("─", 56)+"┐")
		subRow1 := "  " + dimStyle("│") + greenBold(" ▶ criteria_linked_or_acceptance_present()  ") + dimStyle("│") + yellowBold(" Built-in Predicate: true if DoD criteria is satisfied    ") + dimStyle("│")
		subRow2 := "  " + dimStyle("│") + dimStyle("   criteria_refs                            │ Attribute ([]string): registered criteria object IDs     │")
		subRow3 := "  " + dimStyle("│") + dimStyle("   claimed_by                               │ Attribute (string): current assignee agent/user identity │")
		subBottom := "  " + dimStyle("└"+strings.Repeat("─", 103)+"┘")

		dslLines := []string{
			fmt.Sprintf("%s %s%s",
				cyanBold("EXPRESSION:"),
				whiteBold(`status == "in_progress" ==> claimed_by != "" && criteria_linked`),
				yellowBold("█"),
			),
			subTop,
			subRow1,
			subRow2,
			subRow3,
			subBottom,
		}
		buf.WriteString(tds.Panel("2. DECLARATIVE RULE EXPRESSION & REAL-TIME AUTOCOMPLETE", dslLines, termWidth, tds.BorderLight))

		syntaxLines := []string{
			fmt.Sprintf("%s %s │ %s %s │ %s %s",
				greenBold("✓"), greenBold("ISO/IEC 14977 SYNTAX: OK"),
				cyanBold("TYPE SIGNATURE:"), whiteBold("(backlog_item) -> boolean"),
				greenBold("COMPLEXITY PROOF:"), dimStyle("O(1) bounded execution"),
			),
		}
		buf.WriteString(tds.Panel("3. SYNTAX VALIDATION & COMPILE-TIME TYPE RECEIPT", syntaxLines, termWidth, tds.BorderLight))

		actionLines := []string{
			fmt.Sprintf("%s │ %s │ %s │ %s │ %s",
				cyanBold("[Enter] Accept Suggestion"),
				dimStyle("[Tab] Next Match"),
				greenBold("[t] Trigger Dry-Run"),
				yellowBold("[s] Proceed to Save"),
				dimStyle("[Esc] Cancel"),
			),
		}
		buf.WriteString(tds.Panel("KEYBOARD SHORTCUTS", actionLines, termWidth, tds.BorderLight))

		createTitle := paths.RewriteCanonicalCLIInvocations("zqk object inspect --policy-studio") + " — Interactive Policy Rule Creation & Autocomplete"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_policy_studio_create.svg"), createTitle, buf.String(), termWidth); err != nil {
			fmt.Printf("Error rendering Policy Studio Create: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_policy_studio_create.svg")
		}
	}

	// 9d. Policy Rule Studio: Step 2 (Live Dry-Run Evaluation Matrix)
	{
		var buf strings.Builder
		cyanBold := color.New(color.FgCyan, color.Bold).SprintFunc()
		greenBold := color.New(color.FgGreen, color.Bold).SprintFunc()
		yellowBold := color.New(color.FgYellow, color.Bold).SprintFunc()
		redBold := color.New(color.FgRed, color.Bold).SprintFunc()
		dimStyle := color.New(color.Faint).SprintFunc()
		whiteBold := color.New(color.FgWhite, color.Bold).SprintFunc()

		bannerText := "ZQK POLICY RULE STUDIO — REAL-TIME GOVERNANCE DSL"
		buf.WriteString(cyanBold("╔"+strings.Repeat("═", termWidth-2)+"╗") + "\n")
		buf.WriteString(cyanBold("║") + tds.PadCenter(whiteBold(bannerText), termWidth-2) + cyanBold("║") + "\n")
		buf.WriteString(cyanBold("╚"+strings.Repeat("═", termWidth-2)+"╝") + "\n")

		configLines := []string{
			fmt.Sprintf("%s %s   │   %s %s   │   %s %s",
				cyanBold("TARGET KIND:"), whiteBold("[backlog_item]"),
				dimStyle("ACTIVE RULES:"), yellowBold("3 loaded"),
				cyanBold("EVALUATION MODE:"), greenBold("[DRY-RUN]"),
			),
		}
		buf.WriteString(tds.Panel("TARGET & CONFIGURATION", configLines, termWidth, tds.BorderLight))

		dslLines := []string{
			fmt.Sprintf("%s %s", cyanBold("EXPRESSION:"), whiteBold(`status == "in_progress" && claimed_by != ""`)),
			fmt.Sprintf("%s %s  %s  %s  %s  %s",
				dimStyle("AUTOCOMPLETE:"), greenBold("[claimed_by]"), dimStyle("priority_plan_ref"), dimStyle("effort_estimate"), dimStyle("milestone_refs"), dimStyle("description")),
		}
		buf.WriteString(tds.Panel("POLICY EXPRESSION DSL", dslLines, termWidth, tds.BorderLight))

		evalLines := []string{
			fmt.Sprintf("  %s %s", greenBold("✓"), greenBold("194 / 196 objects COMPLIANT (99.0%)")),
			fmt.Sprintf("  %s %s", redBold("✗"), redBold("2 objects VIOLATE RULE:")),
			fmt.Sprintf("    • %s: %s", yellowBold("BLI-AUTH-004"), dimStyle("status is 'in_progress' but 'claimed_by' is empty")),
			fmt.Sprintf("    • %s: %s", yellowBold("BLI-UI-012"), dimStyle("status is 'in_progress' but 'claimed_by' is empty")),
		}
		buf.WriteString(tds.Panel("EVALUATION RESULTS", evalLines, termWidth, tds.BorderLight))

		actionLines := []string{
			fmt.Sprintf("%s │ %s │ %s │ %s │ %s",
				cyanBold("[c] Edit Expression"),
				greenBold("[t] Trigger Dry-Run"),
				dimStyle("[Tab] Autocomplete"),
				yellowBold("[s] Save Rule"),
				dimStyle("[Esc] Return"),
			),
		}
		buf.WriteString(tds.Panel("KEYBOARD SHORTCUTS", actionLines, termWidth, tds.BorderLight))

		policyStudioTitle := paths.RewriteCanonicalCLIInvocations("zqk object inspect --policy-studio") + " — Real-Time Governance DSL Studio"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_policy_studio.svg"), policyStudioTitle, buf.String(), termWidth); err != nil {
			fmt.Printf("Error rendering Policy Studio: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_policy_studio.svg")
		}
	}

	// 9e. Policy Rule Studio: Step 3 (Atomic CAS Promotion Receipt)
	{
		var buf strings.Builder
		cyanBold := color.New(color.FgCyan, color.Bold).SprintFunc()
		greenBold := color.New(color.FgGreen, color.Bold).SprintFunc()
		yellowBold := color.New(color.FgYellow, color.Bold).SprintFunc()
		redBold := color.New(color.FgRed, color.Bold).SprintFunc()
		dimStyle := color.New(color.Faint).SprintFunc()
		whiteBold := color.New(color.FgWhite, color.Bold).SprintFunc()

		bannerText := "ZQK POLICY RULE STUDIO — STEP 3: ATOMIC CAS COMMIT & GATE PROMOTION"
		buf.WriteString(cyanBold("╔"+strings.Repeat("═", termWidth-2)+"╗") + "\n")
		buf.WriteString(cyanBold("║") + tds.PadCenter(whiteBold(bannerText), termWidth-2) + cyanBold("║") + "\n")
		buf.WriteString(cyanBold("╚"+strings.Repeat("═", termWidth-2)+"╝") + "\n")

		specLines := []string{
			fmt.Sprintf("%s %s   │   %s %s   │   %s %s",
				cyanBold("POLICY ID:"), whiteBold("POL-MUTATION-002"),
				cyanBold("TARGET KIND:"), greenBold("[backlog_item]"),
				cyanBold("SEVERITY:"), redBold("ERROR (Reject Non-Compliant Mutations)"),
			),
			fmt.Sprintf("%s %s",
				dimStyle("TARGET FILE:"), whiteBold(paths.ProcessPoliciesDir+"/POL-MUTATION-002.yaml (Declarative Policy Schema v1)"),
			),
			fmt.Sprintf("%s %s",
				cyanBold("EXPRESSION:"), whiteBold(`status == "in_progress" ==> claimed_by != "" && criteria_linked_or_acceptance_present == true`),
			),
		}
		buf.WriteString(tds.Panel("1. POLICY COMMIT SPECIFICATION", specLines, termWidth, tds.BorderLight))

		auditLines := []string{
			fmt.Sprintf("%s %s │ %s %s",
				greenBold("✓"), greenBold("194 / 196 entities COMPLIANT (99.0% safe)"),
				yellowBold("⚠️"), yellowBold("2 entities FLAGGED (Grandfathered until next mutation)"),
			),
			fmt.Sprintf("%s %s",
				cyanBold("ENFORCEMENT MODE:"), dimStyle("[CHECK_VALVE_ON_TRANSITION] — Zero retroactive disruption to planned work"),
			),
		}
		buf.WriteString(tds.Panel("2. PRE-COMMIT POPULATION DRY-RUN AUDIT (196 Active Entities Evaluated)", auditLines, termWidth, tds.BorderLight))

		receiptLines := []string{
			fmt.Sprintf("%s %s %s",
				cyanBold("CAS HASH:"), whiteBold("sha256:7f4a2b9e810459c03842d0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1"), greenBold("(Plane: authoritative)"),
			),
			fmt.Sprintf("%s %s %s",
				cyanBold("GATE BINDING:"), greenBold("✓"), dimStyle("Registered with pre-commit hook, ZQL mutation membrane, and `zqk do` check-valves"),
			),
			fmt.Sprintf("%s %s │ %s %s │ %s %s",
				dimStyle("ETAG:"), whiteBold(`"rev-001-c8104"`),
				dimStyle("PERMISSIONS:"), dimStyle("0644"),
				dimStyle("AUDIT STREAM:"), dimStyle(paths.ProjectDataDir+"/streams/audit_event/AUD-POL-002-INIT.jsonl"),
			),
		}
		buf.WriteString(tds.Panel("3. ATOMIC CAS STORAGE PROMOTION RECEIPT", receiptLines, termWidth, tds.BorderLight))

		navLines := []string{
			fmt.Sprintf("%s │ %s │ %s │ %s",
				cyanBold("[Enter] View in Object Inspector"),
				dimStyle("[e] Re-Edit Expression"),
				greenBold("[l] List All Policies"),
				dimStyle("[Esc] Return to Console"),
			),
		}
		buf.WriteString(tds.Panel("POST-COMMIT NAVIGATION", navLines, termWidth, tds.BorderLight))

		saveTitle := paths.RewriteCanonicalCLIInvocations("zqk object inspect --policy-studio") + " — Step 3: Save & Atomic CAS Promotion Receipt"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_policy_studio_save.svg"), saveTitle, buf.String(), termWidth); err != nil {
			fmt.Printf("Error rendering Policy Studio Save: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_policy_studio_save.svg")
		}
	}

	// 10. Test Verification Dashboard
	{
		var buf strings.Builder
		cyanBold := color.New(color.FgCyan, color.Bold).SprintFunc()
		greenBold := color.New(color.FgGreen, color.Bold).SprintFunc()
		yellowBold := color.New(color.FgYellow, color.Bold).SprintFunc()
		dimStyle := color.New(color.Faint).SprintFunc()

		buf.WriteString(cyanBold(strings.Repeat("=", termWidth)) + "\n")
		buf.WriteString(cyanBold("🚀 ZQK TEST & DEFINITION OF DONE (DoD) DASHBOARD | [ACTIVE WORKING SET]") + "\n")
		buf.WriteString(cyanBold(strings.Repeat("=", termWidth)) + "\n")
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

		buf.WriteString(dimStyle(strings.Repeat("─", termWidth)) + "\n")
		buf.WriteString("📡 RECENT CRITERIA SATISFACTION & SHOCKWAVE EVENTS\n")
		buf.WriteString(dimStyle(strings.Repeat("─", termWidth)) + "\n")
		buf.WriteString("  ⚡ [12:14:01] CRITERION SATISFIED: CRIT-STORAGE-PUREGO-EMBEDDED-001\n")
		buf.WriteString("  ⚡ [12:14:01] TEST CASE TST-STORAGE-PUREGO-001: active -> complete\n")
		buf.WriteString("  ⚡ [12:14:01] TRACEABILITY CHAIN GRADUATED: TST-STORAGE-PUREGO-001 ➔ Moved to Regression Pool\n")
		buf.WriteString("  ⚡ [12:14:02] SHOCKWAVE PROPAGATED: Latch complete on requirement REQ-STORAGE-PUREGO\n")
		buf.WriteString(dimStyle(strings.Repeat("─", termWidth)) + "\n")

		testDashTitle := paths.RewriteCanonicalCLIInvocations("zqk test dashboard") + " — Definition of Done & Live Test Verification Matrix"
		if err := RenderScreenToSVG(filepath.Join(outDir, "ui_test_dashboard.svg"), testDashTitle, buf.String(), termWidth); err != nil {
			fmt.Printf("Error rendering Test Dashboard: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_test_dashboard.svg")
		}
	}

	// 11. Visual Web Studio DAG Graph View
	{
		if err := RenderWebStudioSVG(filepath.Join(outDir, "ui_web_studio_dag.svg")); err != nil {
			fmt.Printf("Error rendering Web Studio DAG: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_web_studio_dag.svg")
		}
		// Also write to canonical ui_web_studio.svg
		_ = RenderWebStudioSVG(filepath.Join(outDir, "ui_web_studio.svg"))
	}

	// 12. Visual Web Studio Timeline & Gantt View
	{
		if err := RenderWebStudioGanttSVG(filepath.Join(outDir, "ui_web_studio_gantt.svg")); err != nil {
			fmt.Printf("Error rendering Web Studio Gantt: %v\n", err)
		} else {
			fmt.Println("✅ Generated ui_web_studio_gantt.svg")
		}
	}

	fmt.Println("🎉 All SVG visual mockups successfully generated and validated!")
}
