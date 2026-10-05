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
| `--help` | bool | `false` | Displays help message and flag taxonomy for any command or subcommand. |
| `--version` | bool | `false` | Displays current ZQK binary version, build commit, and compiler metadata. |

### Common Command-Specific Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--allow-degraded` | bool | `false` | Allow scheduler-dependent commands to run when scheduler daemon is not running. (Available on specific commands, e.g., `system`). |

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

- `zqk init`: Initialize a new ZQK project
- `zqk quickstart`: Zero-friction project onboarding and quickstart guide
- `zqk run`: Run a portable agent swarm package

---

## Everyday Commands

The everyday command family covers core day-to-day interactions with the Knowledge Kernel:

- `zqk auth`: Session and authentication
- `zqk do`: Execute autonomous single-command workflow loop for a backlog item or task
- `zqk explain`: Explain kernel acronyms, ontology terms, and architectural concepts
- `zqk grep`: In-process trigram and AST code search
- `zqk inspect`: Interactive terminal object inspector and semantic projection viewer
- `zqk mutate`: Execute declarative ZQL mutations and transactions
- `zqk object`: Object operations (CRUD, query, and management)
- `zqk pplan`: Priority plan operations (current)
- `zqk query`: Execute declarative ZPARQL graph queries
- `zqk state`: State — inspect live knowledge kernel state graph and audit journals
- `zqk system`: System operations (health, validation, and maintenance)
- `zqk test`: Execute verification tests linked to test cases and criteria
- `zqk ui`: Interactive full-screen terminal mission control
- `zqk use`: Switch active context profile, project, or persona
- `zqk validate`: Validation and verification commands
- `zqk version`: Print version, commit, and build timestamp
- `zqk workflow`: Manage and execute autonomous engineering workflows and lifecycle pipelines

---

## Integrations & Daemon Operations

- `zqk automation`: Automation and integration operations (hooks, CI/CD, and scripts)
- `zqk callback`: Handle scheduler job callbacks and notifications
- `zqk ci`: Local CI (commit → checkout elsewhere → test run)
- `zqk completion`: Generate shell completion script
- `zqk daemon`: Manage background daemons under unified process group supervision
- `zqk feed`: Agent correspondence feed (steer / status onto agent_feed JSONL)
- `zqk inbox`: Inspect and manage the agent autonomy inbox
- `zqk intake`: Semantic ingestion pipeline (Intent Capture)
- `zqk kernel`: Knowledge Kernel governance, steward, and runtime lifecycle
- `zqk keystore`: Keystore operations (create, issue, list, rotate keys)
- `zqk learn`: Interactive curriculum mode
- `zqk mcp`: MCP server operations
- `zqk new`: Write draft YAML templates for object create and scenario bundles
- `zqk pre-commit`: Pre-commit background results (aggregate and write category results)
- `zqk reports`: Generate AI metrics reports (PCS, EDD, D&B)
- `zqk scheduler`: Manage background scheduler daemon, jobs, and recurrent tasks
- `zqk sync`: Synchronize Knowledge Kernel backlog items with external trackers
- `zqk tray`: Tray — configurable named shortcuts to zqk subcommands
- `zqk vendor`: Vendor-specific integrations and IDE adapters

---

## Advanced Knowledge Kernel Commands

- `zqk agent`: Multi-agent orchestration and delegation
- `zqk ambient`: Manage ambient event processing
- `zqk convergence`: Convergence measurement & nest management
- `zqk docman`: Documentation management operations
- `zqk domain`: Domain ontology discovery and registration
- `zqk graph`: Graph-based operations and reasoning
- `zqk job`: Background scheduler job triggers, queues, history, and status
- `zqk join`: Join a node or agent into an existing federation or mesh network
- `zqk kind-pack`: Record a verified spec pack as typed object kinds
- `zqk matrix`: Traceability matrices (CSV registries, validation, reports)
- `zqk mesh`: Manage peer-to-peer agent mesh networking, routing, and discovery
- `zqk observer`: Observer agent operations
- `zqk ontology`: Ontology import and translation
- `zqk ops`: Operations and utility commands
- `zqk organizational`: Organizational structure and change impact analysis
- `zqk pack`: Holonic swarm package management (scaffold, seal, validate)
- `zqk rollback`: List and apply rollback points (lifecycle/maintenance snapshots)
- `zqk semantic`: Semantic operations and maturity assessment
- `zqk service`: Manage per-root host OS scheduler units (launchd/systemd)
- `zqk spec`: Spec operations (list and manage object specifications)
- `zqk swarm`: Multi-agent swarm observability and throughput

---

## Additional Commands

- `zqk help`: Help about any command

---

## Concrete Operator Workflows

### 1. Booting an Agent Session and Discovering Work
```bash
# 1. Self-discover mission and active priority plan
zqk workflow whats-next --format json

# 2. Claim next available backlog item atomically
zqk agent claim BLI-12345

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
| `ZQK_CONTEXT_PROFILE` | Active execution context profile (`developer`, `agent`, `ci`, `human`). | `developer` |
| `ZQK_LOG_LEVEL` | Minimum log severity level (`debug`, `info`, `warn`, `error`). | `info` |
| `ZQK_TIMEOUT` | Global execution timeout duration for commands and sub-processes. | `30s` |
| `ZQK_ALLOW_DEGRADED` | Allow execution to proceed in degraded health or partially degraded network state. | `false` |
