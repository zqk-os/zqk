# ZQK CLI Command Reference & Operator Manual

This reference manual documents the complete CLI surface for the ZQK Knowledge Kernel platform. It provides exhaustive specifications for global flags, everyday commands, daemon services, advanced kernel operations, exit codes, and environment variables.

---

## Global Flags & Conventions

All `zqk` commands accept standard global flags governing execution context, serialization format, timeouts, and degradation policies.

### Core Global Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--context` | string | `ai-agent` | Active context profile: `ai-agent`, `human`, or `debug`. Governs token budgets and machine-readable output envelopes. |
| `--format` | string | `table` | Serialization format for command output: `table`, `json`, `yaml`, `stream`, or `json-rpc`. |
| `--timeout` | duration | `30s` | Maximum execution duration before timing out and failing closed. |
| `--allow-degraded` | bool | `false` | When true, permits read-only command execution when secondary indices or cache services are degraded. |
| `--help` | bool | `false` | Displays help message and flag taxonomy for any command or subcommand. |
| `--version` | bool | `false` | Displays current ZQK binary version, build commit, and compiler metadata. |

### Context Profiles

- **`ai-agent`**: Optimizes output for LLM ingestion. Suppresses decorative ASCII boxes and decorative spinners in favor of compact, token-dense structural JSON or concise tables.
- **`human`**: Rich interactive terminal output with colored highlights, ANSI tables, and user-facing hints.
- **`debug`**: Verbose logging, internal timing metrics, file descriptor counts, and lock acquisition tracing.

### Output Formats

- **`table`**: Human-readable ASCII table formatted to fit terminal width.
- **`json`**: Structured JSON payload conforming to standard ZQK result envelopes.
- **`yaml`**: Clean YAML representation suitable for version-controlled configurations.
- **`stream`**: Line-delimited JSON (`jsonl`) stream for long-running monitoring operations and event logs.
- **`json-rpc`**: JSON-RPC 2.0 wire protocol messages for MCP and agent communication channels.

---

## Getting Started

### Initializing and Seating Workspaces

- `zqk quickstart`: Interactive or automated onboarding wizard. Detects host environment, seats the default workspace, verifies prerequisites, and launches background daemons.

---

## Everyday Commands

The everyday command family covers core day-to-day interactions with the Knowledge Kernel:

- `zqk workflow`: Manages workflow execution states, next actions (`whats-next`), and Verifiable Decomposition Spine (VDS) done-gates.
- `zqk object`: Comprehensive CRUD, inspect, promote, and query operations for all kernel ontological objects.
- `zqk system`: Inspects platform health (`system check`), performs secret scans, validates command specifications, and diagnoses daemons.
- `zqk pplan`: Manages priority plans, Gantt matrices, child backlog item allocations, and execution order locks.
- `zqk grep`: Ultra-fast native code search utilizing Go AST parsing, trigram indexing, and token-budgeted AI payloads.
- `zqk test`: Test execution suite, runner harnesses, and verification matrix DoD checks.
- `zqk auth`: Authentication, session token validation, credential stores, and agent capability bindings.
- `zqk use`: Context switching and active project workspace binding.
- `zqk validate`: Validates instance YAML files, schemas, and ontological invariants against specifications.
- `zqk version`: Displays binary build commit, release version, and compiler toolchain information.

---

## Integrations & Daemon Operations

- `zqk scheduler`: Coordinates background cron jobs, task timers, test execution bundles, and lock cleanup.
- `zqk mcp`: Model Context Protocol (MCP) server daemon, tool registration, and tool evaluation interfaces.
- `zqk feed`: Manages agent correspondence channels, activity feed streaming, and message acknowledgements.
- `zqk inbox`: Manages incoming agent tasks, human-in-the-loop review requests, and notifications.
- `zqk intake`: Ingests external requirements, issues, and work streams into the draft plane.
- `zqk keystore`: Secure storage for cryptographic keys, tokens, and verification signatures.
- `zqk learn`: Records institutional knowledge, architectural decisions, and agent operational lessons into the kernel.
- `zqk new`: Scaffolds new repositories, plugins, adapters, and custom ontological packs.
- `zqk pre-commit`: Runs local pre-commit release gates, secret scans, tree police, and invariant checks.
- `zqk reports`: Generates engineering metrics, burndown charts, and quality evaluation summaries.
- `zqk tray`: macOS menu bar companion for live daemon telemetry and agent status.
- `zqk automation`: Orchestrates scheduled batch workflows, unattended maintenance, and event triggers.
- `zqk callback`: Handles asynchronous webhooks and IPC callback responses from background workers.
- `zqk ci`: Continuous integration pipeline runners, matrix test execution, and CI status reporters.
- `zqk completion`: Generates shell completion scripts for `bash`, `zsh`, and `fish`.

---

## Advanced Knowledge Kernel Commands

- `zqk agent`: Multi-agent orchestration, seat assignment, capability claims, and lock management.
- `zqk swarm`: Swarm topology management, parallel execution rings, and swarm pool health.
- `zqk ambient`: Background ambient intelligence daemon, telemetry monitoring, and signal accumulation.
- `zqk convergence`: Convergence session contracts (`CVS-*`), divergence checks, and alignment scoring.
- `zqk docman`: Document manager, Content-Addressable Storage (CAS) file qualification, and checksum verification.
- `zqk domain`: Domain taxonomy definitions, ontological boundaries, and pack configurations.
- `zqk graph`: Graph engine inspection, lineage visualization, dependency traversal, and topological sorting.
- `zqk join`: Executes relational joins and multi-domain projections across kernel objects.
- `zqk matrix`: Test verification matrix operations, DoD validation, and test case binding.
- `zqk mesh`: P2P mesh synchronization across distributed agent pods and storage membranes.
- `zqk observer`: Codebase AST observer, symbol resolution, and structure queries.
- `zqk ontology`: Ontological metamodel operations, kind definitions, and schema transformations.
- `zqk ops`: Low-level operational maintenance, storage compaction, and index rebuilds.
- `zqk organizational`: Team configurations, persona assignments, and governance policies.
- `zqk rollback`: Rollback journal execution, state restoration, and transaction recovery.
- `zqk semantic`: Semantic recall, embeddings generation, vector search, and glossary lookups.
- `zqk spec`: Command specification generator, spec validation, and schema builders.

---

## Concrete Operator Workflows

### 1. Booting an Agent Session and Discovering Work
```bash
# 1. Self-discover mission and active priority plan
zqk workflow whats-next --format json

# 2. Claim next available backlog item atomically
zqk agent claim BLI-12345 --atomic --skip-parallel

# 3. Execute implementation and continuous verification
zqk test run --all

# 4. Release lock and verify done-gate
zqk workflow vds evaluate BLI-12345
zqk agent release BLI-12345
```

### 2. Checking System Integrity and Resource Hygiene
```bash
# Verify kernel health, open FDs, and lock hygiene
zqk system check --details

# Audit open file descriptor ceilings across active daemons
./scripts/open-core/check-process-fd-leaks.sh
```

---

## Exit Codes & Error Verbs

The CLI enforces deterministic, fail-closed exit codes across all commands:

| Exit Code | Meaning | Description |
| :---: | :--- | :--- |
| `0` | **Success** | The command completed successfully with all invariants satisfied. |
| `1` | **Generic Failure** | Operational failure, network error, or uncaught execution exception. |
| `2` | **Validation / Invariant Failure** | Fail-closed gate violation, schema validation error, unmet precondition, or spec check failure. |
| `137` | **Timeout / Killed** | Execution exceeded configured `--timeout` or was terminated by `SIGKILL`. |

---

## Environment Variables

All behavior can be steered via standard environment variables:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `ZQK_PROJECT_ROOT` | Absolute path to the seated Knowledge Kernel project root directory. | Current working directory or parent traversal. |
| `ZQK_CONTEXT_PROFILE` | Default context profile (`ai-agent`, `human`, `debug`). | `ai-agent` |
| `ZQK_LOG_LEVEL` | Minimum log severity level (`debug`, `info`, `warn`, `error`). | `info` |
| `ZQK_TIMEOUT` | Global default command execution timeout. | `30s` |
| `ZQK_ALLOW_DEGRADED` | Enables degraded execution mode when secondary caches are offline. | `false` |
