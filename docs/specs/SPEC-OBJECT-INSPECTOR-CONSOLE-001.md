# Technical Specification: ZQK Interactive Object Inspector Console & Policy Studio

**Document ID:** `SPEC-OBJECT-INSPECTOR-CONSOLE-001`  
**Governing Goal:** `GOAL-OBJECT-INSPECTOR-001` (Universal Interactive Object Inspection, Semantic Agent Projection, and Live Policy Studio)  
**Governing Requirement:** `REQ-OBJECT-INSPECTOR-001` (Interactive Object Inspector Console and Live Policy Rule Studio)  
**Governing Priority Plan:** `PRI-OBJECT-INSPECTOR-GROOM` (Grooming: Interactive Object Inspector Console & Policy Studio)  
**Scaffolded Lineage Pipeline:** Scaffolding executed via canonical `zqk workflow gen-trace-pipeline` (POL-AGENT-TPM-TRACE-PIPELINE-001)  
- **Governing Backlog Item:** `BLI-1790193716463242000-a2b08acb` (`Implement: Interactive Object Inspector Console and Live Policy Rule Studio`)  
- **Governing Test Case:** `TST-1790193716463242001-ef989579` (`Test Suite: Interactive Object Inspector Console and Live Policy Rule Studio`)  
- **Multi-Criteria Ratio (3 Facets):**  
  1. `CRIT-1790193716463242000-777a8136` (Functional Acceptance: Object Inspector TUI & CLI Projections)  
  2. `CRIT-1790193716463243000-383a63c4` (Boundary & Error Handling: Invalid kinds, unparseable filters, security role rejection)  
  3. `CRIT-1790193716463244000-e006648d` (Integration & Conformance: Policy Studio DSL auto-complete & repository dry-run evaluation)  
**Authoring Personas:** `PER-CHIEF-ARCHITECT`, `PER-UI-UX-DESIGNER`, `PER-INFORMATION-ARCHITECT`  
**Status:** Canonical / Approved Architecture Spec  

---

## 1. Executive Summary & Vision

The **ZQK Knowledge Kernel** models all system artifacts (requirements, backlog items, priority plans, test cases, criteria, policies, prompts, personas, telemetry records) as discrete, typed objects governed by schema contracts, CAS storage, and lifecycle state machines.

Currently, human developers and autonomous agents inspect objects using:
1. `zqk object list [kind] --fields id:30,title:40,status:12 --filter ... --sort-by ...`
2. `zqk object get [kind] [id] -f yaml`

While powerful and scriptable, these existing surfaces impose high cognitive friction:
- **Human Friction:** Memorizing column widths, quoting filter arguments, parsing massive raw YAML dumps cluttered with transport boilerplate (`metadata`, `etag`, `storage_profile`, `created_at_epoch`).
- **Agent Friction:** Token waste ingesting noisy unstructured YAML payloads when only essential attributes, relations, and current lifecycle state are needed.
- **Governance Friction:** Writing and validating policy constraints and CPCP boundary rules in text files without live dry-run validation against repository state.

The **ZQK Object Inspector (`zqk object inspect` / `zqk ui --tab inspector`)** delivers:
1. A **Terminal Design System (TDS)** full-screen console for browsing any kernel kind with responsive keyboard navigation, instant status filtering, column cycling, and master-detail splits.
2. **Modular Property Modules**: Canonical rendering components for common complex structures (Lineage & Traceability Radar, CAS Storage Hygiene, Ontology Linkages, Role Entitlements).
3. **Role & Account Gated Action Palette**: Interactive state transitions, work claiming, and editor hooks gated by the active `SecurityContext`.
4. **Live Policy Rule Studio**: An interactive visual builder for validation DSL expressions with field auto-completion from `FieldRegistry` and real-time dry-run impact evaluation across active kernel objects.
5. **Dual Machine/Human Architecture**: Full-color ANSI TUI for human pair programming, paired with reduced semantic JSON/YAML projections (`--format json`) for autonomous agents.

---

## 2. Architecture & Domain Grammar

### 2.1 CLI Taxonomy & Command DNA
Conforming to `docs/architecture/CLI_COMMAND_TAXONOMY_STANDARDS.md`:

```bash
# Standalone CLI Entry Point
zqk object inspect [kind] [id] [flags]

# Aliases
zqk inspect [kind] [id]
zqk object browser [kind]
```

#### Command Flags & Invariants:
| Flag | Short | Type | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `--kind` | `-k` | `string` | `""` | Initial object kind to browse (e.g. `backlog_item`, `policy`) |
| `--id` | | `string` | `""` | Jump directly into modal inspection for a specific object ID |
| `--filter` | | `stringArray`| `[]` | Filter expressions (`status=active`, `priority_tier=P0`) |
| `--sort-by` | | `string` | `updated_at` | Sort field |
| `--sort-asc` | | `bool` | `false` | Sort ascending (default is descending/newest first) |
| `--policy-studio` | | `bool` | `false` | Launch directly into the Policy Rule Studio |
| `--format` | `-f` | `string` | `table` | `table` (interactive TUI), `json`, `yaml`, `semantic-link` |

---

## 3. Terminal User Interface (TUI) Layout & Interaction Model

### 3.1 Master-Detail Layout Specification

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ 🔍 ZQK OBJECT INSPECTOR — [Kind: backlog_item] (28 items)                    │
├──────────────────────────────────────────────────────────────────────────────┤
│ Filters: [All Status] [P0/P1] [My Claimed]  │ Sort: [updated_at ▼]  │ Page: 1/3│
├──────────────────────────────────────────────────────────────────────────────┤
│   ID             PRIO  STATUS       OWNER         TITLE                      │
│ ▶ BLI-001        P0    in_progress  agent-alpha   Implement TUI 7-Tab View   │
│   BLI-002        P1    planned      unassigned    Idle I/O Stamp Skip Opt    │
│   BLI-003        P2    blocked      agent-beta    Refactor CAS Gate Mutex    │
├──────────────────────────────────────────────────────────────────────────────┤
│ ─── INSPECTED OBJECT: BLI-001 (Press Enter to open Action Palette [o]) ────── │
│ Kind     : backlog_item        │ Status: 🟡 in_progress                      │
│ Title    : Implement TUI 7-Tab View                                          │
│ Owner    : agent-alpha         │ Plan  : PRI-TPM-CONV (Swarm Convergence)    │
│ ┌─────────────────────────────┐ ┌──────────────────────────────────────────┐ │
│ │ Modular Attribute Viewer    │ │ Lineage & Traceability Radar             │ │
│ │ • Priority  : P0 (Critical) │ │ • Root: [goal GOAL-01] World-Class UX    │ │
│ │ • Category  : feature       │ │ • Req : [req REQ-012] TUI Seismograph    │ │
│ │ • Story Pts : 3 SP          │ │ • Test: [test TST-030-01] 3/3 Passing ✓   │ │
│ └─────────────────────────────┘ └──────────────────────────────────────────┘ │
│ Navigation: [Tab] Switch Kind │ [/] Search │ [f] Filter │ [e] Edit │ [Esc] Back │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Navigation & Keyboard Shortcuts
- `Tab` / `Shift+Tab`: Cycle through registered kinds (`backlog_item` ➔ `requirement` ➔ `policy` ➔ `test_case`...).
- `j` / `↓` and `k` / `↑`: Move active row cursor (`▶`).
- `f`: Open quick status filter pill selector (`[All]`, `[Active]`, `[Draft]`, `[Blocked]`, `[Done]`).
- `/`: Open inline search prompt (instant regex / substring matching against ID and Title).
- `s`: Cycle sort order (`updated_at` ➔ `created_at` ➔ `id` ➔ `priority`).
- `Enter`: Open the **Role-Gated Action Palette**.
- `e`: Open full object definition in `$EDITOR` (temporary YAML crossing gate).
- `q` / `Esc`: Exit inspector (or close active modal overlay).

---

## 4. Modular Display Modules

The Inspector decomposes verbose raw YAML into standardized TDS visual modules:

### 4.1 Module 1: Lineage & Traceability Radar
- Dynamically queries upward references (`priority_plan_ref`, `goal_refs`, `requirement_refs`) and downward bindings (`test_case`, `criteria`).
- Renders the end-to-end chain:
  `[Goal] ➔ [Requirement] ➔ [Backlog Item] ➔ [Test Case] ➔ [Criteria]`
- Displays integrity badge: `[✓ INTACT]` or `[✗ BROKEN: missing root]`.

### 4.2 Module 2: CAS Storage & Cryptographic Hygiene
- Renders:
  - CAS content address hash / SHA-256 digest.
  - Storage plane: `draft` vs `authoritative` (CAS master).
  - Storage footprint: bytes, permissions (`0644`), line count.
  - ETag revision and last mutation timestamp.

### 4.3 Module 3: Security & Role-Gated Action Palette (`[Enter]`)
Evaluates `proc.SecurityContext()` against the target object:
- **Status Transition**: Lists only valid next states defined in the object's lifecycle state machine.
- **Work Claiming**: Assigns or unassigns `claimed_by` to the active caller account.
- **Draft Promotion**: Calls `storage.Promote` if the object resides in the draft plane.
- **Authorization Guard**: Unauthorized options are greyed out with an explanatory badge (e.g. `[Requires Role: Admin]`).

---

## 5. Live Policy Rule Studio (`--policy-studio`)

### 5.1 Concept & Workflow
Writing validation policies in raw YAML is error-prone. The **Policy Rule Studio** turns policy creation and testing into an interactive, fail-closed IDE experience.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ 🛡️  ZQK POLICY RULE STUDIO — [Policy: POL-SAFETY-001]                        │
├──────────────────────────────────────────────────────────────────────────────┤
│ Rule Target Kind : [ backlog_item ]                                          │
│ Field Expression : [ status == "complete" => criteria.all_satisfied == true ]│
├──────────────────────────────────────────────────────────────────────────────┤
│ ─── LIVE DRY-RUN EVALUATION MATRIX (Across 28 repository objects) ─────────── │
│ PASS: 26 objects (92.8%) │ FAIL: 2 objects (7.2%)                            │
│                                                                              │
│ Violations:                                                                  │
│ • [BLI-014] "Refactor CAS Gate" (status=complete, criteria open: CRIT-022)   │
│ • [BLI-021] "Update Auth Creds" (status=complete, criteria open: CRIT-049)   │
├──────────────────────────────────────────────────────────────────────────────┤
│ [c] Edit Condition │ [t] Test Expression │ [s] Save Policy │ [Esc] Close     │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Autocompletion Engine
- Utilizes `objects.GetGlobalFieldRegistry()`:
  - Typing `status` suggests enum values: `["draft", "planned", "in_progress", "complete", "blocked"]`.
  - Typing `priority` suggests valid tiers: `["P0", "P1", "P2", "P3"]`.
- Predicate operator selector: `==`, `!=`, `>`, `<`, `in`, `matches_regex`, `all_satisfied`.
- Instant evaluation engine evaluates rules using `pkg/when` predicate evaluators without modifying repository files.

---

## 6. Dual Human & Machine-Readable Interfaces

### 6.1 Human Interface
Interactive full-screen ANSI TUI rendering through `tds.Panel`, `tds.NewTable`, and `tds.StatRow`.

### 6.2 Agent / Machine Interface (`--format json`)
Autonomous agents calling `./bin/zqk object inspect <kind> [id] -f json` receive a streamlined, high-signal projection:

```json
{
  "kind": "backlog_item",
  "id": "BLI-001",
  "status": "in_progress",
  "priority": "P0",
  "title": "Implement TUI 7-Tab View",
  "claimed_by": "agent-alpha",
  "lineage": {
    "goal": "GOAL-STARTER-COMMUNITY-001",
    "requirement": "REQ-STARTER-COMMUNITY-012",
    "test_cases": ["TST-STARTER-COMMUNITY-030-01"],
    "is_intact": true
  },
  "criteria_summary": {
    "total": 3,
    "satisfied": 3,
    "pending": 0
  },
  "actions_available": [
    "transition_status",
    "unclaim",
    "edit_properties"
  ]
}
```

---

## 7. Delivery Plan & Verification Gates

1. **Phase 1: Foundation & Command DNA (BLI-OBJECT-INSPECTOR-001)**
   - Declarative spec `.zqk/cli/specs/object/inspect_command.yaml`.
   - Command builder generated or wired in `cmd/zqk/object/inspect.go`.
2. **Phase 2: Interactive TUI Model & Views (BLI-OBJECT-INSPECTOR-002)**
   - Kind and field auto-discovery from `FieldRegistry`.
   - Interactive table with quick filters, search, and detail modal.
3. **Phase 3: Modular Property Cards (BLI-OBJECT-INSPECTOR-003)**
   - Lineage Radar, Storage Profile, and Role-Gated Action Palette.
4. **Phase 4: Policy Rule Studio (BLI-OBJECT-INSPECTOR-004)**
   - Autocomplete DSL builder and repository-wide live dry-run evaluator.
5. **Phase 5: Automated Verification Gate (CRIT-OBJECT-INSPECTOR-001 / TST-OBJECT-INSPECTOR-001)**
   - Complete unit and integration test suite asserting dual human/machine output and 0 vet/lint defects.
