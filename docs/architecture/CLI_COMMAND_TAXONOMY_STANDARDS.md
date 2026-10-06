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
1. **Root Taxonomy Tiers & Approved Ergonomics Shortcuts:**  
   Every command at the root level must belong to one of five approved taxonomy tiers. Unspecced or unapproved root verbs are prohibited:
   - **Tier 1: Core Architectural Domains:** Primary subsystems providing full lifecycle management (`system`, `agent`, `workflow`, `object`, `test`, `job`, `service`, `vendor`, `mesh`, `graph`, `state`, `ambient`, `scheduler`, `daemon`, `pack`, `mcp`, `keystore`, `kernel`).
   - **Tier 2: Day-0 Onboarding & Swarms:** Zero-friction developer entry points (`init`, `quickstart`, `run`).
   - **Tier 3: Everyday Utilities & Inspection:** High-frequency operational verbs and helpers (`auth`, `explain`, `grep`, `ui`, `version`, `pplan`).
   - **Tier 4: Approved Ergonomics Shortcuts:** Exactly 14 universal root shortcuts aliased to domain commands:
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
   - **Tier 5: Specialized & Extension Domains:** Deep operational modules and admin surfaces cataloged under `.zqk/cli/command_spec_coverage_baseline.json` and ratcheted toward 100% spec parity (`matrix`, `kind-pack`, `inbox`, `intake`, `feed`, `ci`, `docman`, `domain`, `ontology`, `ops`, `organizational`, `semantic`, `spec`, `swarm`, `convergence`, `observer`, `automation`, `callback`).
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

> **Implementation Note (Current State):** The canonical taxonomy below defines the approved classification and responsibilities for all root commands in `cmd/zqk/`. Core domains, Day-0 entry points, and ergonomics shortcuts provide the stable primary user interface, while specialized/extension domains are managed under `.zqk/cli/command_spec_coverage_baseline.json` and ratcheted toward 100% declarative specification parity.

| Command / Domain | Tier | Primary Responsibilities | Example Commands |
| :--- | :--- | :--- | :--- |
| **`zqk init`** | Day-0 Onboarding | Greenfield project knowledge kernel initialization | `init`, `init my-project` |
| **`zqk quickstart`** | Day-0 Onboarding | Interactive zero-friction project onboarding and quickstart guide | `quickstart`, `quickstart --agent` |
| **`zqk run`** | Day-0 / Swarms | Portable multi-agent swarm package execution (local or remote Git) | `run swarm.yaml`, `run https://github.com/org/swarm` |
| **`zqk auth`** | Everyday Utility | Interactive session authentication and credential token management | `auth login`, `auth logout`, `auth status` |
| **`zqk explain`** | Everyday Utility | Progressive disclosure acronym explainer and ontology glossary | `explain VDS`, `explain CAS`, `explain BLI` |
| **`zqk grep`** | Everyday Utility | In-process trigram and polyglot AST code search engine | `grep "Pattern" pkg/`, `grep --ast "func Test*"` |
| **`zqk pplan`** | Everyday Utility | Priority plan everyday convenience shortcut | `pplan current`, `pplan next`, `pplan add` |
| **`zqk ui`** | Everyday Utility | Full-screen terminal mission control console (state, audit, swarm, PM, metrics) | `ui`, `ui -w`, `ui --tab metrics` |
| **`zqk version`** | Everyday Utility | Output version, commit hash, build timestamp, and license info | `version`, `version --json` |
| **`zqk system`** | Core Domain | Host environment, kernel health, resource hygiene, spec validation, initialization | `system check`, `system resource-hygiene`, `system validate-command-specs` |
| **`zqk workflow`** | Core Domain | Process flow, next-action discovery, pipeline generation, VDS gating | `workflow whats-next`, `workflow gen-trace-pipeline`, `workflow vds evaluate` |
| **`zqk object`** | Core Domain | Full CRUD, relationship traversal, draft plane promotion across all kernel objects | `object get <id>`, `object list <kind>`, `object create <kind>`, `object promote <id>` |
| **`zqk agent`** | Core Domain | Multi-agent swarm orchestration, seating, task claims, execution delegation | `agent claim-work`, `agent orchestrate`, `agent prepare-context` |
| **`zqk test`** | Core Domain | Test execution, requirement-to-test verification, test suite dashboards | `test run <tst-id>`, `test dashboard`, `test matrix` |
| **`zqk job`** | Core Domain | Background scheduler job triggers, queues, history, and status | `job list`, `job trigger <id>`, `job status <id>` |
| **`zqk service`** | Core Domain | Host OS service supervisor (launchd / systemd), daemon lifecycles | `service start`, `service status`, `service stop` |
| **`zqk vendor`** | Core Domain | Isolated third-party adapters and IDE integration shims | `vendor cursor paste`, `vendor vscode register` |
| **`zqk ambient`** | Core Domain | Ambient filesystem monitoring, metric waves, telemetry capture | `ambient wave`, `ambient status` |
| **`zqk state`** | Core Domain | Knowledge kernel state graph inspection, audit journals, telemetry | `state stream --dashboard`, `state tree`, `state journal` |
| **`zqk graph`** | Core Domain | Graph database operations, reasoning, and ZPARQL query engine | `graph query`, `graph join`, `graph neighbors` |
| **`zqk mesh`** | Core Domain | P2P agent mesh networking, sync, routing, and federation | `mesh status`, `mesh sync`, `mesh peer` |
| **`zqk mcp`** | Core Domain | Model Context Protocol (MCP) server lifecycle and IDE config | `mcp install`, `mcp serve`, `mcp ensure` |
| **`zqk pack`** | Core Domain | Holonic swarm package management (scaffold, seal, validate) | `pack scaffold`, `pack seal`, `pack validate` |
| **`zqk scheduler`** | Core Domain | Background scheduler daemon, cron tasks, and maintenance | `scheduler status`, `scheduler run` |
| **`zqk daemon`** | Core Domain | Background daemon process group supervisor and lease manager | `daemon list`, `daemon status` |
| **`zqk keystore`** | Core Domain | Cryptographic key management, signing, and token issuance | `keystore list`, `keystore issue`, `keystore rotate` |
| **`zqk kernel`** | Core Domain | Knowledge Kernel runtime steward, integrity, and governance | `kernel steward`, `kernel check` |
| **`zqk do`** | Ergonomics Shortcut | Autonomous CAP loop execution shorthand (`zqk workflow vds do`) | `do`, `do BLI-123` |
| **`zqk inspect`** | Ergonomics Shortcut | Interactive TUI Object Inspector and Policy Studio (`zqk object inspect`) | `inspect <id>` |
| **`zqk mutate`** | Ergonomics Shortcut | ZQL declarative mutation engine shortcut (`zqk object mutate`) | `mutate <zql-query>` |
| **`zqk query`** | Ergonomics Shortcut | ZPARQL graph query language engine shortcut (`zqk graph query`) | `query <zparql>` |
| **`zqk validate`** | Ergonomics Shortcut | Invariant gate and schema validation shortcut (`zqk system validate`) | `validate --all` |
| **`zqk rollback`** | Ergonomics Shortcut | Transaction rollback journal restoration shortcut (`zqk object rollback`) | `rollback list`, `rollback apply <id>` |
| **`zqk completion`**| Ergonomics Shortcut | Shell completion script generator (`zqk system completion`) | `completion zsh`, `completion bash` |
| **`zqk sync`** | Ergonomics Shortcut | Storage CAS and P2P mesh synchronization shortcut (`zqk mesh sync`) | `sync github`, `sync linear` |
| **`zqk pre-commit`**| Ergonomics Shortcut | Local pre-commit release gate and secret scan runner (`zqk system pre-commit`) | `pre-commit` |
| **`zqk learn`** | Ergonomics Shortcut | Institutional memory and operational lessons capture (`zqk agent learn`) | `learn` |
| **`zqk new`** | Ergonomics Shortcut | Scaffolding wizard for new packs, adapters, and schemas (`zqk object new`) | `new goal`, `new plan`, `new req` |
| **`zqk reports`** | Ergonomics Shortcut | Quality evaluation and engineering velocity reporting (`zqk system reports`) | `reports` |
| **`zqk tray`** | Ergonomics Shortcut | macOS status bar daemon companion shortcut (`zqk service tray`) | `tray` |
| **`zqk join`** | Ergonomics Shortcut | Multi-domain relational projection and graph join (`zqk graph join`) | `join` |
| **`zqk matrix`** | Extension / Admin | Traceability matrices (CSV registries, validation, reports) | `matrix validate`, `matrix list` |
| **`zqk kind-pack`** | Extension / Admin | Record a verified spec pack as typed object kinds | `kind-pack record` |
| **`zqk inbox`** | Extension / Admin | Inspect and manage the agent autonomy inbox | `inbox list`, `inbox process` |
| **`zqk intake`** | Extension / Admin | Semantic ingestion pipeline (Intent Capture) | `intake process` |
| **`zqk feed`** | Extension / Admin | Agent correspondence feed (steer / emit-status) | `feed steer`, `feed status` |
| **`zqk ci`** | Extension / Admin | Local CI simulation (commit → checkout elsewhere → test run) | `ci run` |
| **`zqk docman`** | Extension / Admin | Documentation discovery, frontmatter verification, index management | `docman verify` |
| **`zqk domain`** | Extension / Admin | Domain ontology discovery and registration | `domain list` |
| **`zqk ontology`** | Extension / Admin | Ontology import and translation | `ontology import` |
| **`zqk ops`** | Extension / Admin | Operations and utility commands | `ops compact` |
| **`zqk organizational`**| Extension / Admin | Organizational structure and change impact analysis | `organizational list` |
| **`zqk semantic`** | Extension / Admin | Semantic operations and maturity assessment | `semantic eval` |
| **`zqk spec`** | Extension / Admin | Declarative specification inspection and management | `spec list` |
| **`zqk swarm`** | Extension / Admin | Multi-agent swarm observability and throughput | `swarm status` |
| **`zqk convergence`**| Extension / Admin | Convergence measurement & nest management | `convergence status` |
| **`zqk observer`** | Extension / Admin | Observer agent operations and AST extraction | `observer search` |
| **`zqk automation`**| Extension / Admin | Automation and integration operations (hooks, CI/CD, scripts) | `automation run` |
| **`zqk callback`** | Extension / Admin | Handle scheduler job callbacks and notifications | `callback handle` |

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

