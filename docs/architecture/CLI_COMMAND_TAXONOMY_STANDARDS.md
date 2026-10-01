# Canonical CLI Command Taxonomy & Architectural Standards

**Document ID:** `DOC-CLI-COMMAND-TAXONOMY-STANDARDS-001`  
**Authoring Personas:**  
- `PER-INFORMATION-ARCHITECT` (Information Architect)  
- `PER-TECHNICAL-DOCUMENTARIAN` (Technical Documentarian)  
**Status:** Canonical / Enforced  

---

## 1. Executive Summary & Central Theme

The **ZQK Command Interface** is the primary human-agent operating surface for the Knowledge Kernel. As the system scales across multi-agent swarms and varied development environments, command sprawl, ambiguous verbs, un-specced commands, and vendor-specific leakage introduce cognitive fatigue, broken automation, and architectural degradation.

This document establishes the **authoritative ground rules, naming taxonomy, behavioral standards, and governance protocols** for all commands in `zqk`.

> [!IMPORTANT]
> **Zero Tolerance for New Un-specced Commands (Ratcheting Baseline Freeze):**  
> Every new or modified command in `zqk` must have a valid declarative specification in `.zqk/cli/specs/`. Active command coverage is governed by a ratcheting baseline freeze file (`.zqk/cli/command_spec_coverage_baseline.json`). Any new command added without a specification or any coverage regression is treated as a build-breaking defect and trapped by `zqk system validate-command-specs`. Legacy grandfathered commands are progressively migrated to achieve 100% parity.

---

## 2. Core Architectural Ground Rules & Standards

### Rule 1: Declarative Spec Coverage & Ratcheting Baseline Parity
1. **Spec Location:** Every command and subcommand MUST have a matching YAML specification in `.zqk/cli/specs/<domain>/<command>_command.yaml`.
2. **Spec Completeness:** Specifications must document the command purpose, positional arguments, flags (with defaults and descriptions), expected output schema, and at least two realistic usage examples.
3. **Automated Verification:** The pre-commit gate and local CI run `zqk system validate-command-specs`. Commits that introduce un-specced commands beyond the baseline freeze or introduce spec drift fail closed.
4. **Ratcheting Migration Baseline & Current Metrics:** Legacy commands that predate declarative specification enforcement are governed by `.zqk/cli/command_spec_coverage_baseline.json`. The baseline strictly prohibits any new un-specced commands (`new_drift_count == 0`), failing closed on any regressions. As grandfathered commands receive declarative specs, the baseline ratchets forward until 100% full parity (`parity: true`) is achieved:
   - **Loaded Command Inventory:** 518 commands
   - **Declaratively Specced Commands:** 255 commands
   - **Grandfathered Un-specced Commands:** 188 commands (frozen under baseline governance)
   - **New Drift Permitted:** 0 (enforced by `zqk system validate-command-specs`)

### Rule 2: Strict Domain-Resource Grammar & Noun-Verb Hierarchy
1. **Domain Hierarchy & Approved Ergonomics Shortcuts:**  
   Primary root-level commands represent distinct architectural **domains** (`system`, `agent`, `workflow`, `object`, `test`, `job`, `service`, `vendor`). Unqualified verbs are prohibited at the root level unless explicitly designated as approved universal ergonomics shortcuts:
   - **`zqk do`**: Autonomous CAP loop execution shorthand (`zqk workflow vds do`).
   - **`zqk inspect`**: Interactive TUI Object Inspector and Policy Studio shortcut (`zqk object inspect`).
   - **`zqk mutate`**: ZQL declarative mutation engine shortcut (`zqk object mutate`).
   - **`zqk query`**: ZPARQL graph query language engine shortcut (`zqk graph query`).
   - **`zqk validate`**: Invariant gate and schema validation shortcut (`zqk system validate`).
   - **`zqk rollback`**: Transaction rollback journal restoration shortcut (`zqk object rollback`).
   - **`zqk completion`**: Shell completion script generator (`zqk system completion`).
   - **`zqk sync`**: Storage CAS and P2P mesh synchronization shortcut (`zqk mesh sync`).
   - **`zqk pre-commit`**: Local pre-commit release gate and secret scan runner (`zqk system pre-commit`).
   - **`zqk learn`**: Institutional memory and operational lessons capture shortcut (`zqk agent learn`).
   - **`zqk new`**: Scaffolding wizard for new packs, adapters, and schemas (`zqk object new`).
   - **`zqk reports`**: Quality evaluation and engineering velocity reporting tool (`zqk system reports`).
   - **`zqk tray`**: macOS status bar daemon companion (`zqk service tray`).
   - **`zqk join`**: Multi-domain relational projection and graph join engine (`zqk graph join`).
2. **Hierarchical Naming:**  
   Subcommands must follow either:
   - `<domain> <resource> <verb>` (e.g. `zqk object requirement create`, `zqk job trigger list`)
   - `<domain> <verb> [resource]` (e.g. `zqk system check`, `zqk workflow whats-next`)
3. **Consistency of Common Verbs:**
   - `list`: Enumerate resources with optional filtering and pagination.
   - `get` / `show`: Retrieve a single resource by unique identifier.
   - `create`: Instantiate a new resource on the draft plane.
   - `update`: Mutate fields of an existing resource.
   - `delete`: Permanently or soft-delete a resource.
   - `promote` / `demote`: Transition lifecycle status along valid state machine hops.

### Rule 3: Pluggable Vendor Decoupling & Adapter Isolation
1. **Vendor Neutrality:** Core commands (`system`, `workflow`, `object`, `job`, `agent`) must remain 100% vendor-agnostic. No references to proprietary vendor extensions (e.g., Cursor IDE, VSCode, Gemini CLI, Claude Desktop) may exist in core commands or specs.
2. **Vendor Namespace:** Vendor-specific integrations, AppleScript terminal paste scripts, and IDE-specific shims must reside under:
   - `zqk vendor <vendor-name> ...` (for CLI surfaces)
   - `zqk mcp adapter <vendor-name> ...` (for MCP surfaces)
3. **Decoupled Packaging:** Code implementing vendor adapters must reside in isolated packages (e.g. `pkg/adapters/<vendor>`), preventing vendor SDKs or protocol quirks from bleeding into the kernel.

### Rule 4: CLI Command DNA & Behavioral Protocol
All commands must implement the standard **Command DNA**:
1. **Structured Logging (POL-CODE-007):** Every command execution must emit structured logs with stable event keys, execution durations, and contextual fields (seat ID, project root, exit code).
2. **Standard Output Channels:**
   - **`stdout`:** Dedicated strictly to deterministic, machine-readable data (JSON, YAML, or structured tables).
   - **`stderr`:** Dedicated strictly to human status lines, animated spinners, progress counters, warnings, and error diagnostics.
3. **Universal Flag Support:**
   - `--format [json|yaml|table|stream]`: Governs data serialization.
   - `--project-root <path>`: Allows explicit workspace scoping.
   - `--dry-run`: Validates execution without applying mutations.
   - `--context [ai-agent|human|debug]`: Informs output formatting and verbose progress.
4. **Async Progress Protocol:** Any command with execution latency >200ms must integrate `cli.BindAsyncProgress` to stream heartbeat pulses and prevent client timeouts.

### Rule 5: Deprecation & Sunsetting Protocol
1. **Deprecation Notice:** Commands marked for deprecation must be annotated in their command spec:
   ```yaml
   deprecated: true
   deprecation_message: "zqk pplan is deprecated. Use 'zqk object priority_plan' instead."
   ```
2. **Graceful Migration:** Deprecated commands must continue functioning for at least one minor release cycle, outputting a clear deprecation warning on `stderr` with the replacement syntax.
3. **Dead Code Purging:** Abandoned prototypes (e.g., `pkg/cliexamples`), superseded commands (`app use`), and lingering scratch files must be purged immediately once replacements are promoted.

---

## 3. Canonical Domain Taxonomy Matrix

| Domain Namespace | Primary Responsibilities | Example Commands |
| :--- | :--- | :--- |
| **`zqk init`** | Zero-friction greenfield project knowledge kernel initialization | `init`, `init my-project` |
| **`zqk run`** | Portable multi-agent swarm package execution (local or remote Git) | `run swarm.yaml`, `run https://github.com/org/swarm` |
| **`zqk ui`** | Interactive full-screen terminal mission control console (state, audit, swarm, pm, metrics, scheduler) | `ui`, `ui --tab pm`, `ui --tab metrics` |
| **`zqk state`** | Knowledge kernel state graph inspection, audit journals, time-series telemetry, and live streaming | `state stream --dashboard`, `state tree`, `state journal`, `state tsdb` |
| **`zqk grep`** | In-process trigram and AST code search | `grep "Pattern" pkg/`, `grep --ast "func Test*"` |
| **`zqk system`** | Host environment, kernel health, resource hygiene, spec validation, initialization | `system check`, `system resource-hygiene`, `system init`, `system validate-command-specs` |
| **`zqk workflow`** | Process flow, next-action discovery, pipeline generation, VDS gating | `workflow whats-next`, `workflow gen-trace-pipeline`, `workflow vds evaluate` |
| **`zqk object`** | Full CRUD, relationship traversal, draft plane promotion across all kernel objects | `object get <id>`, `object list <kind>`, `object create <kind>`, `object promote <id>` |
| **`zqk agent`** | Multi-agent swarm orchestration, seating, task claims, execution delegation | `agent claim-work`, `agent orchestrate`, `agent prepare-context` |
| **`zqk test`** | Test execution, requirement-to-test verification, test suite dashboards | `test run <tst-id>`, `test dashboard`, `test matrix` |
| **`zqk job`** | Background scheduler job triggers, queues, history, and status | `job list`, `job trigger <id>`, `job status <id>` |
| **`zqk service`** | Persistent daemon lifecycles, MCP service supervise, socket management | `service start`, `service status`, `service stop` |
| **`zqk vendor`** | Isolated third-party adapters and IDE integration shims | `vendor cursor paste`, `vendor vscode register` |
| **`zqk ambient`** | Ambient filesystem monitoring, metric waves, telemetry capture | `ambient wave`, `ambient status` |

---

## 4. Persona Governance & Roles

### Information Architecture Persona (`PER-INFORMATION-ARCHITECT`)
- **Mission:** Guard structural clarity, logical hierarchy, intuitive taxonomy, and domain separation across CLI surfaces and kernel schemas.
- **Responsibilities:**
  - Reviews every proposed new command for taxonomy compliance before approval.
  - Ensures clean directory layouts in `.zqk/cli/specs/` and schema integrity.
  - Prevents domain bleed and rejects ambiguous command verbs.

### Technical Documentarian Persona (`PER-TECHNICAL-DOCUMENTARIAN`)
- **Mission:** Guarantee documentation completeness, spec-to-code alignment, manual page accuracy, and user clarity.
- **Responsibilities:**
  - Enforces zero new spec drift and ratchets grandfathered legacy commands toward 100% declarative specification coverage, maintaining accurate manual pages and verified baseline synchronization.
  - Audits help strings, flag descriptions, and usage examples.
  - Eliminates undocumented flags, hidden arguments, and stale guidance.

---

## 5. Verification & Acceptance Reference

This standard is verified by the automated test suite in `cmd/zqk/system/validate_command_specs_test.go` and executed via `zqk system validate-command-specs`.

