# CLI Command Aliases, Auto-Fix Recipes, and Template Nullability

## Overview

This guide documents CLI ergonomics standards, command alias parity, auto-fix recipe syntax conventions, and template reference field nullability rules across the Knowledge Kernel CLI and tooling runtime.

---

## 1. Object Minting Aliases (`zqk new <kind>`)

The Knowledge Kernel CLI provides ergonomic short aliases for common core kinds under `zqk new`:

| Kind | Aliases | Example Usage |
| :--- | :--- | :--- |
| `priority_plan` | `plan`, `pri` | `zqk new pri --title "Storage Elevation Plan"` |
| `requirement` | `req` | `zqk new req --title "Fast Search"` |
| `criteria` | `crit` | `zqk new crit --title "Sub-15ms Latency"` |
| `test_case` | `tst`, `tc` | `zqk new tst --title "Verify Latency"` |
| `backlog_item` | `bli` | `zqk new bli --title "Index optimization"` |
| `milestone` | `mil` | `zqk new mil --title "v0.4 Release"` |
| `technical_debt` | `td`, `techdebt` | `zqk new td --title "Refactor cache lock"` |
| `vision` | `vis` | `zqk new vis --title "Autonomous Dev"` |
| `mission` | `mis` | `zqk new mis --title "Reliable Swarms"` |
| `workstream` | `ws` | `zqk new ws --title "Core Subsystems"` |
| `question` | `que` | `zqk new que --title "Architecture Decision"` |
| `decision` | `dec` | `zqk new dec --title "CAS Storage Model"` |

All aliases support identical relational and descriptor flags (`--title`, `--description`, `--content`, `--plan`, `--req`, `--goal`, `--bli`, `--criteria`, `--cas`, `--promote`).

---

## 2. Auto-Fix Recipe Command Syntax

When `zqk system check` identifies recoverable schema, reference, or lifecycle issues, deterministic remedy recipes provide actionable CLI commands to repair the anomaly.

### Positional ID Convention
Commands output canonical positional arguments for single-object mutations rather than legacy or invalid `--ids` flags:

- **Object Update**: `zqk object update <ID> --field <FIELD>=<VALUE>`
- **Object Deletion**: `zqk object delete <ID> --unlink-references`
- **System Check Auto-Fix**: `zqk system check --auto-fix <ID>`

The auto-fix engine sanitizes generated recipe strings, stripping any extraneous `--ids` flags to prevent execution failures when operators copy-paste remedy recipes.

---

## 3. Template Scaffolding & Reference Field Nullability

### Problem Statement
In earlier iterations, object templates and draft-plane merges populated optional reference attributes (e.g., `release_ref: ""`, `team_configuration_ref: ""`, `priority_plan_ref: ""`) with empty string scalars (`""`). Because the ontology defines references as non-empty entity pointers, empty string values violated ontology invariants during subsequent draft validation.

### Clean Scaffolding Protocol (GH-944)
1. **Omission on Draft Land**: Optional reference fields (`field.Type == "reference"` or `field.SemanticType == "reference"`, and scalar `*_ref` / `*_id`) are omitted from initial draft skeletons unless explicitly provided by the operator.
2. **Explicit Nullability (`null`)**: Where placeholders are required for lifecycle progression (such as `priority_plan_ref` on `backlog_item`), templates scaffold explicit `null` (`nil`) rather than empty strings `""`.
3. **Validator Permissiveness**: Core schema and ontology validators (`GoValidator`, `InstanceValidator`, and `OntologyRegistry`) treat `null` and omitted attributes as valid unpopulated states for optional reference fields, eliminating false-positive validation rejections.

---

## 4. Object Discovery Symmetry & Traceability Alignment

### Object List Flag and Argument Parity
`zqk object list` supports both positional kinds and the `--kind` flag interchangeably:
- Positional kind: `zqk object list requirement`
- Flag kind: `zqk object list --kind requirement`

The CLI validates kinds strictly against the ontology registry before dispatching storage queries, ensuring early, helpful errors on typos.

### Strategic Traceability Propagation
When generating execution spines via `zqk workflow gen-trace-pipeline <REQ-ID>`, the generator automatically extracts all `goal_refs` from the parent requirement and propagates them to all child `backlog_item` objects. This guarantees end-to-end strategic alignment across the 5-layer cascade without manual reference linking.

