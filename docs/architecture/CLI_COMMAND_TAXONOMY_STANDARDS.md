# Canonical CLI Command Taxonomy & Architectural Standards

**Document ID:** `DOC-CLI-COMMAND-TAXONOMY-STANDARDS-001`  
**Authoring Personas:**  
- `PER-INFORMATION-ARCHITECT` (Information Architect)  
- `PER-TECHNICAL-DOCUMENTARIAN` (Technical Documentarian)  
**Governing Goal:** `GOAL-CLI-TAXONOMY-CANONICAL-001`  
**Governing Requirement:** `REQ-CLI-TAXONOMY-HARMONIZATION-001`  
**Status:** Canonical / Enforced  

---

## 1. Executive Summary & Central Theme

The **ZQK Command Interface** is the primary human-agent operating surface for the Knowledge Kernel. As the system scales across multi-agent swarms and varied development environments, command sprawl, ambiguous verbs, un-specced commands, and vendor-specific leakage introduce cognitive fatigue, broken automation, and architectural degradation.

This document establishes the **authoritative ground rules, naming taxonomy, behavioral standards, and governance protocols** for all commands in `zqk`.

> [!IMPORTANT]
> **Zero Tolerance for Un-specced Commands:**  
> Every command in `zqk` must have a valid declarative specification in `.zqk/cli/specs/`. Any command added without a specification is treated as a build-breaking defect and trapped by `zqk system validate-command-specs`.

---

## 2. Core Architectural Ground Rules & Standards

### Rule 1: 100% Declarative Spec Coverage Mandatory
1. **Spec Location:** Every command and subcommand MUST have a matching YAML specification in `.zqk/cli/specs/<domain>/<command>_command.yaml`.
2. **Spec Completeness:** Specifications must document the command purpose, positional arguments, flags (with defaults and descriptions), expected output schema, and at least two realistic usage examples.
3. **Automated Verification:** The pre-commit gate and local CI run `zqk system validate-command-specs`. Commits that introduce un-specced commands or spec drift will fail closed.

### Rule 2: Strict Domain-Resource Grammar & Noun-Verb Hierarchy
1. **No Orphan Root Verbs:**  
   Root-level commands must represent distinct architectural **domains** (`system`, `agent`, `workflow`, `object`, `test`, `job`, `service`, `vendor`). Unqualified verbs (such as `use`, `run`, `check`, `execute`) are prohibited at the root level unless defined as explicitly documented, universal ergonomics aliases.
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
  - Verifies that 100% of commands have up-to-date specs and accurate manual pages.
  - Audits help strings, flag descriptions, and usage examples.
  - Eliminates undocumented flags, hidden arguments, and stale guidance.

---

## 5. Verification & Acceptance Reference

This standard is verified by the automated test suite in `cmd/zqk/system/validate_command_specs_test.go` and executed via `zqk system validate-command-specs`.

- **Priority Plan:** `PRI-CLI-TAXONOMY-OVERHAUL-001`
- **Backlog Items Verified:**
  - `BLI-CLI-TAXONOMY-HARMONIZE-001` (Harmonized CLI Taxonomy & 100% Spec Coverage)
  - `BLI-CLI-AGENT-FEED-001` (Unify Swarm & Feed Families)
  - `BLI-CLI-CONVERGENCE-001` (Convergence Management Parent)
  - `BLI-CLI-DEPRECATE-PURGE-001` (Purge Obsolete Commands & Specs)
  - `BLI-CLI-SCHEDULER-REORG-001` (Job & State Management Reorg)
  - `BLI-CLI-SERVICES-MGT-001` (Services Management Daemon Lifecycle)
  - `BLI-CLI-SPECS-LAYOUT-001` (Specs Directory Layout Restructuring)
  - `BLI-CLI-SPECS-COVERAGE-001` (100% Command Spec Coverage Audit)
  - `BLI-CLI-TAXONOMY-STANDARDS-001` (Taxonomy Standards Governance)
  - `BLI-CLI-VENDOR-ISOLATION-001` (Vendor Adapter Pluggable Isolation)
