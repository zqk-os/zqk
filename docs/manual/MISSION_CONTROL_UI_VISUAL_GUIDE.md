# Manual: Mission Control TUI, Web Studio & Diagnostics Visual Guide

This visual guide documents the full interface layout, visual components, interactive hotkeys, and data interpretations for the **ZQK Mission Control Console (`zqk ui`)**, **Visual Web Studio (`zqk ui -w`)**, and **Test Verification Dashboard (`zqk test dashboard`)**.

---

## 1. System Navigation & Header Anatomy

When launched in a terminal (`zqk ui`), Mission Control renders a responsive ANSI terminal user interface (TUI). 

### Visual Terminal Screenshot: Header & Navigation Bar
```
╔═══════════════════════════════════════════════════════════════════════════════════════════════════════════════╗
║                             ⚡ ZQK KNOWLEDGE KERNEL — MISSION CONTROL CONSOLE                                 ║
╚═══════════════════════════════════════════════════════════════════════════════════════════════════════════════╝
[1: ⚡ State] │  2: 📜 Audit  │  3: 🤖 Swarm  │  4: 📋 PM  │  5: 📊 Metrics  │  6: ⏱️ Sched  │  7: 🧪 QA  │  8: 🛡️ Health 
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
🔔 MESSAGE: System operating normally — ambient telemetry stream active
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
🔍 SEARCH: [/auth█]  (Press Enter to lock search, Esc to cancel)
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
```

### Datapoint Breakdown & Operator Guidance:

| Datapoint / Component | Visual Format | Semantic Meaning | Why It Is Useful & Action Required |
| :--- | :--- | :--- | :--- |
| **Active Tab** | Reverse video `[1: ⚡ State]` | Indicates the currently active telemetry projection. | Use number keys `[1]`–`[8]` or `[Tab]` / `[Shift+Tab]` to cycle views instantly. |
| **Editor Profile** | `newb` / `pro` / `jedi` | Display density profile. Press `[z]` to cycle. | `newb` shows full borders; `pro` compresses into 1-line top bar; `jedi` maximizes screen rows for high-density monitors. |
| **Dynamic Message Line** | `🔔 MESSAGE: <status>` | Real-time ambient status accumulator and alert banner. | Green `✓` means normal; Yellow `⚡`/`⏳` indicates background compaction or sync; Red `✗` indicates blocked work or broken invariants. |
| **Interactive Search Buffer** | `🔍 SEARCH: [/<query>█]` | In-memory substring filter active across current tab rows. | Press `[/]` to enter search query, `[Enter]` to commit, `[n]`/`[N]` to jump between matches, `[Esc]` to clear. |

---

## 2. Tab 1: ⚡ State (Real-Time State Seismograph & Mutation Journal)

The State Tab provides a real-time seismograph of kernel mutations flowing through the change journal Write-Ahead Log (WAL).

### Visual Terminal Screenshot: Tab 1

![Tab 1: State Seismograph & Mutation WAL](./screenshots/ui_tab1_state.svg)

```
Stream: change_journal │ Buffer: 48 mutations │ Rate: ▄▆█▇▅▃  │ [AUTO-SCROLL: ON]
Active Plane: PlanePromoted (CAS Master) │ Retention Watermark: 2m0s (fresh)
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  TIME     │ EVENT              │ OBJECT REF             │ SUMMARY / DIFF
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
> [11:42:01] │ [PROMOTE]          │ BLI-ECOSYSTEM-001      │ Promoted draft to CAS master (hash: e3b0c442)
  [11:42:08] │ [UPDATE]           │ REQ-DATA-PLANE-004     │ Added requirement_ref CRIT-019
  [11:42:15] │ [CREATE]           │ QUE-199-FIRST-RUN      │ Minted new object on PlaneDraft
  [11:42:22] │ [LATCH]            │ CRIT-STORAGE-PUREGO    │ Latch satisfied: TestPureGoIndex passed
  [11:42:30] │ [TRANSITION]       │ PRI-LAUNCH-READINESS   │ Status planned -> in_progress
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
[Enter] Inspect Row │ [Space] Pause/Resume Stream │ [c] Compact WAL │ [g/G] Top/Bottom │ [q] Quit
```

### Datapoints & Diagnostics:
- **Rate Sparkline (`Rate: ▄▆█▇▅▃ `)**: Moving 60-second mutation frequency. Sudden tall bars indicate active batch ingestion or agent swarms committing work.
- **Active Plane**: Identifies whether the displayed view represents `PlaneDraft` (unverified agent workspace) or `PlanePromoted` (CAS authoritative master).
- **Auto-Scroll Mode**: When active (`[AUTO-SCROLL: ON]`), the viewport tracks the live stream tail. Press `[Space]` or `[↑]` to pause the stream and inspect past diffs without missing new records.
- **Event Badges**:
  - `[CREATE]`: New entity originated.
  - `[UPDATE]`: Scalar attribute or reference modified.
  - `[PROMOTE]`: Entity graduated across membrane from draft to CAS master.
  - `[LATCH]`: Acceptance criterion satisfied by an objective verification test.

---

## 3. Tab 2: 📜 Audit (Operational Audit Log & CAS Cryptographic Provenance)

The Audit Tab provides non-repudiable audit logs tracking which human or AI agent performed each action.

### Visual Terminal Screenshot: Tab 2

![Tab 2: Operational Audit Trail & CAS Provenance](./screenshots/ui_tab2_audit.svg)

```
Stream: audit_event │ Buffer: 124 audit events │ Rate: ▃▄▅▃▂  │ [AUTO-SCROLL: ON]
Actors: ACC-SYSTEM (82), PER-DEFAULT-LEAD (24), agent-alpha (18)
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  TIME     │ ACTOR            │ OPERATION          │ OBJECT REF       │ DETAILS / PAYLOAD
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
> [11:39:10] │ PER-DEFAULT-LEAD │ agent.claim-work   │ BLI-STARTER-001  │ Claimed item for execution loop
  [11:39:24] │ agent-alpha      │ object.ref.add     │ BLI-STARTER-001  │ Linked target REQ-LAUNCH-DOCS
  [11:39:45] │ agent-alpha      │ object.promote     │ BLI-STARTER-001  │ Transition planned -> in_progress
  [11:40:12] │ ACC-SYSTEM       │ scheduler.tick     │ SCH-RETENTION    │ Scanned 185 objects; pruned 0 stale
  [11:40:30] │ ACC-SYSTEM       │ cas.verify-hash    │ CAS-BLOB-9821    │ Verified SHA-256 integrity match
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
[Enter] Drill-down Audit Payload │ [/] Search Actor │ [r] Refresh CAS Index │ [q] Quit
```

### Datapoints & Diagnostics:
- **Actor Breakdown**: Summarizes event volume by persona or system process (`ACC-SYSTEM`, `agent-alpha`, etc.). Unbalanced event distributions point to thrashing agents or runaway loops.
- **Operation Column**: Precise API verb executed.
- **Cryptographic Provenance**: Selecting any row and pressing `[Enter]` displays the full JSON cryptographic receipt: including parent hash, caller account ID, signature, and immutable timestamp.

---

## 4. Tab 3: 🤖 Swarm (Multi-Agent Swarm Topology & Seating)

The Swarm Tab visualizes running agent processes, seated roles, active task assignments, and message queues.

### Visual Terminal Screenshot: Tab 3

![Tab 3: Swarm Topology & Agent Seating](./screenshots/ui_tab3_swarm.svg)

```
Swarm Population: 3 Active Holons │ Mesh Topology: P2P Sovereign Cell │ Mailbox Status: 0 Pending
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  AGENT ID         │ SEAT / ROLE             │ STATUS        │ ACTIVE WORK UNIT │ INBOX │ HEARTBEAT
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
> [agent-lead-01]  │ PER-DEFAULT-LEAD (Lead) │ EXECUTING     │ BLI-LAUNCH-002   │ 0     │ 2s ago (fresh)
  [agent-qa-02]    │ PER-DEFAULT-QA (QA)     │ VALIDATING    │ TST-STORAGE-001  │ 0     │ 4s ago (fresh)
  [agent-tpm-03]   │ PER-DEFAULT-TPM (TPM)   │ IDLE          │ (none)           │ 0     │ 1s ago (fresh)
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
── ACTIVE EXECUTION ENVELOPE: [agent-lead-01] ───────────────────────────────────────────────────────────────────
  Goal        : GOAL-STARTER-COMMUNITY-001 (Community launch testing and vetting)
  Requirement : REQ-LAUNCH-DOCS-COMPLETENESS (100% doc surface verification)
  Current Task: Verifying visual dashboards across all 8 tabs and updating onboarding guides
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
[Enter] Inspect Agent State │ [s] Steer / Send Directive │ [k] Terminate Holon │ [q] Quit
```

### Datapoints & Diagnostics:
- **Status Column**:
  - `IDLE`: Holon waiting for Shovel-Ready work. If runway > 0 and agents remain idle, verify agent seating with `zqk system agent-onboard`.
  - `EXECUTING`: Actively modifying workspace files within a claimed `BLI-*` envelope.
  - `VALIDATING`: Running tests (`zqk test run`) or waiting for VDS Done-Gate evaluation.
  - `BLOCKED`: Work is paused due to an unmet precondition or locked dependency.
- **Heartbeat Age**: Time since last ambient ping. Agents older than `60s` are flagged yellow; older than `180s` are highlighted in red as potentially hung.

---

## 5. Tab 4: 📋 PM (Gantt Matrix, Workstreams & Priority Plans)

The PM Tab displays the Technical Program Management (TPM) Gantt matrix and backlog status breakdown.

### Visual Terminal Screenshot: Tab 4

![Tab 4: TPM Gantt Matrix & Shovel-Ready Backlog](./screenshots/ui_tab4_pm.svg)

```
Strategic Plan: SPL-LAUNCH-2026 │ Lead Priority Plan: PRI-STARTER-COMMUNITY-001 [ACTIVE]
Runway Depth: 1 shovel-ready │ Backlog: 1 planned, 0 in_progress, 0 blocked, 42 complete
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  WORKSTREAM │ PRIORITY PLAN           │ ITEM ID │ STATUS      │ PRI │ TITLE
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
> WS-LAUNCH  │ PRI-STARTER-COMMUNITY-001 │ BLI-001 │ planned     │ P0  │ Community launch testing and vetting
  WS-KERNEL  │ PRI-STORAGE-PUREGO-001    │ BLI-042 │ complete    │ P1  │ Implement pure-Go CAS storage backend
  WS-KERNEL  │ PRI-STORAGE-PUREGO-001    │ BLI-043 │ complete    │ P1  │ Storage concurrency stress test suite
  WS-DOCS    │ PRI-DOC-EXPANSION-001     │ BLI-088 │ complete    │ P2  │ Multi-category documentation portal
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
[Enter] Inspect Backlog Item │ [c] Claim Work (`zqk do`) │ [p] Promote Status │ [q] Quit
```

### Datapoints & Diagnostics:
- **Runway Depth**: Number of shovel-ready tasks with all prerequisites met. A runway depth `<= 1` triggers TPM replenishment warnings.
- **Priority Tier (`P0`–`P3`)**:
  - `P0`: Critical blocker. Halts ordinary grooming until resolved.
  - `P1`: Milestone core deliverable.
  - `P2`: Secondary polish and optimization.
  - `P3`: Hygiene, documentation formatting, housekeeping.

---

## 6. Tab 5: 📊 Metrics (Kernel Vitals & Subsystem Telemetry)

The Metrics Tab reports operational telemetry, execution latencies, and cache efficiency.

### Visual Terminal Screenshot: Tab 5

![Tab 5: Kernel Telemetry & Latency Histograms](./screenshots/ui_tab5_metrics.svg)

```
Memory: 48.2 MB │ CAS Objects: 185 objects (1.4 MB) │ WAL Journal: 14.8 KB │ Uptime: 4h 12m
Token Burn Velocity: 1,420 tokens/hr │ CLI Command Throughput: 42.4 cmd/min
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
── COMMAND LATENCY HISTOGRAM (P50: 12ms │ P95: 84ms │ P99: 142ms) ──────────────────────────────────────────────
  0ms - 20ms   [████████████████████████████████████████] 84.2% (1,240 calls)
  20ms - 50ms  [██████                                  ] 11.4% (168 calls)
  50ms - 100ms [██                                      ]  3.8% (56 calls)
  100ms+       [█                                       ]  0.6% (9 calls)

── SUBSYSTEM CACHE EFFICIENCY ───────────────────────────────────────────────────────────────────────────────────
  Schema Registry Cache : 99.4% Hit Rate (1,840 hits / 11 misses)
  CAS Memory Plane Cache: 94.2% Hit Rate (4,210 hits / 260 misses)
  ZPARQL Plan Cache     : 91.8% Hit Rate (320 hits / 29 misses)
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
[r] Force Cache Prewarm │ [c] Compact Memory Buffer │ [q] Quit
```

### Datapoints & Diagnostics:
- **Latency Histogram**: Tracks execution responsiveness. A shift in the P99 latency past 250ms indicates file lock contention or heavy unindexed query traversals.
- **Cache Hit Rates**: Low hit rates (<80%) on the Schema Registry indicate redundant spec reloading. Run `zqk system check` to verify index health.

---

## 7. Tab 6: ⏱️ Sched (Scheduler Daemons & Maintenance Jobs)

The Sched Tab monitors autonomous background jobs, self-healing timers, and retention sweeps.

### Visual Terminal Screenshot: Tab 6

![Tab 6: Scheduler Daemons & Maintenance Jobs](./screenshots/ui_tab6_scheduler.svg)

```
Scheduler Daemon: RUNNING (PID: 84920) │ Worker Pool: 4 workers │ Ticks: 60/min
Active Cron Jobs: 8 registered │ Last Maintenance Sweep: 42s ago [PASS]
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  JOB ID  │ NAME                        │ INTERVAL │ LAST RUN │ DURATION │ NEXT RUN │ STATUS
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
> SCH-001 │ change_journal_compaction   │ @every 5m│ 11:40:00 │ 42ms     │ 11:45:00 │ [PASS]
  SCH-002 │ audit_aggregation           │ @every 1h│ 11:00:00 │ 124ms    │ 12:00:00 │ [PASS]
  SCH-003 │ cas_hygiene_scan            │ @every 6h│ 06:00:00 │ 310ms    │ 12:00:00 │ [PASS]
  SCH-004 │ memory_leak_watchdog        │ @every 1m│ 11:43:00 │ 12ms     │ 11:44:00 │ [PASS]
  SCH-005 │ test_matrix_prewarm         │ @every 2m│ 11:42:30 │ 88ms     │ 11:44:30 │ [PASS]
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
[Enter] Run Job Immediately │ [p] Pause/Resume Job │ [s] Restart Scheduler │ [q] Quit
```

### Datapoints & Diagnostics:
- **Daemon Health**: If the daemon displays `STOPPED`, background self-healing is suspended. Start it immediately using `zqk scheduler start`.
- **Status Badges**:
  - `[PASS]`: Execution completed within SLA and zero errors.
  - `[FAIL]`: Job failed. Selecting the row and pressing `[Enter]` displays error stack traces. Refer to runbook [`RB-SCH-001`](../runbooks/RB-SCH-001-SCHEDULER-DAEMON-TRIAGE.md).

---

## 8. Tab 7: 🧪 QA (Definition of Done & Verification Radar)

The QA Tab provides full downward traceability verification: proving every backlog item is grounded in verifiable test cases and acceptance criteria.

### Visual Terminal Screenshot: Tab 7

![Tab 7: QA Done-Gates & Verification Radar](./screenshots/ui_tab7_qa.svg)

```
DoD Compliance: 100% [PASS] │ Intact Lineage Chains: 159/159 │ Unbound Criteria: 0 [OK]
Test Coverage: 100% BLI Coverage (43/43 Backlog Items grounded in executable test_case objects)
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  TEST CASE ID             │ STATUS     │ LINEAGE HEALTH │ CRITERIA MET │ PROGRESS BAR
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
> TST-STORAGE-PUREGO-001   │ complete   │ ✓ INTACT       │ 1 / 1 (100%) │ [████████████████████]
  TST-ZQL-ACID-TRANSACT-01 │ complete   │ ✓ INTACT       │ 3 / 3 (100%) │ [████████████████████]
  TST-ZPARQL-PLANNER-002   │ complete   │ ✓ INTACT       │ 2 / 2 (100%) │ [████████████████████]
  TST-COMMUNITY-FIRST-RUN  │ active     │ ✓ INTACT       │ 1 / 1 (100%) │ [████████████████████]
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
── TRACEABILITY RADAR: [TST-STORAGE-PUREGO-001] ─────────────────────────────────────────────────────────────────
  Goal        : [🟢 GOAL-STORAGE-MODERNIZATION-PUREGO]
  Requirement : [🟢 REQ-STORAGE-PUREGO-EMBEDDED-001]
  Backlog Item: [🟢 BLI-STORAGE-PUREGO-EMBEDDED-001]
  Test Target : pkg/storage/purego_index_test.go:TestPureGoIndex (integration)
  Bound Criteria: [🟢 CRIT-STORAGE-PUREGO-EMBEDDED-001] (satisfied)
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
[t] Run Test Suite │ [Enter] Inspect Criteria Details │ [r] Rescan DoD Matrix │ [q] Quit
```

### Datapoints & Diagnostics:
- **DoD Compliance Score**: Percentage of active work units with closed acceptance criteria. Projects cannot be released if DoD compliance is below 100%.
- **Lineage Health (`✓ INTACT` vs `✗ BROKEN`)**:
  - `✓ INTACT`: Fully grounded from `Goal` ➔ `Requirement` ➔ `BLI` ➔ `TestCase` ➔ `Criteria`.
  - `✗ BROKEN`: Missing parent requirement or floating test case. Fails VDS Done-Gate.

---

## 9. Tab 8: 🛡️ Health (Kernel Storage & Membrane Integrity)

The Health Tab reports storage plane consistency, filesystem watcher health, and lock state.

### Visual Terminal Screenshot: Tab 8

![Tab 8: Kernel Storage & Membrane Integrity](./screenshots/ui_tab8_health.svg)

```
Knowledge Kernel State: HEALTHY │ Storage Backend: file (hybrid CAS) │ Lock Contention: 0 deadlocks
Membrane Isolation: Mode B (Strict CAS Verification) │ Integrity Score: 95.4 / 100
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  SUBSYSTEM            │ HEALTH STATUS │ CAS INTEGRITY │ WAL STATE   │ DETAILS
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
> Content-Addressed DB │ [OK]          │ 185/185 valid │ 0 corrupt   │ SHA-256 digests match index
  Filesystem Watcher   │ [OK]          │ (n/a)         │ streaming   │ Inotify / kqueue listener healthy
  Scheduler Daemon     │ [OK]          │ (n/a)         │ nominal     │ 8 jobs running; 0 errors
  Membrane Check-Valve │ [OK]          │ active        │ verified    │ Zero unpromoted draft leaks
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
[Enter] Run System Diagnostic (`zqk system check`) │ [a] Run Alignment (`zqk system align`) │ [q] Quit
```

### Datapoints & Diagnostics:
- **CAS Integrity**: Confirms every hash-addressed file in `.zqk/process/` matches its payload content. A mismatch indicates disk corruption; repair immediately with runbook [`RB-CAS-001`](../runbooks/RB-CAS-001-CAS-CORRUPTION-RECOVERY.md).
- **Integrity Score**: Measures strategic alignment between committed code and documented intent.

---

## 10. The Test Verification Dashboard (`zqk test dashboard`)

For dedicated CI/CD runs and local terminal verification, `zqk test dashboard --check-dod` provides a live stream of test execution and criteria latches.

### Visual Terminal Screenshot: Test Dashboard

![Test Verification Dashboard](./screenshots/ui_test_dashboard.svg)

```
================================================================================
🚀 ZQK TEST & DEFINITION OF DONE (DoD) DASHBOARD
================================================================================
Working Set: 12 In-Flight Tests │ Regression Pool: 159 Verified Chains (Green)

▶ TST-STORAGE-PUREGO-001  [ACTIVE]  Verify Pure-Go Indexing Engine Performance
    Lineage  : [🟢 GOAL-STORAGE-PUREGO] ➔ [🟢 REQ-STORAGE-PUREGO] ➔ [🟢 BLI-STORAGE-001] ➔ [🟡 TST-STORAGE-PUREGO-001]  ✓ Chain Intact
    Target   : pkg/storage/purego_index_test.go:TestPureGoIndex (integration)
    Criteria : [🟢 CRIT-STORAGE-PUREGO-EMBEDDED-001] 
    Progress : [████████████████████] 100% (1/1 satisfied, 0 open)

▶ TST-ZQL-ACID-TRANSACT-01  [ACTIVE]  Verify Multi-Object Atomic Rollbacks
    Lineage  : [🟢 GOAL-ZQL-MUTATION] ➔ [🟢 REQ-ZQL-ACID] ➔ [🟢 BLI-ZQL-001] ➔ [🟡 TST-ZQL-ACID-TRANSACT-01]  ✓ Chain Intact
    Target   : pkg/zql/transaction_test.go:TestAtomicRollback (unit)
    Criteria : [🟢 CRIT-ZQL-STAGED-ISOLATION] [🟢 CRIT-ZQL-ROLLBACK-JOURNAL] 
    Progress : [████████████████████] 100% (2/2 satisfied, 0 open)

🛡️  REGRESSION TESTING POOL: 159 verified chains green & passing
[Pruned from active view — inspect full regression suite with: zqk test dashboard --view regression]

📡 RECENT CRITERIA SATISFACTION & SHOCKWAVE EVENTS
────────────────────────────────────────────────────────────────────────────────
  ⚡ [11:42:01] CRITERION SATISFIED: CRIT-STORAGE-PUREGO-EMBEDDED-001
  ⚡ [11:42:01] TEST CASE TST-STORAGE-PUREGO-001: active -> complete
  ⚡ [11:42:01] TRACEABILITY CHAIN GRADUATED: TST-STORAGE-PUREGO-001 ➔ Moved to Regression Pool
  ⚡ [11:42:02] SHOCKWAVE PROPAGATED: Latch complete on requirement REQ-STORAGE-PUREGO
────────────────────────────────────────────────────────────────────────────────
```

### Key Elements & Interpretation:
1. **Target Line**: Specifies exact Go test function and execution scope (`unit`, `integration`, `e2e`).
2. **Criteria Pills (`[🟢 CRIT-*]`)**: Real-time representation of acceptance criteria. Green indicates verified; yellow indicates currently testing; white indicates unverified.
3. **Chain Graduation**: When all criteria in a test case are green, the entire chain automatically graduates into the **Regression Testing Pool**, preventing test output noise.

---

## 11. Visual Web Studio (`zqk ui -w` at http://127.0.0.1:8080)

When launched with the `-w` flag (`zqk ui -w`), ZQK starts a zero-dependency HTTP server providing interactive visual timelines, Gantt matrices, and DAG graphs.

### Web Studio Layout & Visual Components:

![Visual Web Studio: Timeline, Gantt Roadmap & DAG Visualizer](./screenshots/ui_web_studio.svg)

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ ⚡ ZQK STUDIO  │  Timeline & Gantt  │  Ontology DAG Visualizer  │  System Health  │  Port: 8080        │
├────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                        │
│  TIMELINE & GANTT ROADMAP                                                                              │
│                                                                                                        │
│  Sep 25                   Sep 28                   Today (Sep 29)           Oct 02             Oct 05  │
│  ────────┼────────────────────────┼───────────────────────▼────────────────────────┼────────────────── │
│                                                           │                                            │
│  WS-LAUNCH (Launch Readiness)                             │                                            │
│  ├─ [◆ MIL-COMMUNITY-LAUNCH]                              │                                            │
│  │  └─ [■ PRI-STARTER-COMMUNITY-001]                      │                                            │
│  │     └─ [■ BLI-001] Community launch testing            │  (Active Shovel-Ready)                     │
│  │                                                        │                                            │
│  WS-KERNEL (Core Subsystems)                              │                                            │
│  ├─ [◆ MIL-PUREGO-STORAGE]                                │                                            │
│  │  └─ [■ PRI-STORAGE-PUREGO-001] ════════════════════════╡ (Complete)                                 │
│                                                           │                                            │
│  STATUS LEGEND:  ■ Planned   ■ In Progress   ■ Blocked   ■ Complete   ◆ Milestone Marker               │
│                                                                                                        │
├────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                        │
│  ONTOLOGY DAG DEPENDENCY GRAPH                                                                         │
│                                                                                                        │
│  [GOAL-STARTER] ──► [REQ-LAUNCH-DOCS] ──► [BLI-001] ──► [TST-COMMUNITY-001] ──► [CRIT-DOCS-VERIFIED]   │
│         │                                                                                              │
│         └─────────► [REQ-AIRGAP-BUILD] ──► [BLI-002] ──► [TST-AIRGAP-REPRO]  ──► [CRIT-REPRO-TARBALL]  │
│                                                                                                        │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Visual Web Components:
1. **Vertical "Today" Line**: Anchors the timeline directly over today's date, displaying which milestones are in the past, active right now, or scheduled in the future.
2. **Workstream Bands (`WS-*`)**: Swimlanes grouping related priority plans and backlog items.
3. **Horizontal Status Legend**:
   - `■ Planned` (Gray/Blue): Shovel-ready work ready to be claimed.
   - `■ In Progress` (Amber): Work actively claimed by an operator or AI agent.
   - `■ Blocked` (Red): Work stalled by unmet preconditions or criteria.
   - `■ Complete` (Emerald Green): Verified and merged into canonical state.
   - `◆ Milestone` (Cyan Diamond): Key strategic delivery gate.
4. **Interactive DAG Graph**: Click any node to open the side drawer displaying full schema attributes, CAS hash digests, bound test cases, and cryptographic parent lineage.

---

## 12. Operator Quick Cheatsheet

| Command | Action | Primary Output |
| :--- | :--- | :--- |
| `zqk ui` | Launch full Mission Control TUI console | 8-tab interactive terminal interface |
| `zqk ui -w` | Start local visual Web Studio | Web browser at `http://127.0.0.1:8080` |
| `zqk ui --tab qa` | Launch directly into QA DoD radar | Vitals card, test matrix, and criteria bars |
| `zqk test dashboard` | Terminal test verification dashboard | Active test cards and WAL shockwave stream |
| `zqk test dashboard --check-dod` | Verify 100% Definition of Done | Fails closed (exit code 1) if criteria open |
| `zqk state stream --dashboard` | Stream real-time mutations | Visual ANSI seismograph with auto-scroll |
| `zqk object inspect <kind> [id]` | Deep object inspection | TUI radar, CAS profile, and Policy Studio |
