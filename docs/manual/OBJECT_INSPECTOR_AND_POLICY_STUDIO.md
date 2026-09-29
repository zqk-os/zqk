# Manual: Object Inspector & Policy Studio Reference

This reference manual documents the architecture, visual displays, CLI commands, interaction models, and configuration flags for the **ZQK Object Inspector (`zqk object inspect`)**, **Policy Rule Studio**, and **Object Relationship & Mutation Engines**.

---

## 1. Overview & Dual-Audience Architecture

The ZQK Object Inspector solves the tension between machine density and human ergonomics when interacting with the Knowledge Kernel:

```
                      ┌─────────────────────────────────────────┐
                      │          Knowledge Kernel CAS           │
                      │  (Objects, Traits, Lineage, Policies)   │
                      └────────────────────┬────────────────────┘
                                           │
                    ┌──────────────────────┴──────────────────────┐
                    ▼                                             ▼
       ┌────────────────────────┐                    ┌────────────────────────┐
       │   Machine Projection   │                    │    Human TDS Console   │
       │   `--format json`      │                    │     `--format table`   │
       ├────────────────────────┤                    ├────────────────────────┤
       │ • Reduced semantic JSON│                    │ • TDS Panels & Tables  │
       │ • Essential attributes │                    │ • Vim Nav (j/k/g/G)    │
       │ • Compact lineage IDs  │                    │ • Dynamic Message Line │
       │ • Zero token bloat     │                    │ • Deep Drill-Down ([⏎])│
       │ • Machine parseable    │                    │ • Policy Studio ([p])  │
       └────────────────────────┘                    └────────────────────────┘
```

---

## 2. Main Object Inspector View (`zqk object inspect`)

When launched without a single ID, `zqk object inspect [kind]` displays the full-screen terminal scoreboard:

### Visual Terminal Screenshot: Main Inspector Table

![Object Inspector Table](./screenshots/ui_object_inspector_table.svg)

### Visual Component Breakdown:

| Visual Element | User Action / Keybinding | Description & Purpose |
| :--- | :--- | :--- |
| **Kind Selector** | `[Tab]` / `[Shift+Tab]` | Cycles between registered ontology kinds (`backlog_item`, `requirement`, `criteria`, `test_case`, `priority_plan`, `goal`, `policy`). |
| **Filter Pills** | `[f]` | Toggles between active view filters: `[ALL]`, `[ACTIVE]`, `[DRAFT]`, `[BLOCKED]`, `[COMPLETE]`, and `[MINE]`. |
| **Sort Key** | `[s]` | Cycles sort order: `updated_at`, `created_at`, `id`, `priority`, `status`, `title`. |
| **Selection Cursor** | `[j]/[k]` or `[↓]/[↑]` | Moves the highlighted row cursor `>`. |
| **Search Buffer** | `[/]` | Filters rows interactively in real time. Use `[n]`/`[N]` to cycle through matches. |

---

## 3. Deep Object Inspection Modal (`[Enter]`)

Pressing `[Enter]` on any row opens the deep inspection modal, decomposing raw YAML into standardized visual modules:

### Visual Terminal Screenshot: Deep Inspection Modal

![Object Inspector Console](./screenshots/ui_object_inspector.svg)

### Datapoints & Module Details:
1. **Module 1 (Lineage Radar)**: Traces the upward root chain (`Goal` ➔ `Requirement`) and downward bindings (`TestCase` ➔ `Criteria`). The integrity badge confirms whether the object is valid for release.
2. **Module 2 (CAS Storage & Provenance)**: Displays Content-Addressed Storage SHA-256 hash, storage plane, byte footprint, and cryptographic mutator audit trail.
3. **Module 3 (Action Palette)**: Allows immediate execution of lifecycle transitions and reference mutations directly from the keyboard without exiting the inspector.

---

## 4. Live Policy Rule Studio (`[p]` or `--policy-studio`)

Writing validation rules in raw YAML is error-prone. The **Policy Rule Studio** turns policy authoring and validation testing into an interactive, fail-closed IDE experience.

### Visual Terminal Screenshot: Policy Studio

![Policy Rule Studio](./screenshots/ui_policy_studio.svg)

### Hotkeys & Workflows:
- `[c]`: Enter **DSL Edit Mode**. Type predicate conditions with syntax checking.
- `[Tab]`: Trigger context-aware autocompletion for schema fields and enum values.
- `[t]`: Run immediate dry-run evaluation across all repository objects without disk mutations.
- `[s]`: Save and promote policy to `.zqk/process/policy/`.

---

## 5. Modern vs. Legacy Mutation Commands Reference

Always prefer first-class, schema-aware CLI verbs over unstructured scalar `--field` modifications.

### Comparison Table:

| Operation | 🌟 Preferred Modern Command | ⚠️ Legacy / Unstructured Fallback | Rationale & Safety |
| :--- | :--- | :--- | :--- |
| **Add References** | `zqk object ref add <SRC> <TARG>` | `zqk object update <SRC> --field "<k>_refs=<ID>"` | Validates target existence, deduplicates, prevents cycles. |
| **Remove References** | `zqk object ref remove <SRC> <TARG>` | `zqk object update <SRC> --field "<k>_refs=..."` | Safely prunes graph edges without array parsing syntax bugs. |
| **Promote Lifecycle** | `zqk object promote <ID>` | `zqk object update <ID> --field status=<ST>` | Enforces FSM check-valves, criteria latches, and emits shockwaves. |
| **Demote Lifecycle** | `zqk object demote <ID>` | `zqk object update <ID> --field status=<ST>` | Validates backward transition rules and dependency unlinking. |
| **Park Object** | `zqk object park <ID>` | `zqk object update <ID> --field status=parked` | Validates graceful retirement along an intentional exit path. |
| **Scalar Field Edits** | `zqk object update <ID> --field k=v` | Raw file editing on disk | Intended strictly for scalar fields (`title`, `description`, `body`). |

---

## 6. CLI Command Flags

```bash
# Full interactive TUI
zqk object inspect backlog_item

# Inspect specific object instance
zqk object inspect backlog_item BLI-001

# Autonomous agent semantic projection (token-efficient JSON)
zqk object inspect backlog_item BLI-001 -f json

# Launch directly into Policy Studio
zqk object inspect backlog_item --policy-studio
```

| Flag | Short | Default | Description |
| :--- | :---: | :---: | :--- |
| `--kind` | `-k` | `backlog_item` | Target object kind to inspect. |
| `--id` | | `""` | Optional specific object ID to inspect directly. |
| `--fields` | | `""` | Comma-separated field projections (e.g. `id:30,title:40`). |
| `--filter` | | `""` | Filter predicate expression (e.g. `status=active`). |
| `--sort-by` | | `updated_at` | Field key to sort objects by. |
| `--sort-asc`| | `false` | Sort in ascending order (default: descending). |
| `--policy-studio`| `-p` | `false` | Launch directly into live Policy Rule Studio. |
| `--format` | `-f` | `table` | Output format (`table`, `json`, `jsonl`, `yaml`). |
