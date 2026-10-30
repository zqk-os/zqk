# CLI Command Taxonomy, Operations & Extensible Architecture Guide

Comprehensive operational and architectural guide for CLI command composition, internal object processing, atomic bulk operations, lifecycle hooks, and the extensible CLI generation engine in the ZQK Knowledge Kernel.

---

## 1. Executive Architectural Summary & Command DNA

The `pkg/zqkcli` package implements the operational foundation for CLI object processing and internal subsystem commands within the ZQK ecosystem. It bridges human operator interactions (via interactive CLI and TUI) and autonomous AI agent operations (via headless JSON/YAML streams and Model Context Protocol tools) with Content-Addressed Storage (CAS) and the Knowledge Kernel graph membrane.

### Architectural Invariants & Guarantees

1. **Strict Statement Test Coverage ($\ge 85.0\%$ Invariant):** All CLI command handlers, bulk mutation pathways, grouping engines, and formatting transformers enforce rigorous unit and integration test coverage.
2. **Atomic Bulk Operations:** Multi-object mutations (`bulk create`, `bulk update`, `bulk delete`) operate under transactional boundaries to prevent partial corruption and ensure deterministic rollback on failure.
3. **Fail-Closed Scheduler Guards:** High-risk write operations verify daemon and scheduler readiness unless explicitly overridden by authorized maintenance actors via `--ignore-scheduler-down`.
4. **Unified Command DNA:** All commands inherit standardized flag parsing, context propagation, structured error wrapping (`pkg/errfmt`), and multi-format output streaming (table, JSON, JSONL, YAML, CSV, Markdown, HTML, and JSON-RPC).

---

## 2. Command Architecture & Subsystem Taxonomy

The CLI command hierarchy is organized into cohesive operational categories:

```
zqk internal
├── list                  # List internal and built-in objects with filters, grouping, and pagination
├── get                   # Retrieve single or multiple objects by ID or spec definition
├── create                # Create internal objects from file or inline YAML/field inputs
├── update                # Mutate fields or transition status on existing objects
├── delete                # Purge objects with cascade or GhostRef unlink guards
├── count                 # Fast aggregate counting with parallel workers and group-by
├── fields                # Inspect schema field specs, types, and descriptions
├── process               # Execute declarative multi-operation reference files
└── bulk                  # High-throughput batch operations
    ├── create            # Batch ingestion from YAML object arrays
    ├── update            # Batch field mutation with transactional commit
    ├── get               # Multi-ID batch retrieval
    └── delete            # High-throughput multi-object erasure with cascade
```

### Command Roles & Responsibilities

| Command | Primary Use Case | Output Formats | Concurrency / Safety Guards |
| :--- | :--- | :--- | :--- |
| `list` | Inspecting cataloged internal objects across storage and memory | Table, JSON, YAML, JSONL, CSV, Markdown | Pagination limits, memory cap, source type filtering |
| `get` | Fetching authoritative state of internal objects, specs, and lifecycles | Table, JSON, YAML, Raw | Fallback to disk specs, schema validation |
| `create` | Originating single internal objects | Table, JSON, YAML | ID validation, scheduler guard, schema compliance |
| `update` | Applying field-level patches or lifecycle status changes | Table, JSON, YAML | Dry-run preview, write-behind CAS flush |
| `delete` | Removing internal entities from storage | Table, JSON, YAML | GhostRef refusal, built-in object warnings |
| `count` | Rapid quantitative aggregation across object kinds | Table, JSON, YAML | Bounded worker pools, parallel kind execution |
| `bulk` | High-throughput batch mutations | Table, JSON, YAML | Transactional chunking (500 items/chunk), rollback safety |
| `process` | Replaying complex multi-step recipe files | Table, JSON, YAML | Per-operation error handling, continue-on-error |
| `fields` | Discovering schema fields, descriptions, and rules | Table, JSON, YAML | Field registry caching, dynamic completion |

---

## 3. High-Throughput Bulk Operations & Transactional Semantics

The `bulk` command family provides high-performance data processing pipelines designed for batch migration, graph bootstrapping, and scheduled retention sweeps.

### Batch Chunking & Memory Ceilings

To adhere to memory hygiene policies, bulk mutations chunk operations into discrete atomic transactions:

- **Chunk Size Limit:** Operations are processed in batches of 500 items.
- **Rollback Isolation:** If any operation within an atomic chunk fails, the entire chunk rolls back via `ObjectTransaction.Rollback()`.
- **Memory Footprint Optimization:** Results accumulate lightweight identifiers (`map[string]any{"id": ID}`) rather than full object states during bulk passes, eliminating heap spikes.

```
Incoming Update Items [1..N]
          │
          ▼
┌─────────────────────────────────┐
│ Chunk Slicer (Max 500 items)    │
└─────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────┐
│ BeginTransaction(ctx)           │
├─────────────────────────────────┤
│ • Apply op[0]                   │
│ • Apply op[1]                   │
│ • ...                           │
│ • Apply op[499]                 │
└─────────────────────────────────┘
     │                    │
  (Success)            (Failure)
     │                    │
     ▼                    ▼
┌─────────┐          ┌──────────┐
│ Commit  │          │ Rollback │
└─────────┘          └──────────┘
```

### Dry-Run Verification

All mutating commands (`bulk create`, `bulk update`, `create`, `update`, `delete`) support the `--dry-run` flag. In dry-run mode:
- Schema rules and ID formats are strictly evaluated.
- Pre-mutation state is compared against requested mutations to render a unified diff.
- Storage writes, WAL appends, and cache invalidations are completely bypassed.

---

## 4. Lifecycle Hooks & Scheduler Guard Enforcement

### Scheduler Guard Architecture

The scheduler guard (`pkg/zqkcli/scheduler_guard.go`) acts as a check-valve to prevent desynchronization between persistent storage and background scheduler worker loops:

1. **Pre-Execution Check:** Mutating commands check if the daemon scheduler is operational.
2. **Lockdown Refusal:** If the scheduler is in a degraded or locked-down state, mutating commands fail closed to protect consistency.
3. **Emergency Override:** Operators performing offline maintenance can pass `--ignore-scheduler-down` to explicitly bypass the guard.

### Fail-Closed Delete & GhostRef Sentinel

Deleting objects in a relational graph carries the danger of dangling references (GhostRefs). The `delete` command enforces fail-closed safety:
- **Refusal Without Flags:** Running `zqk internal delete <id>` without flags is immediately refused.
- **Explicit Invariant Resolution:** Operators must specify either `--cascade` (recursively deleting dependents) or `--unlink-references` (cleaning incoming edge references before erasure). Both flags cannot be combined.
- **Built-in Object Warning:** Deleting entities with `built_in: true` metadata emits an explicit operator warning prior to execution.

---

## 5. Extensible CLI Example Generation Engine

The `CLIExampleGenerator` (`pkg/zqkcli/cli_example_generator.go`) dynamically compiles verifiable CLI recipes from live schema definitions and object specifications.

### Capabilities

- **Command Syntax Synthesis:** Automatically generates executable bash/zsh command examples for all standard ontological kinds.
- **Synthetic Fixture Ingestion:** Employs field generators to synthesize valid, schema-compliant sample data for tutorials and developer documentation.
- **Bootstrapping Scripts:** Assembles multi-object graph creation scripts ensuring parent-child dependencies (e.g., mission -> goal -> requirement) are sequenced in valid topological order.

---

## 6. Coverage & Quality Assurance Invariants

In accordance with kernel verification policies, `pkg/zqkcli` maintains continuous automated verification:

| Metric / Dimension | Requirement | Verification Strategy |
| :--- | :--- | :--- |
| **Statement Coverage** | $\ge 85.0\%$ Floor | In-process integration suites exercising all commands, error paths, and formatters |
| **AST Concurrency** | Named goroutines | Strict enforcement of `goroutinelabels` across all parallel workers |
| **Resource Hygiene** | Zero descriptor leaks | Strict cleanup of all test temp directories and file handles |
| **Payload Integrity** | Zero leaked IDs | Verified against `scripts/open-core/check-public-release-payload.sh` |

---

## 7. Operational Recipes & Command Examples

### 1. Batch Creating Internal Objects
```bash
# Ingest multiple kind synonyms from a YAML array
zqk internal bulk create kind_synonym --file /tmp/synonyms.yaml

# Perform dry-run preview before creating
zqk internal bulk create kind_synonym --file /tmp/synonyms.yaml --dry-run
```

### 2. Updating Objects with Dynamic Fields
```bash
# Update title and status via inline flags
zqk internal update OBJ-001 --field title="Updated Title" --field status=active

# Update using a structured YAML delta
zqk internal update OBJ-001 --data "title: New Title\npriority: high\n"
```

### 3. High-Throughput Aggregations
```bash
# Count objects grouped by status
zqk internal count audit_event --group-by status

# Count multiple kinds in parallel with JSON output
zqk internal count audit_event,change_journal_entry --format json
```

### 4. Executing Multi-Step Reference Operations
```bash
# Process a declarative operations manifest
zqk internal process --file /path/to/manifest.yaml

# Preview operations execution in dry-run mode
zqk internal process --file /path/to/manifest.yaml --dry-run
```
