# Manual: Object Inspector & Policy Studio Reference

This reference manual documents the architecture, CLI commands, interaction models, and configuration flags for the **ZQK Object Inspector (`zqk object inspect`)**, **Policy Rule Studio**, and **Unified Mission Control QA Console**.

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

## 2. CLI Command Reference

### `zqk object inspect [kind] [id] [flags]`

Inspects object instances in the Knowledge Kernel with interactive TUI navigation or machine-readable JSON output.

#### Syntax & Invocations:
```bash
# 1. Full interactive TUI shell starting at a given kind
zqk object inspect backlog_item

# 2. Inspect a specific object instance (TDS panel format)
zqk object inspect backlog_item BLI-001

# 3. Autonomous Agent Semantic Projection (reduced token JSON)
zqk object inspect backlog_item BLI-001 -f json

# 4. Filter and select specific fields
zqk object inspect backlog_item --fields id:30,title:40,status:12 --filter "status=in_progress"

# 5. Launch directly into Policy Rule Studio dry-run mode
zqk object inspect backlog_item --policy-studio -f json
```

#### Flags:
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

---

## 3. Interactive TUI Keybindings

When launched without a specific single-object ID, `zqk object inspect` opens the full-screen terminal console:

### Navigation & Views
- `[j] / [k]` or `[↓] / [↑]`: Move selection cursor down / up.
- `[g]`: Jump to top (or toggle to bottom if already at top).
- `[G]`: Jump to bottom (or toggle to top if already at bottom).
- `[Tab]`: Cycle active kind forward through registered ontology schemas.
- `[f]`: Cycle filter pills (`[ALL]`, `[ACTIVE]`, `[DRAFT]`, `[BLOCKED]`, `[COMPLETE]`, `[MINE]`).
- `[s]`: Cycle sort keys (`updated_at`, `created_at`, `id`, `priority`, `status`, `title`).
- `[/]`: Open interactive search input buffer.
- `[n] / [N]`: Jump to next / previous search match.
- `[Esc]`: Clear search query or dismiss active modal.
- `[z] / [?]`: Cycle human editor profile: `newb` ➔ `pro` ➔ `jedi` (Zen mode).
- `[r]`: Force refresh object list and metadata from CAS storage.
- `[q]`: Quit application.

### Drill-Down Modal & Action Palette
- `[Enter]`: Step into the selected row's **Deep Object Inspection Modal**.
  - Displays Lineage & Traceability Radar, CAS Storage Profile, Ontology schema traits, and raw attributes.
  - `[t]`: Trigger immediate status transition to next valid lifecycle state.
  - `[a]`: Open the **Role-Gated Action Palette**:
    - `[c]`: Claim / Unclaim object for autonomous agent execution.
    - `[t]`: Transition status along lifecycle graph.
    - `[e]`: Edit object definition in `$EDITOR`.
    - `[p]`: Open Policy Rule Studio.
    - `[d]`: Delete object (role-gated: requires write/delete permissions).

---

## 4. Policy Rule Studio

The Policy Rule Studio enables live authoring and real-time evaluation of validation rules against objects in the kernel:

### Hotkeys within Policy Studio (`[p]`):
- `[j] / [k]`: Select active policy rule to inspect.
- `[c]`: Enter **DSL Edit Mode**.
  - Type validation conditions (e.g. `status == "complete" && len(criteria_refs) > 0`).
  - `[Tab]`: Autocomplete tokens sourced dynamically from `objects.GetGlobalFieldRegistry()`.
  - `[Enter]`: Commit condition to the active policy rule.
  - `[Esc]`: Cancel DSL edit mode without saving.
- `[t]`: Re-evaluate all rules in dry-run mode against active objects.
- `[Esc] / [q]`: Close Policy Studio and return to table view.

---

## 5. Mission Control QA Tab (`zqk ui --tab qa`)

Tab 7 (`🧪 QA`) in Mission Control provides full-lifecycle traceability down to test cases and acceptance criteria:

- **DoD Vitals Card**: Real-time display of Traceability DoD compliance, intact chains, unbound criteria count, criteria met percentage, and BLI test coverage.
- **Test Suites Table**: Interactive list of active test cases with status badges, lineage health (`✓ INTACT` vs `✗ BROKEN`), and criteria progress.
- **[t] Hotkey**: On-demand test matrix re-scanning without exiting the TUI.
- **[Enter] Drill-down**: Deep inspection of test case lineage, bound requirements, backlog items, and acceptance criteria.
