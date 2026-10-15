# CLI System Command Subsystem & Kernel Health Runtime Guide

## Overview

The ZQK System Subsystem (`cmd/zqk/system`) provides the operational core for health monitoring, integrity validation, automated remediation, resource hygiene, and runtime lifecycle coordination across the Zen Quantum Kernel. It acts as the primary gatekeeper ensuring that all entities in the persistent Content Addressable Storage (CAS), active execution plane, and runtime state conform to strict ontological and cryptographic constraints.

The subsystem delivers five core capabilities:
1. **Multi-Layer Kernel Verification (`system check`)**: Evaluates the knowledge graph across four distinct layers (syntactic specifications, active in-flight execution, completed deliverables, and underlying OS/storage I/O telemetry).
2. **Automated Graph & Storage Remediation (`--auto-remedy` / `--auto-fix`)**: Safely purges stale lock files, fixes schema field omissions, cleans orphaned files, and repairs damaged reference graphs without manual operator intervention.
3. **Strategic Alignment Engine (`system align`)**: Quantifies real-time alignment between active priority plans, backlog items, and top-level mission, vision, and strategic goals.
4. **Policy Interrupt Firewall (`system policy-interrupts`)**: Detects and enforces precedence on critical workflow deviations, delivery disparities, and Done-gate violations, requiring explicit acknowledgement (`ack`) before normal execution proceeds.
5. **Runtime Lifecycle & Service Coordination (`init`, `status`, `start`, `shutdown`, `restart`, `whoami`)**: Supervises system bootstrap, daemon states, agent seating identity, and environment configurations.

---

## Architectural Taxonomy & Component Layout

```
cmd/zqk/system/
├── system.go                         # Parent 'system' command definition & subcommand registry
├── system_check_pipeline.go          # Multi-layer system check discovery & validation pipeline
├── check_impl.go                     # Core object validation runner & layer aggregation
├── check_impl_helpers.go             # Kind validation dispatch and schema assertion helpers
├── check_impl_autofix.go             # Auto-remedy engine for repairable schema & reference issues
├── check_cache_freshness.go          # Memory cache & CAS storage synchronization check
├── auto_fix_helpers.go               # Remediation recipes for lock files, fields, & refs
├── status.go                         # Runtime hive and daemon health inspection
├── status_helpers.go                 # Telemetry formatting & process health status summarizers
├── align_helpers.go                  # Strategic alignment scoring & gap analysis
├── policy_interrupts.go              # Critical policy interrupt detection and ack management
├── resource_hygiene.go               # File descriptor, disk volume, & stale lock monitoring
├── disk_usage.go                     # Storage breakdown across .zqk directories
├── stream_gc.go                      # Stream compaction and garbage collection
├── kernel_integrity.go               # Dangling reference detection and graph healing
├── whoami.go                         # Seating, agent profile, & caller identity resolution
└── ... (specialized repair, migration, and daemon controllers)
```

---

## 1. System Health & Verification Engine (`zqk system check`)

The `system check` command is the foundational verification tool of the ZQK operating system. It executes against all tracked objects in `.zqk/process/` and checks filesystem storage health.

### Command Syntax

```bash
zqk system check [flags]
```

### Supported Flags

| Flag | Type | Description |
|------|------|-------------|
| `--auto-remedy` | bool | Automatically applies all safe, deterministic remediation recipes (clearing stale locks, cleaning orphaned temp files, fixing known schema defaults). |
| `--auto-fix` | bool | Applies targeted field-level fixes to objects failing validation. |
| `--ids` | string | Restricts validation or auto-fixing to a comma-separated list of specific object IDs. |
| `--refresh-cache` | bool | Invalidates the warm memory object ID cache and forces cold filesystem rescanning. |
| `--relaxed` | bool | Suppresses non-fatal reference warnings during transitional states. |
| `--dry-run` | bool | Previews fixes that would be applied without mutating storage. |
| `-f, --format` | string | Output format (`table`, `json`, `jsonl`, `yaml`, `stream`). Default: `table`. |
| `--timeout` | duration | Maximum execution duration (0 = auto-calculated based on storage size). |

### Four-Layer Validation Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│ Layer 1: Core Definitions & Ontological Schemas                        │
│ • Policies, Prompt Templates, Vocabulary Schemes, Trait Specs         │
├────────────────────────────────────────────────────────────────────────┤
│ Layer 2: Active Lifecycle Plane (In-Flight Work)                       │
│ • In-Progress / Planned Priority Plans, Backlog Items, Agent Tasks     │
│ • Active Test Cases, Open Risk Blockers, Active Workstreams/Visions    │
├────────────────────────────────────────────────────────────────────────┤
│ Layer 3: Completed & Validated Artifacts                               │
│ • Complete Goals, Requirements, Test Cases, and Backlog Items          │
│ • QA Success Receipts and Archived Deliverables                        │
├────────────────────────────────────────────────────────────────────────┤
│ Layer 4: I/O Resource Hygiene & Storage Telemetry                      │
│ • Open File Descriptors (vs OS limit)                                  │
│ • Total Volume Tracking (.zqk file count and byte footprint)           │
│ • Stale Lock Files (.cas_sweep.lock, session.lock)                    │
│ • Orphaned Temporary and Staging Files                                 │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Strategic Alignment Engine (`zqk system align`)

The `align` command continuously computes the coherence score between the operational backlog and high-level strategic compasses.

### Command Syntax

```bash
zqk system align [flags]
```

### Core Responsibilities

1. **Alignment Scoring**: Generates a composite score (0–100) reflecting how completely planned work covers open goal gaps and strategic initiatives.
2. **Ambient Cache Persisting**: Persists ambient alignment state to `.zqk/state/ambient/align-latest.json` for low-overhead consumption by `whats-next` and scheduling loops.
3. **Goal Gap Surfacing**: Identifies unaddressed goals, orphaned backlog items, or unlinked milestones requiring triage.

---

## 3. Policy Interrupt Firewall (`zqk system policy-interrupts`)

The policy interrupt subsystem intercepts delivery discrepancies, test regressions, or unacknowledged workflow violations that take absolute precedence over new task claiming.

### Command Syntax

```bash
# List active policy interrupts
zqk system policy-interrupts

# Acknowledge an interrupt by deduplication key
zqk system policy-interrupts ack --dedupe-key <key>
```

### Operational Rules

- **Precedence Hierarchy**: Critical policy interrupts take precedence over convergence sessions and standard backlog routing.
- **Fail-Closed Execution**: If an unacknowledged policy interrupt exists, `zqk workflow next` refuses to route normal backlog tasks, guiding the operator to acknowledge or resolve the underlying disparity first.

---

## 4. Resource Hygiene & Telemetry

Storage and runtime integrity depend on strict filesystem hygiene:

- **Lock File Discipline**: Process-level operations use `.lock` files to prevent write collisions. If an agent or daemon terminates unexpectedly, locks exceeding the TTL threshold are flagged as attention items and automatically removed via `zqk system check --auto-remedy`.
- **File Descriptor Monitoring**: The subsystem monitors active file descriptor usage against `kern.maxfiles` to prevent descriptor exhaustion during large concurrent test runs or swarm batch operations.
- **Orphan File Detection**: The CAS membrane scanner cross-references files in `.zqk/process/` against the primary object index, identifying and isolating unindexed or orphaned data files.

---

## 5. Standard Operating Procedures

### Morning Inspection / Pre-Session Initialization

```bash
# 1. Warm check and auto-remedy stale locks
zqk system check --auto-remedy

# 2. Refresh ambient alignment cache
zqk system align --format json -o .zqk/state/ambient/align-latest.json

# 3. Verify workflow trajectory
zqk workflow whats-next
```

### Diagnosing Ghost References

When `zqk system check` reports a `GhostRef` (an object referencing an ID that does not exist in storage):
1. **Confirm absence**: `zqk object get <ghost-id>`
2. **If missing**: Re-mint the required dependency or use `zqk system kernel-integrity heal-dangling --apply`.
3. **If present but unindexed**: Run `zqk system check --refresh-cache` to force index re-indexing.
