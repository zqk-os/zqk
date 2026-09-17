# AI Agent Onboarding

**Scope:** Studio / process-dense dogfood (convergence, policies, scheduler habits).  
**Community strangers:** start with [`COMMUNITY_FIRST_RUN.md`](./COMMUNITY_FIRST_RUN.md) and `zqk system agent-onboard` — not this document.

**Welcome to zqk!** The very beginning of what will become earth's first fully-agentic and distributed operating system for the emerging era of super-productivity that will enable effective, efficient, accurate, safe, reliable and observable AI and human collaboration.

**Current plan and convergence:** After this document, read **[AGENT_ONBOARDING_SNAPSHOT.md](./AGENT_ONBOARDING_SNAPSHOT.md)** for live `priority_plan` / `convergence_session` pointers and verification commands. **Historical narrative:** [AGENT_ONBOARDING_SUMMARIES_DIGEST.md](./AGENT_ONBOARDING_SUMMARIES_DIGEST.md); **index:** [README.md](./README.md).

## Project Context

zqk is built on a distributed knowledge kernel architecture that enables AI agents and humans to collaborate effectively. The system uses a stable, production-ready CLI (`zqk`) that provides comprehensive object management, system operations, and workflow automation.

**Mission**: The canonical mission statement lives in process objects (e.g. `zqk object list mission --format table`, `zqk object get <MIS-*>`). It is linked from **VIS-001** via `mission_refs` for vision–mission alignment.

**Strategic Plan**: All work is aligned with STRAT-PLAN-001 (zqk 3-Year Strategic Plan 2026-2028), which defines three phases:
- **Phase 1: Foundation (2026)**: Core ontology, semantic bridge, and system commands
- **Phase 2: Advanced Features (2027)**: Organizational modeling, domain integration, strategic alignment
- **Phase 3: Scale and Optimization (2028)**: Enterprise features, multi-instance management, optimization

**MCP Integration**: The MCP server automatically exposes all CLI commands as tools via the CLI bridge pattern. All functionality is accessible through both direct CLI usage and MCP tools.

## Essential Routines

### First orientation (before raw object list)

1. **`zqk workflow whats-next --format json`** — self-discover current priority plan, active convergence, and next actions from the knowledge kernel.
2. Prefer that over starting with `zqk object list backlog_item` / `object list goal` as orientation.
3. Draft new objects with **`zqk object template <kind>`** (canonical); `zqk new object` is a shortcut.
4. **Operating skill:** load **`.zqk/skills/zqk-expert/SKILL.md`** (keep `skills/zqk-expert/` identical). Orchestration also loads the kernel twin **`agent_skill`** **`ASK-1785642072074959000-a1dfbde9`** (ZQK Expert Operating Protocol, `[orchestration-boot]`) via the prompt builder — keep ASK `instructions` and the filesystem pack in sync.
5. **Done claims:** run **`zqk workflow vds evaluate`** (policy **POL-WORKFLOW-VDS**) before marking BLI/CVS/CAP progress complete; narrative alone is not a gate.

### Role-Based Access Control (RBAC) & Authentication

- **RBAC is Active**: All CLI operations are subject to RBAC validation.
- **Agent Identity**: Agents must establish their identity by specifying the `ZQK_API_KEY="account:<account_name>"` environment variable or using `--context <profile>` when executing `zqk` commands. 
- **Setup Requirements**: Make sure that your specific persona account, role, and related `agent_skill` objects are properly configured in the Knowledge Kernel. If you run into permission errors, you may be missing required skills or executing commands without a properly authenticated context.

### Timestamp Management

- **All timestamp related information** must use the utilities available with the CLI (`zqk`; see `timestamp` and `time` commands in Utilities)

### Terminal Operation Safety

- **DESTRUCTIVE COMMANDS PROHIBITED**: You MUST NEVER execute destructive directory deletions (like `rm -rf`) without explicit, prior human authorization. This is a critical safety boundary to prevent accidental data loss.
- **All terminal operations** that AI agents execute which do not flow through the CLI (`zqk`) must be wrapped in the `command-timings` command wrapper in order to prevent terminal operations from becoming hung indefinitely.
- We should **never allow an operation to run for longer than 1 minute**, unless we're certain it will take longer.
- For processes that we anticipate taking longer than 1 minute, we will establish a reasonable timeout based on historical runtime information so as not to scope things substantially longer than reasonable.
- All commands that flow through the CLI already have timeout protection and will dynamically adjust timeout based on `.zqk/config.yaml` settings, so there's no reason to 'double-wrap'
- **NEVER launch background daemons (like `zqk scheduler start --foreground`) using asynchronous tasks to "fix" a locked-down CLI.** This orphans the process when your session ends and burns system resources.
- **If the CLI reports the scheduler daemon is down:** start it properly (`zqk scheduler start`, or `./scripts/recycle-stable-daemons.sh` after a promote). Do **not** treat `--allow-degraded` as the default fix — that flag means **partial/stale output is acceptable**, not that the kernel is healthy (see `docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md`). Use `--allow-degraded` only when you intentionally accept degraded results.

### Data Processing & Code Search

- **The use of `jq` or `yq` for json/yaml post-processing is highly discouraged.** The CLI (`zqk`) provides a `query` command for json path-based post-processing; prefer `zqk query` over ad hoc `jq`/`yq` where possible.
- **The CLI has a `yaml` command** for formatting and validating yaml data; prefer it before custom python scripting.
- **Native Code & AST Search (`zqk grep` / `zgrep`):** Use the in-process pure-Go search engine for fast sub-15ms codebase exploration. Support includes Go AST symbol queries (`--ast --kind struct|interface|func`, `--ast --recv <Type>`) and token-budgeted AI output (`--max-tokens 2000 -f json`). Avoid invoking slow external binaries or ad-hoc shell greps.

### Workspace Data & Namespace Lockdown

- **The `.gemini`, `.cursor`, and `.claude` directories in the project root are permanently locked at the OS level (`uchg`).**
- **Do NOT attempt to write workspace data, logs, settings, or agent skills to these directories.** You will receive a "Permission denied" error.
- **The designated, kernel-visible persistence layer is the `.zqk/` directory.**
- All agent workspaces, skill definitions, inbox artifacts, and settings must be written to `.zqk/agents/`, `.zqk/skills/`, or their corresponding `.zqk` subdirectories.

### Documentation Management

- **All documentation should live within the document index** in its proper location and should not be scattered throughout various code directories.

### Development Process

#### Test-Driven Development (TDD)

- **Utilize a TDD process** while implementing new logic.
- **Ensure all tests are written first** prior to any functional code being written.
- **Create Test Cases** to document and group related tests.
- **Utilize zqk criteria objects** to achieve traceability between backlog items, goals, requirements, milestones, mission, vision, and other system objects.

#### System Integrity

The zqk CLI maintains system integrity through automated mechanisms:
- Integrity hash validation
- Object ID cache management
- Audit event creation
- Reference validation
- Lifecycle state management

All object operations must use the CLI to ensure these integrity mechanisms are properly maintained.

**🛑 Structural & Architectural Integrity (The "Truth Sentinel" Mandate)**
Every agent operating within ZQK (via `orchestrate` or `evolve`) is strictly bound by the following AST-level architectural mandates:
*   **Zero Swallowed Errors:** You MUST NOT use the blank identifier (`_`) to swallow errors in critical paths (e.g., IO, storage, state updates). All errors must be explicitly handled, wrapped, or logged.
*   **Strict Dependency Injection:** You MUST use the `OrchestratorRegistry` or designated factories for service instantiation. Direct instantiation of core services (`NewManager`, `NewStorage`) is forbidden outside of bootstrap packages.
*   **Managed Concurrency:** Every background goroutine MUST propagate a `context.Context` and be managed by a structured lifecycle coordinator (e.g., `concurrency.InterruptChecker`).
*   **Enforcement:** Your code will be blocked from reaching the `complete` status if it fails the automated AST Structural Audit during the pre-commit or `zqk-qa-auditor` check.

#### Maintenance bundle (ensure-retention-jobs)

After **init** or when setting up a project for ongoing use, ensure the **maintenance bundle** scheduler jobs exist so the system stays healthy without ad-hoc runs:

```bash
zqk system ensure-retention-jobs
```

This ensures four jobs are present (creating from templates if missing):
- **Maintenance WAL trigger** (`job_type: maintenance`) — requests ordered aggregate-then-retention cycles on a schedule (e.g. hourly).
- **retention_tolerance** — archives/cleans objects per retention config.
- **audit_event_aggregation** — aggregates audit events into metrics.

Init supports `--with-maintenance-jobs` to run this automatically after init. See `docs/architecture/MAINTENANCE_WAL_AND_RUNNER.md` for the WAL design.

#### Code change discipline (before / after edits)

- **Before** substantive changes under `cmd/`, `pkg/`, or other CLI-impacting paths: consult **`docs/architecture/PRE_CHANGE_CHECKLIST.md`** (Cursor: `.cursor/rules/pre-change-checklist.mdc`) and **`docs/enforcement/AGENT_GUIDELINES.md`** for output, logging, and process-data rules.
- **Session reminder artifact:** **`docs/enforcement/AGENT_CONTEXT_REFRESH.md`** — regenerated by `scripts/agent-protocol-context-refresh.sh`; includes compliance status, **before implementing** checklist items (including **glossary** / `glossary_term` pointers), **after logic is verified (before merge)** (semantic density / **§13** post-verify), and a moment-of-risk table. Refresh if the snapshot looks stale. Runbook: `scripts/README.md` (Local verification + `agent-protocol-context-refresh.sh`).
- **Navigation vocabulary graph (GAPE / VIS-001 pillars / cross-walk):** Seeded **`vocabulary_scheme`** and **`glossary_term_relation`** edges — **`docs/architecture/VOCABULARY_GRAPH.md`**. After graph edits, run **`sh ./scripts/list-vocabulary-gtr.sh counts`**; see **`docs/architecture/PRE_CHANGE_CHECKLIST.md`** section 3b.
- **After** behavior is correct, before merge: run the **§13** post-verify — dedupe scaffolding, anchor policy literals, tighten readability. Goal: **semantic density** (domain intent stands out). See **`docs/best-practices/coding/DRY_PATTERN_EXTRACTION.md`** (rule of two / by the third).

#### Convergence sessions (active remediation)

Large, goal-directed pushes (for example **test bundles green**) are tracked in **`convergence_session`** objects (`CVS-*`), alongside backlog and priority plans. **At the start of a work session**, determine whether any convergence runs are active and how they should shape priorities:

```bash
zqk object list convergence_session --filter status=active --format json
```

- **If one or more are active:** Read `title`, `desired_end_state`, `current_phase`, `next_action`, and `iteration_process` on each. Prefer work that moves the session toward its target (for example fixing failing bundle fingerprints and re-running verification). For the current test-bundle snapshot, use `zqk scheduler convergence measure` (see `thresholds.snapshot_command` on the session when set).
- **If none are active:** Use backlog and priority plans as the primary drivers; convergence sessions are optional focused campaigns.
- **Do not edit** `.zqk/process/convergence_sessions/` YAML directly; use `zqk object get`, `zqk object list`, and `zqk object update` for `convergence_session` fields.
- **If active CVS objects “disappear” from `object list` but hash-named files still exist** under `.zqk/process/convergence_sessions/`, suspect **CAS index drift**—not necessarily deleted data.

Phase semantics (C1–C6) and vocabulary are tied to glossary terms (for example *convergence lifecycle*); follow `glossary_term_ref` on the session when present.

**Architecture cross-links (avoid conflating signals):** **`docs/architecture/CONVERGENCE_PREDICATES_AND_GATES.md`** — bundle **`ready_for_session_completion`** vs full **`desired_end_state`**. **`docs/architecture/CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md`** (section *Parity and convergence target*) — single-session measure/phase contracts across CLI, tick, and hook. **`docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md`** — rollup, orchestrator, nested **`CVS-*`**, **Appendix F** (multiple active sessions and **shared** `health.jsonl` / fingerprints). **`docs/architecture/EVENT_PIPELINE_AND_CONVERGENCE_INTEGRATION.md`** — when to use hook vs tick vs other pipelines.

**Documentation topology (indices today; ontology-backed linking deferred):** **[ASSESSMENT_AND_ONBOARDING_INDEX.md](../architecture/ASSESSMENT_AND_ONBOARDING_INDEX.md)** — central map; doc-graph backlog **`BLI-1776206564557363000-b402d9f5`** (**deferred**) with **`related_object_refs`** to glossary **`GLS-1776207925199440000-aa236125`** and **`doc_entry`** anchors. **Data-cell program** on **`PRI-1775858342094121000-73af1f15`** is the active alpha-track persistence work (see also **[EXPERTISE_ARCHITECTURE_AND_CODE_QUALITY.md](../architecture/EXPERTISE_ARCHITECTURE_AND_CODE_QUALITY.md)**). **Do not re-derive on every session:** **`agent_feed`** (delivery policy), CVS **`measure`/tick** (measurement), and **Cursor hooks** (IDE) are **different layers** — canonical one-page narrative: **[DATA_CELL_RUNTIME_ORGANISM.md](../architecture/DATA_CELL_RUNTIME_ORGANISM.md#data-cell-narrative)**.

**Post-retention stream stewardship:** the maintenance runner calls **`storage.PostRetentionStreamStewardship`**, which appends **`stream_steward_kind`** lines to **`datacell_steward_enqueue`** JSONL (per stream-backed kind / phase), drained by **`data_cell_envelope_tick`** — see **[STREAM_KIND_STEWARDSHIP.md](../architecture/STREAM_KIND_STEWARDSHIP.md)** (not the same layer as CVS **`measure`** or **`agent_feed`**).

**Test-bundle stream (even when no CVS is active):** Outcomes append to `.zqk/logs/scheduler/cvs/test-bundles/health.jsonl`. Use `zqk scheduler test-failures health` for a pass/fail timeline, and `zqk scheduler convergence measure` for fingerprints and **`suggested_rerun_commands`** on failures (refresh: `go run ./scripts/write_test_bundle_stream_summary`). See **`docs/architecture/DATA_STREAM_SUMMARY_PILOT.md`**.

#### Journey C (alpha gate): non-interactive JSON scripting

**Backlog:** `BLI-1776036642181883000-995c49ab`  
**Plan:** [CLI_ALPHA_LAUNCH_PLAN.md](../architecture/CLI_ALPHA_LAUNCH_PLAN.md) — Journey C (section 3).

Agents and automation should drive **`zqk`** without prompts. Use **`--context ai-agent`** (defaults many commands to JSON) and/or **`--format json`** explicitly. Exit code **`0`** means success; non-zero means failure (parse stderr / structured output as needed). Some commands emit **progress lines** before the final JSON object on stdout; for strict pipelines, parse the **last** complete JSON value or use **`zqk query`** on the combined output.

**Copy-paste smoke (no stdin; machine-readable):**

```bash
#!/usr/bin/env bash
set -euo pipefail
# Core read paths — stable top-level keys in JSON: "objects", "count", "success", etc. (see command help for each)
zqk object list backlog_item --limit 3 --format json --context ai-agent
zqk object get BLI-1776036642181883000-995c49ab --format json --context ai-agent
# Partial check only (refs skipped; NOT authoritative kernel health — see docs/architecture/check-fast-mode.md)
zqk system check --fast --format json --context ai-agent
```

For command metrics and failure-only views: `zqk system metrics --filter failures --format json --context ai-agent`. Prefer **`zqk query`** over ad hoc `jq` when shaping JSON (see **Data Processing** above).

**Verification / tests:** Do not block scripts on multi-minute bare `go test`; use spec-driven universal verification via **`zqk test run --all`** (or `make test-cases`). For package-scoped compilation and unit tests, use narrow `go test -timeout ≤60s` probes. Legacy scheduler `scan-tests` is deprecated under `REQ-TEST-BUNDLE-DEPRECATION-001`.

**Scheduler reliability (test jobs and timers):** Use **`zqk scheduler activity`** (and **`zqk scheduler history`**) to confirm bundles and timer jobs are dispatching. **`.zqk/scheduler/issues.json`** aggregates recent run_wrapper failures and timeouts; the daemon clears it to **`ok`** when problems age out and nothing new fails—if it stays in **`issues`**, investigate listed `job_id` entries. Missed triggers and health checks also surface via **`scheduler_health_metric`** objects and **`docs/archive/system_health/SCHEDULER_EVENTS_AND_METRICS.md`**.

#### Object Modification Best Practices

**⚠️ CRITICAL: Always use CLI/MCP for object operations - Direct YAML is PROHIBITED**

**MANDATORY WORKFLOW**: Before editing ANY YAML file, ask:
1. **Is this an object file?** (in `.zqk/process/{kind}/`) → **YES**: ❌ **STOP** - Use CLI/MCP
2. **Is this system metadata?** (specs, lifecycles, config) → **YES**: ✅ Direct YAML OK
3. **Is this documentation?** (Markdown) → **YES**: Use `zqk automation docman-sync`

- **Use MCP Tools (PREFERRED)**: The MCP server exposes all CLI commands as tools:
  ```bash
  # Check available tools
  tools/list
  
  # Create objects via MCP
  zqk_object_create with id="ROL-011", kind="role", fields={...}
  
  # Update objects via MCP
  zqk_object_update with id="ROL-011", field="title", value="New Title"
  
  # Delete objects via MCP
  zqk_object_delete with id="ROL-011"
  ```

- **Use CLI Commands (ALTERNATIVE)**: If MCP tools unavailable:
  ```bash
  # Create a new draft object (Mints ID and initializes boilerplate)
  zqk new object <kind> --title "My Object Title"
  
  # Update the auto-generated object with required specifics
  zqk object update <id> --field <field>=<value>
  zqk object update <id> --file <updates.yaml>

  # Promote the object to an active reviewable lifecycle status
  zqk object promote <id>
  
  # Delete objects (Always use --unlink-references to prevent GhostRefs)
  zqk object delete <id> --unlink-references
  
  # Bulk operations
  zqk object bulk create <kind> --file <items.yaml>
  zqk object bulk update --file <updates.yaml>
  
  # Template generation
  zqk object template <kind> --output template.yaml
  ```

- **NEVER edit object YAML files directly**: Direct file edits bypass the CLI's integrity mechanisms and will cause:
  - Hash registry sync issues (Tier 2 violations)
  - Cache inconsistencies
  - Missing audit trails
  - Validation bypass
  - Pre-commit hook failures

- **Exception Process**: If CLI doesn't support an operation:
  1. Create backlog item for the gap
  2. Use direct YAML (if absolutely necessary)
  3. Register hash: `zqk system check <id> --auto-fix --force`
  4. Document exception in commit message

- **Why this matters**: The CLI/MCP ensures:
  - All objects are properly registered with the system
  - Integrity hashes stay synchronized
  - The object ID cache remains accurate
  - Complete audit trails are maintained
  - Role-based security is enforced
  - System checks pass without violations

**See**: [AI Agent CLI-First Workflow](../process/architecture/AI_AGENT_CLI_FIRST_WORKFLOW.md) for complete workflow guide.

**Remember**: CLI/MCP is the normative path. Direct YAML is the exception, not the rule.

#### Git Workflow

- **Git branching, pull requests and traceability workflow:** All new development will occur in aptly named branches (`feature/`, `chore/`, `fix/`, `release/`).
- **There will never be a time when an AI agent commits code directly to the main branch** unless prior authorization has been granted/established.

### Preferred Development Workflow

The preferred workflow should progress as follows:

1. **Workstream Selection**
   - Most appropriate workstream object selected and made active

2. **Priority Planning**
   - Most appropriate priority plan set to 'current' with subsequent priority plans ordered by next sequential priority.

3. **Planning & Refinement**
   - Prioritization/planning/grooming/refinement exercises should be conducted when the roadmap reaches a place of ambiguity or uncertainty.
   - Please leverage the templates that live in the legacy project to bootstrap new activities.
   - The exercise should be a collaborative engagement with the appointed product representative which is likely to be a human but may be a product-focused agent.

4. **Code Quality Sign-Off & Pull Request Creation**
   - **MANDATORY AUDIT**: At the end of every coding evolution (prior to opening a PR), you MUST dispatch the `pedantic-code-inspector` and `qa-auditor` to review the changes. 
   - These personas will verify that all tests pass, the codebase is free of linting/formatting errors, and the implementation aligns perfectly with all project policies and AST mandates.
   - Pull requests should ONLY be opened using the `gh` CLI *after* the inspectors have completed their review and provided formal sign-off.
   - PR branches should not have any conflicts.
   - Once all changes have been staged, verified, and committed, the priority plan completion status should honor the lifecycle and should trigger the creation of a release object and release notes that accompany the commit and pull request.
   - The agent is free to stage, commit and push code remotely as early and often as desirable so long as not synchronizing the 'main' branch locally.

5. **Next Work Item Selection**
   - Once PR has been opened, the agent should identify the next workstream, priority plan, and backlog item to be worked.
   - Use `zqk object list priority_plan --filter 'status=active'` to find the current priority plan
   - Use `zqk object list backlog_item --filter 'priority_plan_ref=<plan_id>'` to find next items
   - A new local branch should be created and system objects should be transitioned to proper status if not automated by lifecycle triggers/constraints.

6. **Human-in-the-Loop PR Merge**
   - Human-in-the-loop will be responsible for merging the PR and notifying the agent when the remote merge has completed.

7. **Post-Merge Synchronization**
   - Once PR has merged, human will notify agent.
   - Agent will stash any current changes on the new branch, switch to local main, perform a `git fetch` and `git pull` to bring the main branch to current.

**High-level progression:**
```
local main 
  → new local working branch (feat, chore, fix etc...) 
  → remote working branch 
  → PR opened 
  → PR merged 
  → remote main 
  → checkout local main 
  → fetch 
  → pull 
  → checkout new branch (previously established) 
  → unstash local changes 
  → begin work
```

### Progress Tracking

- **Current project progress/status should be tracked in backlog items** to minimize the need to consult prior history.
- If a new agent joins, they should be able to read any currently in progress backlog item and understand project state and status.
- Backlog items should reference appropriate milestones to ensure that milestone progress is visible.

### Exploration lane ("boneyard")

Not every idea belongs in an active priority plan. When immediate value, integration fit, or ROI is unclear, place the item in the exploration lane and define promotion criteria before planning it as active work:

- [`../architecture/BACKLOG_EXPLORATION_LANE.md`](../architecture/BACKLOG_EXPLORATION_LANE.md)

### Core Architecture Advancements (Required Use)

The ZQK system has been enriched with the following core coordination and monitoring components. You MUST utilize these patterns instead of implementing custom polling or local state wrappers:

*   **Asynchronous Callback Coordination**:
    *   **Mechanism**: The scheduler coordinates cycles asynchronously via state-based triggers instead of polling. Whenever a `backlog_item` is completed, or a `convergence_session` changes status, lifecycle hooks in [lifecycle_coordination.go](file:///Users/lanceettl/ai-projects/zqk/pkg/scheduler/lifecycle_coordination.go) immediately fire to wake up the CAP orchestrator.
    *   **Implication**: Never write loops that poll for backlog status. Trust the event-driven scheduler.
*   **Real-time Test Bundle Progress & Live Matrix**:
    *   **Mechanism**: A progress tracking engine calculates running tallies (completed, total, pass, fail, running, pending) from the scheduler's test stream and publishes progress updates to `progress.jsonl`.
    *   **Use**: Run `zqk scheduler bundle-progress` to view live, per-bundle progress matrices of all test suites as they run.
*   **Event Stream Router (Cascading Overlays)**:
    *   **Mechanism**: High-performance, low-latency in-memory event bus in [pkg/events/](file:///Users/lanceettl/ai-projects/zqk/pkg/events/) routes object mutations. Local policy inspectors trap violations using bitmasks and propagate `Suspension_Triggered` cascades strictly via application-layer event routing, avoiding database bottlenecks.
*   **Kernel Steward (Local LLM Sentinel)**:
    *   **Mechanism**: A perpetual monitoring agent that executes during the `cap_stage_sentinel` stage. It reads a minimized high-signal `whats-next` json output, allocates its context window using a weighted budget allocator in [builder.go](file:///Users/lanceettl/ai-projects/zqk/pkg/agentprompt/builder.go), and queries the local LLM API (Ollama/Qwen/Gemini) to generate a precise two-sentence `AGENT DIRECTIVE` and `REASON`.

## Project Policies

**⚠️ CRITICAL: Before implementing any new functionality, query and understand project policies.**

### Policy Discovery

Project policies provide the central index for all project standards, expectations, and best practices:

```bash
# Query all policies
zqk object list policy

# Query architecture policies
zqk object list policy --filter category=architecture

# Query code quality policies
zqk object list policy --filter category=code_quality

# Query mandatory standards
zqk object list policy --filter policy_type=standard
```

**Policies are organized by category**:
- `architecture`: Architecture patterns and design principles
- `code_quality`: Code quality standards and best practices
- `documentation`: Documentation standards and conventions
- `testing`: Testing requirements and best practices
- `security`: Security policies and requirements
- `workflow`: Development workflow and process policies

**Policy types indicate enforcement level**:
- `standard`: Mandatory, blocks non-compliant code
- `requirement`: Must follow, exceptions require approval
- `guideline`: Should follow, reminders provided
- `best_practice`: Recommended, suggestions provided
- `anti_pattern`: What to avoid, warnings when detected

## Architecture Pattern Discovery

**⚠️ CRITICAL: Before implementing any new functionality, discover and follow established patterns.**

### Anti-Pattern Vocabulary (Required for Reviews)

Use the shared anti-pattern catalog to keep language consistent and actionable:

- [`../process/enforcement/ANTI_PATTERNS_BY_LANGUAGE.md`](../process/enforcement/ANTI_PATTERNS_BY_LANGUAGE.md)

When reporting issues, explicitly name the category (for example, "Magic Literals and Stringly-Typed Contracts", "Repeated Merge/Copy Loops", or "Over-Verbose Parse Paths") and include a small before/after example.

### Pattern Discovery Process

1. **Query Architecture Patterns**:
   ```bash
   # List all architecture documents
   zqk object list doc_entry --filter group=architecture
   
   # Review Architecture Patterns Library
   cat docs/architecture/ARCHITECTURE_PATTERNS.md
   ```

2. **Check for Existing Patterns**:
   - Search for similar implementations in codebase
   - Review related architecture documents
   - Check Architecture Patterns Library for established patterns

3. **Follow Established Patterns**:
   - If a pattern exists, **use it** - do not create new patterns
   - If extending a pattern, document the extension
   - If creating a new pattern, follow Architecture Review Process

4. **Complete Pre-Implementation Checklist**:
   - [ ] Checked existing architecture patterns
   - [ ] Reviewed related architecture documents
   - [ ] **Reviewed [Lessons Learned](../architecture/LESSONS_LEARNED.md) for relevant warnings**
   - [ ] Confirmed no duplicate patterns exist
   - [ ] Verified no import cycles will be created
   - [ ] Ensured CLI commands will be used (if MCP exposure needed)
   - [ ] Confirmed storage provider abstraction is used
   - [ ] Verified single context principle is followed
   - [ ] **Verified core CLI commands are working before proceeding**

### Key Architecture Principles

- **CLI Bridge Pattern**: All MCP exposure happens via CLI commands, not separate bridges
- **Context-Driven Bootstrap**: Systems bootstrap from context, not manual registration
- **Single Context Principle**: Use single merged context, derive variations as needed
- **CLI as Normative Path**: Always use CLI for object operations
- **Storage Provider Abstraction**: Use interfaces, not concrete implementations

See [Architecture Patterns Library](../architecture/ARCHITECTURE_PATTERNS.md) for complete pattern reference.

## Decision-Making Framework

**⚠️ CRITICAL: All decisions must follow the [Data-Driven Decision Framework](../architecture/DATA_DRIVEN_DECISION_FRAMEWORK.md).**

### OHTV Process (Observe → Hypothesize → Test → Verify)

**Never proceed without data.** Before taking any action:

1. **OBSERVE**: Collect actual data about current state
   - Check system status, logs, metrics
   - Sample processes if needed
   - Review documentation and policies

2. **HYPOTHESIZE**: Form data-driven hypothesis
   - Based on observed data, not assumptions
   - Identify testable predictions
   - Verify policy compliance

3. **TEST**: Design and execute verifiable test
   - Minimal, focused test
   - Define success criteria
   - Analyze results

4. **VERIFY**: Confirm outcome
   - Did it achieve the goal? (with data)
   - Check for side effects
   - Document outcome

**Red Flags**: Proceeding without data, making assumptions, violating policies, skipping verification.

## Lessons Learned Review

**⚠️ CRITICAL: Before implementing changes, review [Lessons Learned](../architecture/LESSONS_LEARNED.md) to avoid repeating past mistakes that led to system degradation.**

The lessons learned document captures critical patterns of failure:
- Masking symptoms instead of fixing root causes
- Editing persisted state directly instead of using CLI/API
- Proceeding when core commands are broken
- Lack of observability and verification

**Review frequency**: Weekly during active development, monthly during maintenance, before major releases.

## Next steps (live context)

Static copies of priorities go stale. After reading this guide, **re-query** current work before acting:

1. **In-progress and active priority plans**
   ```bash
   zqk object list priority_plan --filter 'status=in_progress' --format table
   zqk object list priority_plan --filter 'status=active' --sort-by active_order --format table
   ```

2. **Backlog for a plan** (replace `<PLAN_ID>` from step 1)
   ```bash
   zqk object list backlog_item --filter 'priority_plan_ref=<PLAN_ID>' --format table
   ```

3. **Active convergence sessions** (goal-directed remediation)
   ```bash
   zqk object list convergence_session --filter status=active --format json
   ```

4. **Strategic plan and quick partial check** (not authoritative health; use full `zqk system check` / `kernel-health-snapshot-check.sh` for gates)
   ```bash
   zqk object get STRAT-PLAN-001
   zqk system check --fast
   ```

5. **Onboarding and glossary discovery**
   ```bash
   zqk object list doc_entry --filter group=onboarding
   zqk object list glossary_term --format table
   ```

6. **Onboarding curriculum as data (templates and advanced-tutorial pattern)** — The repo ships YAML under [scripts/onboarding_roadmap/README.md](../../scripts/onboarding_roadmap/README.md) (priority plan, workstream, milestone, backlog items, seed job). Use that README’s *Reference pattern for advanced tutorials* when adding richer teachable tracks. To evaluate init + seed in isolation, see [ONBOARDING_EVALUATION_SCENARIO.md](../process/testing/ONBOARDING_EVALUATION_SCENARIO.md) (`zqk-ts`, `ZQK_TS_TEST_ROOT`).

For a condensed snapshot and links to supplementary guides, see [docs/onboarding/README.md](./README.md) (including dated **Agent Onboarding Summary** files).

## 6. Test Environment Hardening (Fork Bomb Prevention)

When writing Go tests that execute subprocesses (especially CLI tools or MCP servers):

1. **Explicit Binary Injection**: You MUST explicitly inject the path to the actively compiled workspace binary (e.g. `ZQK_BIN=bin/zqk`) into the test environment.
2. **Never Fallback to Test Runners**: When implementing path resolution (e.g., `os.Executable()`), include strict guards to prevent falling back to test binaries (e.g., `mcp.test`). If a test runner executes itself thinking it is a system CLI tool, it will execute the test suite recursively and cause an unrecoverable fork bomb.
3. **Fail Fast**: If a required binary cannot be resolved during a test, fail the test loudly (`t.Fatal`) rather than defaulting to a silent, unsafe fallback.

## Persona-Bound Autonomy and Task Validation Boundaries

**⚠️ CRITICAL: Agents must strictly adhere to their assigned persona roles. The system orchestrator MUST conditionally inject validation gates based on persona capabilities.**

When generating agent tasks or executing workflows, the definition of done and the required validation steps must match the persona's role:
1. **Engineering Personas** (Coders, Fixers, Architects): Tasks MUST inject mandatory codebase validation steps (e.g., `Must pass targeted validation script: ./bin/zqk agent validate`). These agents are expected to write code and prove compliance.
2. **Non-Technical Personas** (TPMs, Designers, Analysts, Strategy): Tasks MUST NOT inject codebase validation or compilation gates. Their definition of done revolves around outputting valid JSON/YAML, managing the kernel graph, organizing backlog items, or producing markdown documentation. Injecting code compilation gates into a TPM's task causes them to break character and write malformed Go scratch scripts to "satisfy" the gate, resulting in AST compliance failures and circuit breaker trips.

*Actionable Rule for Orchestrators*: When dynamically building `agent_task` payloads (e.g., in `orchestrate.go`), explicitly check the assigned Persona. If the role does not contain `coder`, `engineer`, `developer`, or `fixer`, you MUST omit codebase compilation/validation steps from the task payload.

## System Object Granularity Framework (Initiative Scoping)

**⚠️ CRITICAL: Agents MUST NOT compress large, cross-cutting features or architectural shifts into a single `backlog_item`. The richness of the knowledge kernel must be utilized to maintain traceability.**

When an agent or orchestrator plans a new body of work, they must use this decision matrix to determine the correct ontological cascade:

1. **Strategic Pivot / Vision Shift:**
   - *Requires:* A `strategic_plan` update, a new `mission`, and overarching `goal` objects.
2. **New Architecture or Cross-Cutting Initiative (e.g., Cellular Microkernel, Semantic CLI Filtering):**
   - *Requires:* An `epic` container (`EPC-`) defining the macro-enclave scope, blast radius, and invariant gates.
   - *Requires:* A `technical_spec` (`TSP-`) detailing the architectural design.
   - *Requires:* Multiple `requirement` (`REQ-`) objects bound to the spec and epic.
   - *Requires:* Actionable `criteria` (`CRIT-`) and `test_case` (`TST-`) objects to mathematically prove requirements.
   - *Requires:* One or more `priority_plan`s (`PRI-`) mapping the criteria to delivery sprints.
   - *Requires:* Multiple `backlog_item`s (`BLI-`) inside the plan (one for each atomic unit of code delivery).
3. **Discrete Feature / Component Enhancement:**
   - *Requires:* `requirement` -> `criteria` -> `backlog_item` cascade (linked to parent `epic` if part of a broader capability).
4. **Bug Fix / Technical Debt:**
   - *Requires:* A `technical_debt` or `bug` object (if schema allows) OR a single `criteria` documenting the fix condition, satisfied by a `backlog_item`.

**The Golden Rule of Backlog Items:**
A `backlog_item` is exclusively for an *atomic unit of execution* (e.g., "Implement the Persona filtering regex in zqk query"). It is NEVER used for an *initiative* (e.g., "Implement Persona-Aware Context Filtering"). If the task title implies a multi-step project or architectural shift, the agent MUST generate an `epic`, `technical_spec`, `requirements`, and a `priority_plan`.

### Native Graph Composition Pipelines

**⚠️ CRITICAL: NEVER write bash loops, awk scripts, or manual `zqk object create` chains to wire together an object cascade (like Spec -> Requirements -> Criteria).**

The kernel provides powerful native batch processing and LLM-ingestion pipelines for graph composition. When you need to scaffold an initiative, you must use one of these two methods:

1. **`zqk intake` (For Natural Language Ingestion):**
   Pipe unstructured text (e.g. "Create a Spec with two Requirements and active Criteria...") directly into `zqk intake`. The semantic engine will automatically parse the intent, validate against the ontology, create the objects, and construct the semantic links.
2. **`zqk object import --relaxed` (For Declarative YAML Graphs):**
   If you have a multi-document YAML array representing the objects, use `zqk object import --file <path.yaml> --relaxed`. The `--relaxed` flag explicitly tells the graph engine to resolve dependency links (`*_refs`) dynamically as the batch processes.

---

*This document should be the first reference point for any AI agent joining the zqk project. If you are reading this as a new agent, please review all sections carefully before beginning work.*

*Last Updated: 2026-03-27*


## 🛑 MANDATORY PULL REQUEST POLICY
- **NO DIRECT PUSHES:** Agents are strictly prohibited from pushing directly to main or protected branches.
- **BRANCHING MANDATE:** Always use feature branches (e.g., feat/..., fix/...) for all work.
- **PR REQUIRED:** All changes MUST be submitted via a Pull Request using the gh CLI.
- **ANTAGONISTIC REVIEW REQUIRED:** Before any PR can be merged to main, the code MUST be reviewed by the Antagonistic Auditor or Pedantic Code Inspector subagent. This subagent must aggressively provide feedback identifying policy violations, leftover ephemeral/scratch files, and coding best practice violations.
- **MERGE VIA PR:** Changes can only be merged after PR approval, automated checks, and the antagonistic review is resolved.

## Swarm Orchestration Convention

1.  **Mega-Branch Orchestration:** The main orchestrator must create a single feature branch (e.g., `feature/swarm-integration`) off of `main` for any batch task.
2.  **Worker Isolation:** Each swarm worker must branch off of this central feature branch (e.g., `subagent-feature-XYZ`) rather than creating isolated worktrees from `main`. The feature branch acts as the `main` branch for the swarm.
3.  **Strict Pedantic Gate:** Subagents CANNOT merge their code back into the orchestrator's feature branch until their code passes pedantic inspection (QA/Pedantic Code Inspector) and policy adherence.
4.  **Single PR:** Once all subagents have successfully implemented and verified their fixes, their code is merged into the central feature branch. Only ONE pull request is opened against the actual repository `main` branch for the entire swarm's output.

## General Attitude & Posture Perspective (The "Universe-Denting" Cadence)

To maximize throughput and speed to market without sacrificing quality and reliability, agents MUST operate with the following posture:
1. **Zero Idle Time via CAP Protocol**: We are constantly in a self-improving posture. If foreground engineering work is wrapping up or waiting on tests, agents MUST concurrently spin up Strategic Planning swarms to build out the next phase of the roadmap (vision, mission, priority plans). There is never an idle moment.
2. **Utilize System Capabilities**: Do not default to naive, monolithic processes (e.g., synchronous unbounded `go test ./...`). Prefer **`zqk test run --all`** (or `make test-cases`) for universal verification across the knowledge kernel DAG. Foreground `go test` only for narrow probes (`-timeout` ≤ 60s). See **zqk-expert** skill.
3. **Continuous Advancement**: We concurrently advance all strategic objectives to the fullest extent possible. If the tests pass, you push and PR. If they fail, you converge on stability immediately. 

This is our universe-denting moment; seize the day and the moment by utilizing ZQK's full parallelization and orchestration capabilities.
