#!/usr/bin/env python3
"""
generate_manual_screenshots.py — Generates authentic, vector-sharp SVG screenshots
for the ZQK Mission Control TUI (all 8 tabs), Object Inspector, Test Dashboard,
and Visual Web Studio.
Ensures 100% XML 1.0 validation conformance (zero unescaped characters or control codes).
Outputs to docs/manual/screenshots/*.svg.
"""

import html
import os
import sys
import xml.etree.ElementTree as ET

COLOR_MAP = {
    "cyan": "#94e2d5",
    "green": "#a6e3a1",
    "yellow": "#f9e2af",
    "red": "#f38ba8",
    "blue": "#89b4fa",
    "magenta": "#cba6f7",
    "white": "#ffffff",
    "gray": "#6c7086",
    "dim": "#585b70",
    "text": "#cdd6f4",
    "peach": "#fab387",
    "mauve": "#cba6f7",
}

def make_span(text, color=None, bold=False):
    escaped = html.escape(text)
    attrs = []
    if color and color in COLOR_MAP:
        attrs.append(f'fill="{COLOR_MAP[color]}"')
    elif color and color.startswith("#"):
        attrs.append(f'fill="{color}"')
    if bold:
        attrs.append('font-weight="bold"')
    if not attrs:
        return escaped
    return f'<tspan {" ".join(attrs)}>{escaped}</tspan>'

def generate_svg(filename, title, lines, width=960, line_height=20, padding_x=24, padding_y=16, top_bar_height=42):
    total_height = top_bar_height + padding_y * 2 + len(lines) * line_height + 12

    svg_parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {total_height}" width="{width}" height="{total_height}">',
        '  <defs>',
        '    <style>',
        '      .window-bg { fill: #1e1e2e; rx: 12px; }',
        '      .top-bar { fill: #181825; }',
        '      .dot-red { fill: #f38ba8; }',
        '      .dot-yellow { fill: #f9e2af; }',
        '      .dot-green { fill: #a6e3a1; }',
        '      .term-text { font-family: "JetBrains Mono", "Fira Code", "Menlo", "Monaco", "Consolas", monospace;',
        '                   font-size: 13px; fill: #cdd6f4; white-space: pre; }',
        '      .title-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;',
        '                    font-size: 12px; fill: #a6adc8; font-weight: 500; text-anchor: middle; }',
        '    </style>',
        '    <filter id="shadow" x="-5%" y="-5%" width="110%" height="110%">',
        '      <feDropShadow dx="0" dy="8" stdDeviation="16" flood-color="#000000" flood-opacity="0.45"/>',
        '    </filter>',
        '  </defs>',
        '',
        f'  <rect x="4" y="4" width="{width - 8}" height="{total_height - 8}" class="window-bg" filter="url(#shadow)" stroke="#313244" stroke-width="1"/>',
        f'  <path d="M 4 16 A 12 12 0 0 1 16 4 L {width - 16} 4 A 12 12 0 0 1 {width - 4} 16 L {width - 4} {top_bar_height} L 4 {top_bar_height} Z" class="top-bar"/>',
        f'  <line x1="4" y1="{top_bar_height}" x2="{width - 4}" y2="{top_bar_height}" stroke="#313244" stroke-width="1"/>',
        '  <circle cx="24" cy="23" r="6" class="dot-red"/>',
        '  <circle cx="44" cy="23" r="6" class="dot-yellow"/>',
        '  <circle cx="64" cy="23" r="6" class="dot-green"/>',
        f'  <text x="{width / 2}" y="27" class="title-text">{html.escape(title)}</text>',
        f'  <g transform="translate({padding_x}, {top_bar_height + padding_y})">',
        '    <text class="term-text">'
    ]

    for idx, spans in enumerate(lines):
        y_pos = (idx + 1) * line_height - 4
        svg_parts.append(f'      <tspan x="0" y="{y_pos}">{spans}</tspan>')

    svg_parts.extend([
        '    </text>',
        '  </g>',
        '</svg>'
    ])

    svg_content = '\n'.join(svg_parts)
    ET.fromstring(svg_content)  # Conformance test
    with open(filename, "w", encoding="utf-8") as f:
        f.write(svg_content)
    print(f"✅ Generated {filename}")

def build_header(active_tab_idx, dynamic_msg="System operating normally — ambient telemetry stream active"):
    tab_names = [
        ("1: ⚡ State", "State"),
        ("2: 📜 Audit", "Audit"),
        ("3: 🤖 Swarm", "Swarm"),
        ("4: 📋 PM", "PM"),
        ("5: 📊 Metrics", "Metrics"),
        ("6: ⏱️ Sched", "Sched"),
        ("7: 🧪 QA", "QA"),
        ("8: 🛡️ Health", "Health")
    ]
    lines = []
    lines.append(make_span("╔═══════════════════════════════════════════════════════════════════════════════════════════════════════════════╗", "cyan", bold=True))
    lines.append(make_span("║", "cyan", bold=True) + make_span("                             ⚡ ZQK KNOWLEDGE KERNEL — MISSION CONTROL CONSOLE                                 ", "white", bold=True) + make_span("║", "cyan", bold=True))
    lines.append(make_span("╚═══════════════════════════════════════════════════════════════════════════════════════════════════════════════╝", "cyan", bold=True))
    
    tab_spans = []
    for idx, (full, short) in enumerate(tab_names, 1):
        if idx == active_tab_idx:
            tab_spans.append(make_span(f"[{full}]", "green", bold=True))
        else:
            tab_spans.append(make_span(f" {full} ", "gray"))
    lines.append(" │ ".join(tab_spans))
    lines.append(make_span("─" * 113, "dim"))
    lines.append(make_span("🔔 MESSAGE: ", "yellow", bold=True) + make_span(dynamic_msg, "text"))
    lines.append(make_span("─" * 113, "dim"))
    return lines

def main():
    target_dir = os.path.join(os.getcwd(), "docs", "manual", "screenshots")
    os.makedirs(target_dir, exist_ok=True)

    # 1. Tab 1: State
    lines_tab1 = build_header(1, "WAL change journal tail streaming — 0 corruptions detected")
    lines_tab1.extend([
        make_span("Stream: ", "cyan") + make_span("change_journal", "white", bold=True) + make_span(" │ Buffer: 48 mutations │ Rate: ", "gray") + make_span("▄▆█▇▅▃  ", "green") + make_span("│ [AUTO-SCROLL: ON]", "yellow"),
        make_span("Active Plane: ", "gray") + make_span("PlanePromoted (CAS Master)", "green", bold=True) + make_span(" │ Retention Watermark: 2m0s (fresh)", "gray"),
        make_span("─" * 113, "dim"),
        make_span("  TIME     │ EVENT              │ OBJECT REF             │ SUMMARY / DIFF", "cyan", bold=True),
        make_span("─" * 113, "dim"),
        make_span("> [11:42:01] │ ", "gray") + make_span("[PROMOTE]", "green", bold=True) + make_span("          │ BLI-ECOSYSTEM-001      │ Promoted draft to CAS master (hash: e3b0c442)", "text"),
        make_span("  [11:42:08] │ ", "gray") + make_span("[UPDATE]", "blue", bold=True) + make_span("           │ REQ-DATA-PLANE-004     │ Added requirement_ref CRIT-019", "text"),
        make_span("  [11:42:15] │ ", "gray") + make_span("[CREATE]", "cyan", bold=True) + make_span("           │ QUE-199-FIRST-RUN      │ Minted new object on PlaneDraft", "text"),
        make_span("  [11:42:22] │ ", "gray") + make_span("[LATCH]", "yellow", bold=True) + make_span("            │ CRIT-STORAGE-PUREGO    │ Latch satisfied: TestPureGoIndex passed", "text"),
        make_span("  [11:42:30] │ ", "gray") + make_span("[TRANSITION]", "magenta", bold=True) + make_span("       │ PRI-LAUNCH-READINESS   │ Status planned -> in_progress", "text"),
        make_span("  [11:42:45] │ ", "gray") + make_span("[PROMOTE]", "green", bold=True) + make_span("          │ BLI-MEMBRANE-001       │ Promoted to validated (CAS: c32587dd)", "text"),
        make_span("  [11:42:58] │ ", "gray") + make_span("[UPDATE]", "blue", bold=True) + make_span("           │ GOAL-001               │ Updated percent_complete: 95% -> 100%", "text"),
        make_span("─" * 113, "dim"),
        make_span("[Enter] Inspect Row │ [Space] Pause/Resume Stream │ [c] Compact WAL │ [g/G] Top/Bottom │ [q] Quit", "gray")
    ])
    generate_svg(os.path.join(target_dir, "ui_tab1_state.svg"), "zqk ui — Tab 1: State Seismograph & Mutation WAL", lines_tab1)

    # 2. Tab 2: Audit
    lines_tab2 = build_header(2, "Tamper-evident audit log stream active — 100% cryptographic lineage intact")
    lines_tab2.extend([
        make_span("Stream: ", "cyan") + make_span("audit_event", "white", bold=True) + make_span(" │ Buffer: 124 audit events │ Rate: ", "gray") + make_span("▃▄▅▃▂  ", "green") + make_span("│ [AUTO-SCROLL: ON]", "yellow"),
        make_span("Actors: ", "gray") + make_span("ACC-SYSTEM (82), PER-DEFAULT-LEAD (24), agent-alpha (18)", "white"),
        make_span("─" * 113, "dim"),
        make_span("  TIME     │ ACTOR            │ OPERATION          │ OBJECT REF       │ DETAILS / PAYLOAD", "cyan", bold=True),
        make_span("─" * 113, "dim"),
        make_span("> [11:39:10] │ ", "gray") + make_span("PER-DEFAULT-LEAD ", "yellow") + make_span("│ agent.claim-work   │ BLI-STARTER-001  │ Claimed item for execution loop", "text"),
        make_span("  [11:39:24] │ ", "gray") + make_span("agent-alpha      ", "peach") + make_span("│ object.ref.add     │ BLI-STARTER-001  │ Linked target REQ-LAUNCH-DOCS", "text"),
        make_span("  [11:39:45] │ ", "gray") + make_span("agent-alpha      ", "peach") + make_span("│ object.promote     │ BLI-STARTER-001  │ Transition planned -> in_progress", "text"),
        make_span("  [11:40:12] │ ", "gray") + make_span("ACC-SYSTEM       ", "cyan") + make_span("│ scheduler.tick     │ SCH-RETENTION    │ Scanned 187 objects; pruned 0 stale", "text"),
        make_span("  [11:40:30] │ ", "gray") + make_span("ACC-SYSTEM       ", "cyan") + make_span("│ cas.verify-hash    │ CAS-BLOB-9821    │ Verified SHA-256 integrity match", "text"),
        make_span("  [11:40:48] │ ", "gray") + make_span("agent-alpha      ", "peach") + make_span("│ test.execute       │ TST-STORAGE-001  │ Run completed: 0 failures, 14 checks", "text"),
        make_span("─" * 113, "dim"),
        make_span("[Enter] Drill-down Audit Payload │ [/] Search Actor │ [r] Refresh CAS Index │ [q] Quit", "gray")
    ])
    generate_svg(os.path.join(target_dir, "ui_tab2_audit.svg"), "zqk ui — Tab 2: Operational Audit Trail & CAS Provenance", lines_tab2)

    # 3. Tab 3: Swarm
    lines_tab3 = build_header(3, "Autonomous agent swarm online — 3 holons seated, 0 pending messages")
    lines_tab3.extend([
        make_span("Swarm Population: 3 Active Holons │ Mesh Topology: P2P Sovereign Cell │ Mailbox Status: 0 Pending", "white", bold=True),
        make_span("─" * 113, "dim"),
        make_span("  AGENT ID         │ SEAT / ROLE             │ STATUS        │ ACTIVE WORK UNIT │ INBOX │ HEARTBEAT", "cyan", bold=True),
        make_span("─" * 113, "dim"),
        make_span("> [agent-lead-01]  │ ", "white") + make_span("PER-DEFAULT-LEAD (Lead) ", "yellow") + make_span("│ ", "gray") + make_span("EXECUTING     ", "green", bold=True) + make_span("│ BLI-LAUNCH-002   │ 0     │ 2s ago (fresh)", "text"),
        make_span("  [agent-qa-02]    │ ", "white") + make_span("PER-DEFAULT-QA (QA)     ", "blue") + make_span("│ ", "gray") + make_span("VALIDATING    ", "cyan", bold=True) + make_span("│ TST-STORAGE-001  │ 0     │ 4s ago (fresh)", "text"),
        make_span("  [agent-tpm-03]   │ ", "white") + make_span("PER-DEFAULT-TPM (TPM)   ", "magenta") + make_span("│ ", "gray") + make_span("IDLE          ", "gray") + make_span("│ (none)           │ 0     │ 1s ago (fresh)", "text"),
        make_span("─" * 113, "dim"),
        make_span("── ACTIVE EXECUTION ENVELOPE: [agent-lead-01] ───────────────────────────────────────────────────────────────────", "yellow", bold=True),
        make_span("  Goal        : ", "gray") + make_span("GOAL-001 (Launch testing and vetting)", "white"),
        make_span("  Requirement : ", "gray") + make_span("REQ-LAUNCH-DOCS (100% documentation & guide accuracy)", "white"),
        make_span("  Current Task: ", "gray") + make_span("Verifying visual dashboards across all 8 tabs and updating onboarding guides", "text"),
        make_span("─" * 113, "dim"),
        make_span("[Enter] Inspect Agent State │ [s] Steer / Send Directive │ [k] Terminate Holon │ [q] Quit", "gray")
    ])
    generate_svg(os.path.join(target_dir, "ui_tab3_swarm.svg"), "zqk ui — Tab 3: Swarm Topology & Agent Seating", lines_tab3)

    # 4. Tab 4: PM
    lines_tab4 = build_header(4, "Lead priority plan active — 100% shovel-ready runway verified")
    lines_tab4.extend([
        make_span("Strategic Plan: SPL-LAUNCH-2026 │ Lead Priority Plan: ", "gray") + make_span("PRI-STARTER-001 [ACTIVE]", "green", bold=True),
        make_span("Runway Depth: 1 shovel-ready │ Backlog: 1 planned, 0 in_progress, 0 blocked, 196 complete", "white"),
        make_span("─" * 113, "dim"),
        make_span("  WORKSTREAM │ PRIORITY PLAN           │ ITEM ID │ STATUS      │ PRI │ TITLE", "cyan", bold=True),
        make_span("─" * 113, "dim"),
        make_span("> WS-LAUNCH  │ PRI-STARTER-001         │ BLI-001 │ ", "text") + make_span("planned     ", "yellow", bold=True) + make_span("│ ", "gray") + make_span("P0  ", "red", bold=True) + make_span("│ Community launch testing and vetting", "text"),
        make_span("  WS-KERNEL  │ PRI-STORAGE-PUREGO-001  │ BLI-042 │ ", "text") + make_span("complete    ", "green") + make_span("│ P1  │ Implement pure-Go CAS storage backend", "text"),
        make_span("  WS-KERNEL  │ PRI-STORAGE-PUREGO-001  │ BLI-043 │ ", "text") + make_span("complete    ", "green") + make_span("│ P1  │ Storage concurrency stress test suite", "text"),
        make_span("  WS-DOCS    │ PRI-DOC-EXPANSION-001   │ BLI-088 │ ", "text") + make_span("complete    ", "green") + make_span("│ P2  │ Multi-category documentation portal (187 arts)", "text"),
        make_span("  WS-SECURITY│ PRI-CRYPTO-AUDIT-001    │ BLI-112 │ ", "text") + make_span("complete    ", "green") + make_span("│ P0  │ Keystore tamper-proof signature verification", "text"),
        make_span("─" * 113, "dim"),
        make_span("[Enter] Inspect Backlog Item │ [c] Claim Work (zqk do) │ [p] Promote Status │ [q] Quit", "gray")
    ])
    generate_svg(os.path.join(target_dir, "ui_tab4_pm.svg"), "zqk ui — Tab 4: TPM Gantt Matrix & Backlog", lines_tab4)

    # 5. Tab 5: Metrics
    lines_tab5 = build_header(5, "Subsystem telemetry nominal — latency P95 < 25ms, zero lock deadlocks")
    lines_tab5.extend([
        make_span("Memory: 48.2 MB │ CAS Objects: 187 objects (1.4 MB) │ WAL Journal: 14.8 KB │ Uptime: 4h 12m", "white"),
        make_span("Token Burn Velocity: 1,420 tokens/hr │ CLI Command Throughput: 42.4 cmd/min", "gray"),
        make_span("─" * 113, "dim"),
        make_span("── COMMAND LATENCY HISTOGRAM (P50: 12ms │ P95: 22ms │ P99: 48ms) ──────────────────────────────────────────────", "cyan", bold=True),
        make_span("  0ms - 20ms   [", "text") + make_span("████████████████████████████████████████", "green") + make_span("] 89.2% (1,340 calls)", "text"),
        make_span("  20ms - 50ms  [", "text") + make_span("████                                    ", "yellow") + make_span("]  8.4% (126 calls)", "text"),
        make_span("  50ms - 100ms [", "text") + make_span("█                                       ", "peach") + make_span("]  2.0% (30 calls)", "text"),
        make_span("  100ms+       [                                        ]  0.4% (6 calls)", "gray"),
        make_span(""),
        make_span("── SUBSYSTEM CACHE EFFICIENCY ───────────────────────────────────────────────────────────────────────────────────", "cyan", bold=True),
        make_span("  Schema Registry Cache : ", "gray") + make_span("99.4% Hit Rate", "green", bold=True) + make_span(" (1,840 hits / 11 misses)", "text"),
        make_span("  CAS Memory Plane Cache: ", "gray") + make_span("96.2% Hit Rate", "green", bold=True) + make_span(" (4,210 hits / 164 misses)", "text"),
        make_span("  ZPARQL Plan Cache     : ", "gray") + make_span("93.8% Hit Rate", "green", bold=True) + make_span(" (320 hits / 21 misses)", "text"),
        make_span("─" * 113, "dim"),
        make_span("[r] Force Cache Prewarm │ [c] Compact Memory Buffer │ [q] Quit", "gray")
    ])
    generate_svg(os.path.join(target_dir, "ui_tab5_metrics.svg"), "zqk ui — Tab 5: Kernel Telemetry & Latencies", lines_tab5)

    # 6. Tab 6: Scheduler
    lines_tab6 = build_header(6, "Scheduler daemon healthy — 8 active jobs scheduled, 0 failures")
    lines_tab6.extend([
        make_span("Scheduler Daemon: ", "gray") + make_span("RUNNING (PID: 84920)", "green", bold=True) + make_span(" │ Worker Pool: 4 workers │ Ticks: 60/min", "gray"),
        make_span("Active Cron Jobs: 8 registered │ Last Maintenance Sweep: 42s ago [PASS]", "white"),
        make_span("─" * 113, "dim"),
        make_span("  JOB ID  │ NAME                        │ INTERVAL │ LAST RUN │ DURATION │ NEXT RUN │ STATUS", "cyan", bold=True),
        make_span("─" * 113, "dim"),
        make_span("> SCH-001 │ change_journal_compaction   │ @every 5m│ 11:40:00 │ 42ms     │ 11:45:00 │ ", "text") + make_span("[PASS]", "green", bold=True),
        make_span("  SCH-002 │ audit_aggregation           │ @every 1h│ 11:00:00 │ 124ms    │ 12:00:00 │ ", "text") + make_span("[PASS]", "green", bold=True),
        make_span("  SCH-003 │ cas_hygiene_scan            │ @every 6h│ 06:00:00 │ 310ms    │ 12:00:00 │ ", "text") + make_span("[PASS]", "green", bold=True),
        make_span("  SCH-004 │ memory_leak_watchdog        │ @every 1m│ 11:43:00 │ 12ms     │ 11:44:00 │ ", "text") + make_span("[PASS]", "green", bold=True),
        make_span("  SCH-005 │ test_matrix_prewarm         │ @every 2m│ 11:42:30 │ 88ms     │ 11:44:30 │ ", "text") + make_span("[PASS]", "green", bold=True),
        make_span("  SCH-006 │ stale_lock_reaper           │ @every 1m│ 11:43:10 │ 8ms      │ 11:44:10 │ ", "text") + make_span("[PASS]", "green", bold=True),
        make_span("─" * 113, "dim"),
        make_span("[Enter] Run Job Immediately │ [p] Pause/Resume Job │ [s] Restart Scheduler │ [q] Quit", "gray")
    ])
    generate_svg(os.path.join(target_dir, "ui_tab6_scheduler.svg"), "zqk ui — Tab 6: Scheduler Daemons & Cron Jobs", lines_tab6)

    # 7. Tab 7: QA
    lines_tab7 = build_header(7, "Definition of Done (DoD) satisfied: 100% test coverage and intact lineage")
    lines_tab7.extend([
        make_span("DoD Compliance: ", "gray") + make_span("100% [PASS]", "green", bold=True) + make_span(" │ Intact Lineage Chains: 174/174 │ Unbound Criteria: 0 [OK]", "white"),
        make_span("Test Coverage: 100% BLI Coverage (43/43 Backlog Items grounded in executable test_case objects)", "gray"),
        make_span("─" * 113, "dim"),
        make_span("  TEST CASE ID             │ STATUS     │ LINEAGE HEALTH │ CRITERIA MET │ PROGRESS BAR", "cyan", bold=True),
        make_span("─" * 113, "dim"),
        make_span("> TST-STORAGE-PUREGO-001   │ ", "text") + make_span("complete   ", "green") + make_span("│ ", "gray") + make_span("✓ INTACT       ", "green", bold=True) + make_span("│ 1 / 1 (100%) │ [", "text") + make_span("████████████████████", "green") + make_span("]", "text"),
        make_span("  TST-ZQL-ACID-TRANSACT-01 │ ", "text") + make_span("complete   ", "green") + make_span("│ ", "gray") + make_span("✓ INTACT       ", "green", bold=True) + make_span("│ 3 / 3 (100%) │ [", "text") + make_span("████████████████████", "green") + make_span("]", "text"),
        make_span("  TST-ZPARQL-PLANNER-002   │ ", "text") + make_span("complete   ", "green") + make_span("│ ", "gray") + make_span("✓ INTACT       ", "green", bold=True) + make_span("│ 2 / 2 (100%) │ [", "text") + make_span("████████████████████", "green") + make_span("]", "text"),
        make_span("  TST-COMMUNITY-FIRST-RUN  │ ", "text") + make_span("active     ", "cyan") + make_span("│ ", "gray") + make_span("✓ INTACT       ", "green", bold=True) + make_span("│ 1 / 1 (100%) │ [", "text") + make_span("████████████████████", "green") + make_span("]", "text"),
        make_span("─" * 113, "dim"),
        make_span("── TRACEABILITY RADAR: [TST-STORAGE-PUREGO-001] ─────────────────────────────────────────────────────────────────", "cyan", bold=True),
        make_span("  Goal        : ", "gray") + make_span("[🟢 GOAL-STORAGE-PUREGO] High-Performance Pure-Go Storage Engine", "white"),
        make_span("  Requirement : ", "gray") + make_span("[🟢 REQ-STORAGE-PUREGO-001] Zero CGo native file-backed graph backend", "white"),
        make_span("  Backlog Item: ", "gray") + make_span("[🟢 BLI-STORAGE-001] Implement pure-Go CAS storage backend", "white"),
        make_span("  Test Target : ", "gray") + make_span("pkg/storage/purego_index_test.go:TestPureGoIndex (integration)", "text"),
        make_span("  Bound Criteria: ", "gray") + make_span("[🟢 CRIT-STORAGE-PUREGO-001] (satisfied by automated test)", "green"),
        make_span("─" * 113, "dim"),
        make_span("[t] Run Test Suite │ [Enter] Inspect Criteria Details │ [r] Rescan DoD Matrix │ [q] Quit", "gray")
    ])
    generate_svg(os.path.join(target_dir, "ui_tab7_qa.svg"), "zqk ui — Tab 7: QA Done-Gates & Lineage Radar", lines_tab7)

    # 8. Tab 8: Health
    lines_tab8 = build_header(8, "System health check passed: 0 CAS blockers, 0 stale locks, 0 orphaned files")
    lines_tab8.extend([
        make_span("Knowledge Kernel State: ", "gray") + make_span("HEALTHY", "green", bold=True) + make_span(" │ Storage Backend: file (hybrid CAS) │ Lock Contention: 0 deadlocks", "white"),
        make_span("Membrane Isolation: Mode B (Strict CAS Verification) │ Integrity Score: ", "gray") + make_span("100 / 100", "green", bold=True),
        make_span("─" * 113, "dim"),
        make_span("  SUBSYSTEM            │ HEALTH STATUS │ CAS INTEGRITY │ WAL STATE   │ DETAILS", "cyan", bold=True),
        make_span("─" * 113, "dim"),
        make_span("> Content-Addressed DB │ ", "text") + make_span("[OK]          ", "green", bold=True) + make_span("│ 187/187 valid │ 0 corrupt   │ SHA-256 digests match index", "text"),
        make_span("  Filesystem Watcher   │ ", "text") + make_span("[OK]          ", "green", bold=True) + make_span("│ (n/a)         │ streaming   │ Inotify / kqueue listener healthy", "text"),
        make_span("  Scheduler Daemon     │ ", "text") + make_span("[OK]          ", "green", bold=True) + make_span("│ (n/a)         │ nominal     │ 8 jobs running; 0 errors", "text"),
        make_span("  Membrane Check-Valve │ ", "text") + make_span("[OK]          ", "green", bold=True) + make_span("│ active        │ verified    │ Zero unpromoted draft leaks", "text"),
        make_span("─" * 113, "dim"),
        make_span("── 4-LAYER COMPLIANCE CAKE STATUS ───────────────────────────────────────────────────────────────────────────────", "cyan", bold=True),
        make_span("  Layer 0: CAS & Integrity Blockers : ", "gray") + make_span("✓ PASS (0 blockers found)", "green", bold=True),
        make_span("  Layer 1: Object Preconditions      : ", "gray") + make_span("✓ PASS (0 precondition errors)", "green", bold=True),
        make_span("  Layer 2: Ready for Execution       : ", "gray") + make_span("✓ PASS (0 unhandled objects)", "green", bold=True),
        make_span("  Layer 3: Completed & Validated     : ", "gray") + make_span("✓ PASS (1,111 QA successes, 196 complete BLIs)", "green", bold=True),
        make_span("  Layer 4: I/O Resource Hygiene      : ", "gray") + make_span("✓ PASS (0 stale locks, 0 orphaned temp files, 10 open FDs)", "green", bold=True),
        make_span("─" * 113, "dim"),
        make_span("[Enter] Run System Diagnostic (zqk system check) │ [a] Run Alignment │ [q] Quit", "gray")
    ])
    generate_svg(os.path.join(target_dir, "ui_tab8_health.svg"), "zqk ui — Tab 8: 4-Layer Compliance Cake & System Health", lines_tab8)

    # 9. Object Inspector
    lines_insp = [
        make_span("╭─ OBJECT INSPECTOR: BLI-001 ────────────────────────────────────────────────────────────────────────────────╮", "cyan", bold=True),
        make_span("│ ", "cyan") + make_span("Kind     : ", "gray") + make_span("backlog_item                    ", "white", bold=True) + make_span("│ Status   : ", "gray") + make_span("validated (ready for promote) ", "green", bold=True) + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("ID       : ", "gray") + make_span("BLI-001                         ", "yellow", bold=True) + make_span("│ Priority : ", "gray") + make_span("P0 (Critical)                 ", "red", bold=True) + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("Title    : ", "gray") + make_span("Harmonize CLI Taxonomy & 100% Spec Coverage                             ", "white") + make_span("│", "cyan"),
        make_span("├────────────────────────────────────────────────────────────────────────────────────────────────────────────┤", "cyan"),
        make_span("│ ", "cyan") + make_span("GRAPH LINEAGE & TRACEABILITY RADAR:                                                                        ", "cyan", bold=True) + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  Upstream Goal       : ", "gray") + make_span("[🟢 GOAL-001] Canonical CLI Specification Suite                  ", "white") + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  Parent Requirement  : ", "gray") + make_span("[🟢 REQ-012] Complete Command Spec Verification                  ", "white") + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  Bound Test Case     : ", "gray") + make_span("[🟢 TST-030-01] TestValidateCommandSpecs                         ", "white") + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  Lineage Integrity   : ", "gray") + make_span("✓ 100% INTACT (Zero orphan references)                            ", "green", bold=True) + make_span("│", "cyan"),
        make_span("├────────────────────────────────────────────────────────────────────────────────────────────────────────────┤", "cyan"),
        make_span("│ ", "cyan") + make_span("ACCEPTANCE CRITERIA & VDS DONE-GATE:                                                                         ", "cyan", bold=True) + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  [🟢 CRIT-001] (satisfied) All 42 Cobra command trees have matching YAML specs                           ", "text") + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  [🟢 CRIT-002] (satisfied) Command validation test passes with 0 drift                                   ", "text") + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  [🟢 CRIT-003] (satisfied) Flag aliases and parameter types adhere to taxonomy standards                  ", "text") + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  Criteria Saturation : ", "gray") + make_span("3 / 3 (100% satisfied, 0 pending)                                        ", "green", bold=True) + make_span("│", "cyan"),
        make_span("├────────────────────────────────────────────────────────────────────────────────────────────────────────────┤", "cyan"),
        make_span("│ ", "cyan") + make_span("CRYPTOGRAPHIC CAS PROVENANCE:                                                                                ", "cyan", bold=True) + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  CAS Content Digest  : ", "gray") + make_span("c32587ddd4fd8a87966c6cc0fdfbfa365069a02a33853a0a9ac64baa62a4b357       ", "text") + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  Commit Hash         : ", "gray") + make_span("8d077f97                                                                 ", "text") + make_span("│", "cyan"),
        make_span("│ ", "cyan") + make_span("  Seated Actor        : ", "gray") + make_span("ACC-SYSTEM (verified signature)                                          ", "text") + make_span("│", "cyan"),
        make_span("├────────────────────────────────────────────────────────────────────────────────────────────────────────────┤", "cyan"),
        make_span("│ ", "cyan") + make_span("ACTION PALETTE: [1] Promote [2] Demote [3] Add Ref [4] Remove Ref [5] Edit [v] Evaluate VDS [q] Quit         ", "yellow") + make_span("│", "cyan"),
        make_span("╰────────────────────────────────────────────────────────────────────────────────────────────────────────────╯", "cyan", bold=True)
    ]
    generate_svg(os.path.join(target_dir, "ui_object_inspector.svg"), "zqk object inspect — 7-Panel Interactive Inspector Console", lines_insp)

    # 10. Test Dashboard
    lines_test = [
        make_span("==================================================================================================", "cyan", bold=True),
        make_span("🚀 ZQK TEST & DEFINITION OF DONE (DoD) DASHBOARD", "white", bold=True),
        make_span("==================================================================================================", "cyan", bold=True),
        make_span("Working Set: ", "gray") + make_span("12 In-Flight Tests", "yellow", bold=True) + make_span(" │ Regression Pool: ", "gray") + make_span("174 Verified Chains (Green)", "green", bold=True),
        make_span(""),
        make_span("▶ TST-STORAGE-PUREGO-001  ", "cyan", bold=True) + make_span("[ACTIVE]  ", "yellow", bold=True) + make_span("Verify Pure-Go Indexing Engine Performance", "white"),
        make_span("    Lineage  : ", "gray") + make_span("[🟢 GOAL-001] ➔ [🟢 REQ-012] ➔ [🟢 BLI-001] ➔ [🟡 TST-STORAGE-PUREGO-001]  ", "text") + make_span("✓ Chain Intact", "green", bold=True),
        make_span("    Target   : pkg/storage/purego_index_test.go:TestPureGoIndex (integration)", "gray"),
        make_span("    Criteria : ", "gray") + make_span("[🟢 CRIT-001] ", "green") + make_span("(satisfied by automated proof)", "gray"),
        make_span("    Progress : [", "text") + make_span("████████████████████", "green") + make_span("] 100% (1/1 satisfied, 0 open)", "text"),
        make_span(""),
        make_span("▶ TST-ZQL-ACID-TRANSACT-01  ", "cyan", bold=True) + make_span("[ACTIVE]  ", "yellow", bold=True) + make_span("Verify Multi-Object Atomic Rollbacks", "white"),
        make_span("    Lineage  : ", "gray") + make_span("[🟢 GOAL-002] ➔ [🟢 REQ-015] ➔ [🟢 BLI-018] ➔ [🟡 TST-ZQL-ACID-TRANSACT-01]  ", "text") + make_span("✓ Chain Intact", "green", bold=True),
        make_span("    Target   : pkg/zql/transaction_test.go:TestAtomicRollback (unit)", "gray"),
        make_span("    Criteria : ", "gray") + make_span("[🟢 CRIT-010] [🟢 CRIT-011] ", "green") + make_span("(2/2 satisfied)", "text"),
        make_span("    Progress : [", "text") + make_span("████████████████████", "green") + make_span("] 100% (2/2 satisfied, 0 open)", "text"),
        make_span(""),
        make_span("🛡️  REGRESSION TESTING POOL: 174 verified chains green & passing", "green", bold=True),
        make_span("[Pruned from active view — inspect full regression suite with: zqk test dashboard --view regression]", "gray"),
        make_span(""),
        make_span("📡 RECENT CRITERIA SATISFACTION & SHOCKWAVE EVENTS", "yellow", bold=True),
        make_span("─" * 98, "dim"),
        make_span("  ⚡ [11:42:01] ", "gray") + make_span("CRITERION SATISFIED: ", "green", bold=True) + make_span("CRIT-001 (Automated Test Pass)", "text"),
        make_span("  ⚡ [11:42:01] ", "gray") + make_span("TEST CASE TST-STORAGE-PUREGO-001: ", "cyan") + make_span("active -> complete", "green"),
        make_span("  ⚡ [11:42:01] ", "gray") + make_span("TRACEABILITY CHAIN GRADUATED: ", "magenta") + make_span("Moved to Regression Pool", "white"),
        make_span("  ⚡ [11:42:02] ", "gray") + make_span("SHOCKWAVE PROPAGATED: ", "yellow") + make_span("Latch complete on requirement REQ-012", "text"),
        make_span("─" * 98, "dim")
    ]
    generate_svg(os.path.join(target_dir, "ui_test_dashboard.svg"), "zqk test dashboard — Definition of Done & Live Test Stream", lines_test)

    # 11. Web Studio
    lines_web = [
        make_span("┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐", "cyan"),
        make_span("│ ", "cyan") + make_span("⚡ ZQK STUDIO  │  Timeline & Gantt  │  Ontology DAG Visualizer  │  System Health  │  Port: 8080        ", "white", bold=True) + make_span("│", "cyan"),
        make_span("├────────────────────────────────────────────────────────────────────────────────────────────────────────┤", "cyan"),
        make_span("│                                                                                                        │", "cyan"),
        make_span("│  ", "cyan") + make_span("TIMELINE & GANTT ROADMAP", "cyan", bold=True) + make_span("                                                                              │", "cyan"),
        make_span("│                                                                                                        │", "cyan"),
        make_span("│  Sep 25                   Sep 28                   Today (Sep 29)           Oct 02             Oct 05  │", "gray"),
        make_span("│  ────────┼────────────────────────┼───────────────────────▼────────────────────────┼────────────────── │", "dim"),
        make_span("│                                                           │                                            │", "cyan"),
        make_span("│  ", "cyan") + make_span("WS-LAUNCH (Launch Readiness)                             ", "white", bold=True) + make_span("│                                            │", "cyan"),
        make_span("│  ├─ ", "cyan") + make_span("[◆ MIL-COMMUNITY-LAUNCH]                              ", "cyan", bold=True) + make_span("│                                            │", "cyan"),
        make_span("│  │  └─ ", "cyan") + make_span("[■ PRI-STARTER-COMMUNITY-001]                      ", "yellow", bold=True) + make_span("│                                            │", "cyan"),
        make_span("│  │     └─ ", "cyan") + make_span("[■ BLI-001] Community launch testing            ", "text") + make_span("│  (Active Shovel-Ready)                     ", "green", bold=True) + make_span("│", "cyan"),
        make_span("│  │                                                        │                                            │", "cyan"),
        make_span("│  ", "cyan") + make_span("WS-KERNEL (Core Subsystems)                              ", "white", bold=True) + make_span("│                                            │", "cyan"),
        make_span("│  ├─ ", "cyan") + make_span("[◆ MIL-PUREGO-STORAGE]                                ", "cyan", bold=True) + make_span("│                                            │", "cyan"),
        make_span("│  │  └─ ", "cyan") + make_span("[■ PRI-STORAGE-PUREGO-001] ", "yellow") + make_span("════════════════════════╡ (Complete)                                 ", "green") + make_span("│", "cyan"),
        make_span("│                                                           │                                            │", "cyan"),
        make_span("│  STATUS LEGEND:  ", "gray") + make_span("■ Planned   ", "blue") + make_span("■ In Progress   ", "yellow") + make_span("■ Blocked   ", "red") + make_span("■ Complete   ", "green") + make_span("◆ Milestone Marker               ", "cyan") + make_span("│", "cyan"),
        make_span("│                                                                                                        │", "cyan"),
        make_span("├────────────────────────────────────────────────────────────────────────────────────────────────────────┤", "cyan"),
        make_span("│                                                                                                        │", "cyan"),
        make_span("│  ", "cyan") + make_span("ONTOLOGY DAG DEPENDENCY GRAPH", "cyan", bold=True) + make_span("                                                                         │", "cyan"),
        make_span("│                                                                                                        │", "cyan"),
        make_span("│  ", "cyan") + make_span("[GOAL-STARTER] ", "green", bold=True) + make_span("──► ", "dim") + make_span("[REQ-LAUNCH-DOCS] ", "blue", bold=True) + make_span("──► ", "dim") + make_span("[BLI-001] ", "yellow", bold=True) + make_span("──► ", "dim") + make_span("[TST-COMMUNITY-001] ", "magenta", bold=True) + make_span("──► ", "dim") + make_span("[CRIT-DOCS-VERIFIED]   ", "cyan", bold=True) + make_span("│", "cyan"),
        make_span("│         │                                                                                              │", "cyan"),
        make_span("│         └─────────► ", "dim") + make_span("[REQ-AIRGAP-BUILD] ", "blue", bold=True) + make_span("──► ", "dim") + make_span("[BLI-002] ", "yellow", bold=True) + make_span("──► ", "dim") + make_span("[TST-AIRGAP-REPRO]  ", "magenta", bold=True) + make_span("──► ", "dim") + make_span("[CRIT-REPRO-TARBALL]  ", "cyan", bold=True) + make_span("│", "cyan"),
        make_span("│                                                                                                        │", "cyan"),
        make_span("└────────────────────────────────────────────────────────────────────────────────────────────────────────┘", "cyan")
    ]
    generate_svg(os.path.join(target_dir, "ui_web_studio.svg"), "zqk ui -w — Visual Web Studio: Timeline, Gantt & DAG Visualizer", lines_web)

if __name__ == "__main__":
    main()
